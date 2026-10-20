import React from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";

import { setLocale } from "../i18n/i18n";
import { PROXY_REVEAL_MS, ProxyUrlField } from "./ProxyUrlField";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  setLocale("en");
});

function Harness(props: { initial?: string; onValue?: (v: string) => void }) {
  const [value, setValue] = React.useState(props.initial ?? "");
  return (
    <ProxyUrlField
      value={value}
      onChange={(v) => {
        setValue(v);
        props.onValue?.(v);
      }}
      ariaLabel="Proxy"
      inputClassName="settings-input"
      rowClassName="settings-key-row"
      buttonClassName="settings-key-toggle"
      dataTestId="proxy"
    />
  );
}

const input = () => screen.getByTestId("proxy") as HTMLInputElement;

// The reported field: http://vlasov:<password with @>@vm-squid3...:3128. Typed
// character by character, the password is dots except the last character, which
// stays readable for three seconds; the value handed up is the real one.
test("typing a password hides it, showing the last character for three seconds", () => {
  let latest = "";
  render(<Harness onValue={(v) => (latest = v)} />);
  const target = "http://vlasov:p@ss@vm-squid3.corp:3128";
  input().focus();
  for (let i = 1; i <= target.length; i++) {
    fireEvent.change(input(), { target: { value: input().value + target[i - 1] } });
  }
  expect(latest).toBe(target);
  expect(input().value).toBe("http://vlasov:••••@vm-squid3.corp:3128");

  // Type into the password: the new character shows, then hides.
  fireEvent.change(input(), {
    target: { value: "http://vlasov:••••X@vm-squid3.corp:3128" },
  });
  expect(latest).toBe("http://vlasov:p@ssX@vm-squid3.corp:3128");
  expect(input().value).toBe("http://vlasov:••••X@vm-squid3.corp:3128");
  act(() => {
    vi.advanceTimersByTime(PROXY_REVEAL_MS - 100);
  });
  expect(input().value).toContain("X");
  act(() => {
    vi.advanceTimersByTime(200);
  });
  expect(input().value).toBe("http://vlasov:•••••@vm-squid3.corp:3128");
});

test("a saved value is shown with the password hidden, and leaving the field hides it at once", () => {
  render(<Harness initial="http://vlasov:secret@proxy:3128" />);
  expect(input().value).toBe("http://vlasov:••••••@proxy:3128");
  input().focus();
  fireEvent.change(input(), { target: { value: "http://vlasov:••••••Z@proxy:3128" } });
  expect(input().value).toContain("Z");
  fireEvent.blur(input());
  expect(input().value).toBe("http://vlasov:•••••••@proxy:3128");
});

test("a proxy without a password is shown as typed", () => {
  render(<Harness initial="socks5h://127.0.0.1:1080" />);
  expect(input().value).toBe("socks5h://127.0.0.1:1080");
});

test("the … button builds the URL from separate fields, encoding the password", () => {
  let latest = "";
  render(<Harness initial="http://vlasov:old@vm-squid3.corp:3128" onValue={(v) => (latest = v)} />);
  fireEvent.click(screen.getByTestId("proxy-edit"));

  const dialog = screen.getByTestId("proxy-editor");
  expect((screen.getByTestId("proxy-editor-host") as HTMLInputElement).value).toBe(
    "vm-squid3.corp",
  );
  expect((screen.getByTestId("proxy-editor-port") as HTMLInputElement).value).toBe("3128");
  expect((screen.getByTestId("proxy-editor-user") as HTMLInputElement).value).toBe("vlasov");
  expect((screen.getByTestId("proxy-editor-password") as HTMLInputElement).value).toBe("old");

  fireEvent.change(screen.getByTestId("proxy-editor-password"), {
    target: { value: "p@ss/#" },
  });
  fireEvent.change(screen.getByTestId("proxy-editor-scheme"), {
    target: { value: "socks5h" },
  });
  // The preview keeps the password hidden too - all 12 characters of its
  // encoded form, p%40ss%2F%23.
  expect(screen.getByTestId("proxy-editor-result").textContent).toBe(
    "socks5h://vlasov:••••••••••••@vm-squid3.corp:3128",
  );
  fireEvent.submit(dialog);
  expect(latest).toBe("socks5h://vlasov:p%40ss%2F%23@vm-squid3.corp:3128");
  expect(screen.queryByTestId("proxy-editor")).toBeNull();
  expect(input().value).toBe("socks5h://vlasov:••••••••••••@vm-squid3.corp:3128");
});

test("the editor refuses a bad port, needs a host, and can clear the proxy", () => {
  let latest = "unchanged";
  render(<Harness initial="" onValue={(v) => (latest = v)} />);
  fireEvent.click(screen.getByTestId("proxy-edit"));

  fireEvent.submit(screen.getByTestId("proxy-editor"));
  expect(screen.getByRole("alert").textContent).toBe("Enter the proxy host.");
  expect(latest).toBe("unchanged");

  fireEvent.change(screen.getByTestId("proxy-editor-host"), { target: { value: "proxy" } });
  fireEvent.change(screen.getByTestId("proxy-editor-port"), { target: { value: "70000" } });
  expect(screen.getByRole("alert").textContent).toBe("The port is a number from 1 to 65535.");
  expect((screen.getByTestId("proxy-editor-apply") as HTMLButtonElement).disabled).toBe(true);

  fireEvent.click(screen.getByRole("button", { name: "No proxy" }));
  expect(latest).toBe("");
});

test("Escape closes the editor without changing the value", () => {
  let latest = "unchanged";
  render(<Harness initial="http://proxy:3128" onValue={(v) => (latest = v)} />);
  fireEvent.click(screen.getByTestId("proxy-edit"));
  fireEvent.change(screen.getByTestId("proxy-editor-host"), { target: { value: "other" } });
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.queryByTestId("proxy-editor")).toBeNull();
  expect(latest).toBe("unchanged");
});

test("the editor speaks Russian", () => {
  setLocale("ru");
  render(<Harness initial="" />);
  expect(screen.getByTestId("proxy-edit").getAttribute("aria-label")).toBe(
    "Настроить прокси: протокол, хост, порт, логин и пароль",
  );
  fireEvent.click(screen.getByTestId("proxy-edit"));
  expect(screen.getByRole("button", { name: "Применить" })).toBeTruthy();
  expect(screen.getByText("Пароль")).toBeTruthy();
});
