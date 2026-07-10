package dev.foxxycode.intellij.process

import com.intellij.util.net.HttpConfigurable
import java.net.InetSocketAddress
import java.net.Proxy
import java.net.ProxySelector
import java.net.URI
import java.net.URLEncoder
import java.nio.charset.StandardCharsets

/** Builds proxy environment variables for the bundled `foxxycode http` process. */
internal object ProxyEnvironment {
    private val proxyKeys = listOf(
        "HTTP_PROXY",
        "HTTPS_PROXY",
        "ALL_PROXY",
        "http_proxy",
        "https_proxy",
        "all_proxy",
    )
    private val noProxyKeys = listOf("NO_PROXY", "no_proxy")

    // Representative external HTTPS endpoint used to resolve the effective proxy in auto-detect/PAC
    // mode. A PAC script selects a proxy per destination; LLM API traffic is external, so a neutral
    // external host yields the proxy such requests will use (internal/external split PACs agree here).
    private const val PROBE_URL = "https://api.openai.com"

    fun intellijProxyEnvironment(): Map<String, String> {
        val configurable = HttpConfigurable.getInstance()
        val manual = buildProxyEnv(configurable)
        if (manual.isNotEmpty()) return manual
        // Fall back to "Auto-detect proxy settings" (PAC/WPAD), which the manual path does not cover.
        return autoDetectedProxyEnv(configurable)
    }

    /**
     * Builds the proxy env map from any object exposing IntelliJ's `HttpConfigurable` field/method
     * contract via reflection. Kept separate from [intellijProxyEnvironment] so it can be unit-tested
     * with a fake configurable, without booting the IntelliJ platform.
     */
    internal fun buildProxyEnv(configurable: Any): Map<String, String> {
        val proxyUrl = proxyUrl(configurable) ?: return emptyMap()
        val out = LinkedHashMap<String, String>()
        for (key in proxyKeys) out[key] = proxyUrl
        noProxy(configurable)?.let { value ->
            for (key in noProxyKeys) out[key] = value
        }
        return out
    }

    /**
     * Env for IntelliJ "Auto-detect proxy settings" (PAC/WPAD), which [buildProxyEnv] does not cover.
     * Resolves the effective proxy for a representative external URL through the IDE-wide
     * [ProxySelector] (IntelliJ installs one that honors PAC/auto-detect) and forwards it.
     *
     * Caveat: the bundled foxxycode (Go) binary supports only Basic proxy auth. An NTLM/Kerberos
     * corporate proxy still returns 407 even when forwarded here — pair it with a local NTLM
     * forwarder (e.g. Px/cntlm) exposed as a no-auth proxy and select that in Manual mode instead.
     */
    private fun autoDetectedProxyEnv(configurable: Any): Map<String, String> {
        if (booleanField(configurable, "USE_PROXY_PAC") != true) return emptyMap()
        val selector = ProxySelector.getDefault() ?: return emptyMap()
        val proxies = runCatching { selector.select(URI.create(PROBE_URL)) }.getOrNull() ?: return emptyMap()
        val proxyUrl = resolvedProxyUrl(proxies, authPrefix(configurable)) ?: return emptyMap()
        val out = LinkedHashMap<String, String>()
        for (key in proxyKeys) out[key] = proxyUrl
        return out
    }

    /**
     * Builds a proxy URL from a [ProxySelector.select] result. Picks the first non-DIRECT entry with a
     * socket address; emits `socks5://` for SOCKS proxies and `http://` otherwise. Pure/testable.
     */
    internal fun resolvedProxyUrl(proxies: List<Proxy>, authPrefix: String): String? {
        val proxy = proxies.firstOrNull {
            it.type() != Proxy.Type.DIRECT && it.address() is InetSocketAddress
        } ?: return null
        val addr = proxy.address() as InetSocketAddress
        val host = addr.hostString?.trim().orEmpty()
        if (host.isBlank()) return null
        val scheme = if (proxy.type() == Proxy.Type.SOCKS) "socks5" else "http"
        val authorityHost = if (host.contains(":") && !host.startsWith("[")) "[$host]" else host
        val portPart = if (addr.port > 0) ":${addr.port}" else ""
        return "$scheme://$authPrefix$authorityHost$portPart"
    }

    private fun proxyUrl(configurable: Any): String? {
        if (booleanField(configurable, "USE_HTTP_PROXY") != true) return null
        val host = stringField(configurable, "PROXY_HOST")?.trim().orEmpty()
        if (host.isBlank()) return null
        val port = intField(configurable, "PROXY_PORT") ?: 0
        val auth = authPrefix(configurable)
        val authorityHost = if (host.contains(":") && !host.startsWith("[")) "[$host]" else host
        val portPart = if (port > 0) ":$port" else ""
        return "http://$auth$authorityHost$portPart"
    }

    private fun authPrefix(configurable: Any): String {
        if (booleanField(configurable, "PROXY_AUTHENTICATION") != true) return ""
        val login = stringField(configurable, "proxyLogin")?.takeIf { it.isNotBlank() } ?: return ""
        val password = stringMethod(configurable, "getPlainProxyPassword")
            ?: stringMethod(configurable, "getProxyPassword")
            ?: ""
        return encode(login) + ":" + encode(password) + "@"
    }

    private fun noProxy(configurable: Any): String? {
        val raw = stringField(configurable, "PROXY_EXCEPTIONS")?.trim().orEmpty()
        if (raw.isBlank()) return null
        val normalized = raw
            .split(Regex("[,;|\\s]+"))
            .map { it.trim() }
            .filter { it.isNotEmpty() }
            .joinToString(",")
        return normalized.ifBlank { null }
    }

    private fun booleanField(target: Any, name: String): Boolean? =
        runCatching { target.javaClass.getField(name).get(target) as? Boolean }.getOrNull()

    private fun intField(target: Any, name: String): Int? =
        runCatching { target.javaClass.getField(name).get(target) as? Int }.getOrNull()

    private fun stringField(target: Any, name: String): String? =
        runCatching { target.javaClass.getField(name).get(target) as? String }.getOrNull()

    private fun stringMethod(target: Any, name: String): String? =
        runCatching { target.javaClass.getMethod(name).invoke(target) as? String }.getOrNull()

    private fun encode(value: String): String =
        URLEncoder.encode(value, StandardCharsets.UTF_8.name()).replace("+", "%20")
}
