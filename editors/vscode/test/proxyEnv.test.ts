import { describe, expect, it } from "vitest";
import { proxyEnvFrom, proxyEnvFromSetting, withProxyEnv } from "../src/process/proxyEnv";

describe("proxyEnvFromSetting", () => {
  it("maps VS Code http.proxy into standard proxy environment variables", () => {
    expect(proxyEnvFromSetting("http://user:pass@proxy.local:8080")).toEqual({
      HTTP_PROXY: "http://user:pass@proxy.local:8080/",
      HTTPS_PROXY: "http://user:pass@proxy.local:8080/",
      ALL_PROXY: "http://user:pass@proxy.local:8080/",
      http_proxy: "http://user:pass@proxy.local:8080/",
      https_proxy: "http://user:pass@proxy.local:8080/",
      all_proxy: "http://user:pass@proxy.local:8080/",
    });
  });

  it("accepts host:port shorthand", () => {
    expect(proxyEnvFromSetting("proxy.local:3128").HTTP_PROXY).toBe("http://proxy.local:3128/");
  });

  it("ignores blank or unsupported proxy settings", () => {
    expect(proxyEnvFromSetting("")).toEqual({});
    expect(proxyEnvFromSetting("socks5://proxy.local:1080")).toEqual({});
  });
});

describe("proxyEnvFrom (http.noProxy)", () => {
  it("emits NO_PROXY/no_proxy from the exceptions list alongside the proxy URL", () => {
    const env = proxyEnvFrom("http://proxy.local:8080", ["localhost", "127.0.0.1", ".internal"]);
    expect(env.HTTP_PROXY).toBe("http://proxy.local:8080/");
    expect(env.NO_PROXY).toBe("localhost,127.0.0.1,.internal");
    expect(env.no_proxy).toBe("localhost,127.0.0.1,.internal");
  });

  it("trims and drops blank no-proxy entries", () => {
    const env = proxyEnvFrom("proxy.local:3128", ["  localhost  ", "", "   ", "example.com"]);
    expect(env.NO_PROXY).toBe("localhost,example.com");
  });

  it("omits NO_PROXY when there is no proxy URL", () => {
    expect(proxyEnvFrom("", ["localhost"])).toEqual({});
  });

  it("omits NO_PROXY when the exceptions list is empty", () => {
    const env = proxyEnvFrom("http://proxy.local:8080", []);
    expect(env.NO_PROXY).toBeUndefined();
    expect(env.no_proxy).toBeUndefined();
    expect(env.HTTP_PROXY).toBe("http://proxy.local:8080/");
  });
});

describe("withProxyEnv", () => {
  it("does not mutate the base environment", () => {
    const base = { PATH: "bin" };
    const merged = withProxyEnv(base, { HTTP_PROXY: "http://proxy.local:8080/" });

    expect(merged).toEqual({ PATH: "bin", HTTP_PROXY: "http://proxy.local:8080/" });
    expect(base).toEqual({ PATH: "bin" });
  });
});
