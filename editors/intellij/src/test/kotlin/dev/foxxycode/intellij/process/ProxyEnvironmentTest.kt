package dev.foxxycode.intellij.process

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.InetSocketAddress
import java.net.Proxy

/**
 * Unit tests for [ProxyEnvironment.buildProxyEnv], driven through a fake object that mimics the
 * reflection contract of IntelliJ's `HttpConfigurable` (public fields + password getter). This
 * keeps the tests platform-free.
 */
class ProxyEnvironmentTest {

    /**
     * Mirrors the fields/methods ProxyEnvironment reads reflectively off HttpConfigurable. Matches the
     * real 222/223 contract: the proxy host/port/flags are public fields, but the login is exposed
     * *only* as getProxyLogin() — there is no `proxyLogin` field (verified against the SDK jars).
     */
    @Suppress("PropertyName")
    class FakeConfigurable(
        @JvmField var USE_HTTP_PROXY: Boolean = false,
        @JvmField var PROXY_HOST: String = "",
        @JvmField var PROXY_PORT: Int = 0,
        @JvmField var PROXY_AUTHENTICATION: Boolean = false,
        @JvmField var PROXY_EXCEPTIONS: String = "",
        private val login: String = "",
        private val plainPassword: String = "",
    ) {
        @Suppress("unused")
        fun getProxyLogin(): String = login

        @Suppress("unused")
        fun getPlainProxyPassword(): String = plainPassword
    }

    private val proxyKeys = listOf("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy")

    @Test
    fun `disabled proxy yields empty map`() {
        val env = ProxyEnvironment.buildProxyEnv(FakeConfigurable(USE_HTTP_PROXY = false, PROXY_HOST = "proxy.local"))
        assertTrue(env.isEmpty())
    }

    @Test
    fun `enabled proxy with blank host yields empty map`() {
        val env = ProxyEnvironment.buildProxyEnv(FakeConfigurable(USE_HTTP_PROXY = true, PROXY_HOST = "   "))
        assertTrue(env.isEmpty())
    }

    @Test
    fun `host and port populate every proxy variable`() {
        val env = ProxyEnvironment.buildProxyEnv(
            FakeConfigurable(USE_HTTP_PROXY = true, PROXY_HOST = "proxy.local", PROXY_PORT = 3128),
        )
        for (key in proxyKeys) {
            assertEquals("http://proxy.local:3128", env[key])
        }
        assertNull(env["NO_PROXY"])
    }

    /**
     * Regression: the login used to be read as a `proxyLogin` *field*, which does not exist on
     * HttpConfigurable (222/223). Credentials were silently dropped and an authenticating proxy
     * answered 407. It must be read from getProxyLogin().
     */
    @Test
    fun `credentials are url-encoded into the authority`() {
        val env = ProxyEnvironment.buildProxyEnv(
            FakeConfigurable(
                USE_HTTP_PROXY = true,
                PROXY_HOST = "proxy.local",
                PROXY_PORT = 3128,
                PROXY_AUTHENTICATION = true,
                login = "user name",
                plainPassword = "p@ss:word/",
            ),
        )
        assertEquals("http://user%20name:p%40ss%3Aword%2F@proxy.local:3128", env["HTTP_PROXY"])
    }

    @Test
    fun `credentials are omitted when proxy authentication is off`() {
        val env = ProxyEnvironment.buildProxyEnv(
            FakeConfigurable(
                USE_HTTP_PROXY = true,
                PROXY_HOST = "proxy.local",
                PROXY_PORT = 3128,
                PROXY_AUTHENTICATION = false,
                login = "user",
                plainPassword = "secret",
            ),
        )
        assertEquals("http://proxy.local:3128", env["HTTP_PROXY"])
    }

    @Test
    fun `ipv6 host is bracketed`() {
        val env = ProxyEnvironment.buildProxyEnv(
            FakeConfigurable(USE_HTTP_PROXY = true, PROXY_HOST = "::1", PROXY_PORT = 3128),
        )
        assertEquals("http://[::1]:3128", env["HTTP_PROXY"])
    }

    @Test
    fun `proxy exceptions are normalized into NO_PROXY`() {
        val env = ProxyEnvironment.buildProxyEnv(
            FakeConfigurable(
                USE_HTTP_PROXY = true,
                PROXY_HOST = "proxy.local",
                PROXY_PORT = 3128,
                PROXY_EXCEPTIONS = "localhost, 127.0.0.1;  .internal",
            ),
        )
        assertEquals("localhost,127.0.0.1,.internal", env["NO_PROXY"])
        assertEquals("localhost,127.0.0.1,.internal", env["no_proxy"])
    }

    // --- resolvedProxyUrl (auto-detect / PAC) ---

    private fun httpProxy(host: String, port: Int) =
        Proxy(Proxy.Type.HTTP, InetSocketAddress.createUnresolved(host, port))

    @Test
    fun `resolved HTTP proxy becomes an http url`() {
        val url = ProxyEnvironment.resolvedProxyUrl(listOf(httpProxy("vm-squid3.rosenergo.com", 3128)), "")
        assertEquals("http://vm-squid3.rosenergo.com:3128", url)
    }

    @Test
    fun `resolved SOCKS proxy becomes a socks5 url`() {
        val socks = Proxy(Proxy.Type.SOCKS, InetSocketAddress.createUnresolved("127.0.0.1", 1080))
        assertEquals("socks5://127.0.0.1:1080", ProxyEnvironment.resolvedProxyUrl(listOf(socks), ""))
    }

    @Test
    fun `resolved proxy carries the auth prefix`() {
        val url = ProxyEnvironment.resolvedProxyUrl(listOf(httpProxy("proxy.local", 3128)), "user%20name:p%40ss@")
        assertEquals("http://user%20name:p%40ss@proxy.local:3128", url)
    }

    @Test
    fun `ipv6 resolved host is bracketed`() {
        assertEquals("http://[::1]:3128", ProxyEnvironment.resolvedProxyUrl(listOf(httpProxy("::1", 3128)), ""))
    }

    @Test
    fun `DIRECT-only selection resolves to null`() {
        assertNull(ProxyEnvironment.resolvedProxyUrl(listOf(Proxy.NO_PROXY), ""))
    }

    @Test
    fun `empty selection resolves to null`() {
        assertNull(ProxyEnvironment.resolvedProxyUrl(emptyList(), ""))
    }

    @Test
    fun `first non-direct proxy wins`() {
        val url = ProxyEnvironment.resolvedProxyUrl(
            listOf(Proxy.NO_PROXY, httpProxy("first.proxy", 8080), httpProxy("second.proxy", 9090)),
            "",
        )
        assertEquals("http://first.proxy:8080", url)
    }

    // --- normalizeNoProxy (loopback always excluded) ---

    @Test
    fun `normalizeNoProxy without exceptions still excludes loopback`() {
        assertEquals("localhost,127.0.0.1,::1", ProxyEnvironment.normalizeNoProxy(null))
        assertEquals("localhost,127.0.0.1,::1", ProxyEnvironment.normalizeNoProxy("   "))
    }

    @Test
    fun `normalizeNoProxy prepends loopback and normalizes separators`() {
        assertEquals(
            "localhost,127.0.0.1,::1,.internal,10.0.0.0/8",
            ProxyEnvironment.normalizeNoProxy(".internal, 10.0.0.0/8"),
        )
    }

    @Test
    fun `normalizeNoProxy dedupes loopback the user also listed`() {
        assertEquals(
            "localhost,127.0.0.1,::1,corp.example",
            ProxyEnvironment.normalizeNoProxy("localhost; corp.example; 127.0.0.1"),
        )
    }

    // --- buildEnvFromResolved (ProxySelector path) ---

    @Test
    fun `buildEnvFromResolved fills every proxy and no_proxy variable`() {
        val env = ProxyEnvironment.buildEnvFromResolved("http://proxy.local:3128", ".internal")
        for (key in proxyKeys) assertEquals("http://proxy.local:3128", env[key])
        assertEquals("localhost,127.0.0.1,::1,.internal", env["NO_PROXY"])
        assertEquals("localhost,127.0.0.1,::1,.internal", env["no_proxy"])
    }

    @Test
    fun `buildEnvFromResolved forwards a socks url and still excludes loopback`() {
        val env = ProxyEnvironment.buildEnvFromResolved("socks5://127.0.0.1:1080", null)
        assertEquals("socks5://127.0.0.1:1080", env["ALL_PROXY"])
        assertEquals("localhost,127.0.0.1,::1", env["NO_PROXY"])
    }

    // --- log masking (credentials must never reach the IDE log) ---

    @Test
    fun `password is masked in the logged proxy url`() {
        assertEquals(
            "http://user%20name:***@proxy.local:3128",
            ProxyEnvironment.maskProxyPassword("http://user%20name:p%40ss@proxy.local:3128"),
        )
    }

    @Test
    fun `url without credentials is left alone`() {
        assertEquals(
            "http://proxy.local:3128",
            ProxyEnvironment.maskProxyPassword("http://proxy.local:3128"),
        )
    }

    @Test
    fun `describe masks the password and names the source`() {
        val resolved = ProxyEnvironment.Resolved(
            "legacy-manual",
            mapOf("HTTP_PROXY" to "http://u:secret@proxy.local:3128", "NO_PROXY" to "localhost"),
        )
        val line = ProxyEnvironment.describe(resolved)
        assertTrue(line.contains("source=legacy-manual"))
        assertTrue(line.contains("http://u:***@proxy.local:3128"))
        assertTrue("password leaked into the log line", !line.contains("secret"))
    }

    @Test
    fun `describe reports no proxy when none was resolved`() {
        val line = ProxyEnvironment.describe(ProxyEnvironment.Resolved("legacy-none", emptyMap()))
        assertTrue(line.contains("source=legacy-none"))
        assertTrue(line.contains("HTTP_PROXY=<none>"))
    }
}
