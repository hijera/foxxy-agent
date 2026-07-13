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

    /** Mirrors the fields/methods ProxyEnvironment reads reflectively off HttpConfigurable. */
    @Suppress("PropertyName")
    class FakeConfigurable(
        @JvmField var USE_HTTP_PROXY: Boolean = false,
        @JvmField var PROXY_HOST: String = "",
        @JvmField var PROXY_PORT: Int = 0,
        @JvmField var PROXY_AUTHENTICATION: Boolean = false,
        @JvmField var proxyLogin: String = "",
        @JvmField var PROXY_EXCEPTIONS: String = "",
        private val plainPassword: String = "",
    ) {
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

    @Test
    fun `credentials are url-encoded into the authority`() {
        val env = ProxyEnvironment.buildProxyEnv(
            FakeConfigurable(
                USE_HTTP_PROXY = true,
                PROXY_HOST = "proxy.local",
                PROXY_PORT = 3128,
                PROXY_AUTHENTICATION = true,
                proxyLogin = "user name",
                plainPassword = "p@ss:word/",
            ),
        )
        assertEquals("http://user%20name:p%40ss%3Aword%2F@proxy.local:3128", env["HTTP_PROXY"])
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
}
