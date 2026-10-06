import { describe, expect, it } from "vitest";
import {
  applyMaskedEdit,
  buildProxyUrl,
  maskProxyValue,
  parseProxyUrl,
  passwordSpan,
  PROXY_MASK_CHAR,
} from "./proxyUrl";

const dots = (n: number) => PROXY_MASK_CHAR.repeat(n);

describe("passwordSpan", () => {
  it("runs from the first colon after the scheme to the last @", () => {
    expect(passwordSpan("http://vlasov:p@ss@vm-squid3:3128")).toEqual([14, 18]);
    expect(passwordSpan("http://vlasov:secret@host:3128")).toEqual([14, 20]);
  });
  it("is empty without credentials or without a password", () => {
    expect(passwordSpan("http://proxy.corp:3128")).toBeNull();
    expect(passwordSpan("socks5h://127.0.0.1:1080")).toBeNull();
    expect(passwordSpan("http://token@proxy:3128")).toBeNull();
    expect(passwordSpan("")).toBeNull();
  });
  it("masks a password being typed before the @ arrives", () => {
    // "vlasov:se" cannot be a host and port: the password starts after the colon.
    expect(passwordSpan("http://vlasov:se")).toEqual([14, 16]);
    // A port being typed stays visible.
    expect(passwordSpan("http://proxy.corp:31")).toBeNull();
    expect(passwordSpan("http://proxy.corp:3128/")).toBeNull();
  });
});

describe("maskProxyValue", () => {
  it("hides every password character and keeps the length", () => {
    const v = "http://vlasov:p@ss@vm-squid3:3128";
    const m = maskProxyValue(v);
    expect(m).toBe(`http://vlasov:${dots(4)}@vm-squid3:3128`);
    expect(m.length).toBe(v.length);
  });
  it("can leave one character visible", () => {
    expect(maskProxyValue("http://u:abc@h:1", 10)).toBe(
      `http://u:${dots(1)}b${dots(1)}@h:1`,
    );
  });
  it("leaves a URL without a password alone", () => {
    expect(maskProxyValue("http://proxy.corp:3128")).toBe("http://proxy.corp:3128");
  });
});

describe("applyMaskedEdit", () => {
  const real = "http://u:abc@h:1";
  const shown = maskProxyValue(real);

  it("inserts a typed character in the middle of the hidden password", () => {
    // Caret after "a" (index 10), type "X".
    const next = shown.slice(0, 10) + "X" + shown.slice(10);
    const r = applyMaskedEdit(real, shown, next, [10, 10], 11);
    expect(r.value).toBe("http://u:aXbc@h:1");
    expect(r.caret).toBe(11);
    expect(r.typedAt).toBe(10);
  });
  it("deletes the right hidden character with Backspace", () => {
    // Caret after "b" (index 11); the browser removes the dot before it.
    const next = shown.slice(0, 10) + shown.slice(11);
    const r = applyMaskedEdit(real, shown, next, [11, 11], 10);
    expect(r.value).toBe("http://u:ac@h:1");
    expect(r.typedAt).toBeNull();
  });
  it("deletes the right hidden character with Delete", () => {
    const next = shown.slice(0, 9) + shown.slice(10);
    const r = applyMaskedEdit(real, shown, next, [9, 9], 9);
    expect(r.value).toBe("http://u:bc@h:1");
  });
  it("replaces a selection with pasted text", () => {
    // Select the whole password and paste "p@ss/#".
    const next = shown.slice(0, 9) + "p@ss/#" + shown.slice(12);
    const r = applyMaskedEdit(real, shown, next, [9, 12], 15);
    expect(r.value).toBe("http://u:p@ss/#@h:1");
    expect(r.typedAt).toBeNull();
  });
  it("ignores a caret that did not follow the edit", () => {
    // A programmatic change leaves the caret at the end although the character
    // went in the middle; taking the caret at its word would append it.
    const next = shown.slice(0, 12) + "X" + shown.slice(12);
    const r = applyMaskedEdit(real, shown, next, [16, 16], next.length);
    expect(r.value).toBe("http://u:abcX@h:1");
  });
  it("falls back to a diff when the caret is unknown", () => {
    const next = "http://u:" + dots(3) + "d@h:1";
    expect(applyMaskedEdit(real, shown, next).value).toBe("http://u:abcd@h:1");
    expect(applyMaskedEdit("", "", "http://a:b@c:1").value).toBe("http://a:b@c:1");
  });
});

describe("parseProxyUrl / buildProxyUrl", () => {
  it("reads a URL typed with a raw @ in the password", () => {
    expect(parseProxyUrl("http://vlasov:p@ss@vm-squid3.corp:3128")).toEqual({
      scheme: "http",
      host: "vm-squid3.corp",
      port: "3128",
      user: "vlasov",
      password: "p@ss",
    });
  });
  it("decodes an encoded one and keeps a broken escape as typed", () => {
    expect(parseProxyUrl("socks5h://u%20x:%D0%BF%40%2F@[::1]:1080/")).toEqual({
      scheme: "socks5h",
      host: "::1",
      port: "1080",
      user: "u x",
      password: "п@/",
    });
    expect(parseProxyUrl("http://u:100%@h").password).toBe("100%");
  });
  it("fills defaults for an empty field", () => {
    expect(parseProxyUrl("")).toEqual({
      scheme: "http",
      host: "",
      port: "",
      user: "",
      password: "",
    });
  });
  it("encodes every character a password can carry", () => {
    for (const password of ["p@ss", "a:b", "/?#%", "пароль", "sp ace", "()*!'~"]) {
      const url = buildProxyUrl({
        scheme: "http",
        host: "vm-squid3.corp",
        port: "3128",
        user: "vlasov",
        password,
      });
      expect(url.startsWith("http://vlasov:")).toBe(true);
      expect(url.endsWith("@vm-squid3.corp:3128")).toBe(true);
      // Exactly one @ left in the result: the separator.
      expect(url.split("@").length).toBe(2);
      expect(parseProxyUrl(url).password).toBe(password);
    }
  });
  it("builds the plain forms", () => {
    expect(
      buildProxyUrl({ scheme: "socks5h", host: "127.0.0.1", port: "1080", user: "", password: "" }),
    ).toBe("socks5h://127.0.0.1:1080");
    expect(
      buildProxyUrl({ scheme: "http", host: "::1", port: "3128", user: "t", password: "" }),
    ).toBe("http://t@[::1]:3128");
    expect(
      buildProxyUrl({ scheme: "http", host: " ", port: "", user: "", password: "" }),
    ).toBe("");
  });
});
