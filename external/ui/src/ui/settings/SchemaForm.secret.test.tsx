import React from "react";
import { afterEach, expect, test } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { SchemaForm, type JsonSchema } from "./SchemaForm";
import { setLocale } from "../i18n/i18n";

afterEach(() => {
  cleanup();
  setLocale("en");
});

// Mirrors the api_key provider field produced by the Go UISchemaMap(): a secret
// string field carrying x-foxxycode-secret, whose description is translated by the
// Russian schema overlay.
const secretSchema: JsonSchema = {
  type: "object",
  properties: {
    api_key: {
      type: "string",
      title: "API key",
      description:
        "You may set a literal key, reference ${ENV} in YAML (expanded when the file is loaded), or leave empty so the process reads the conventional NAME_API_KEY variable derived from the provider name (see provider name description).",
      "x-foxxycode-secret": true,
    } as JsonSchema,
  },
} as unknown as JsonSchema;

test("secret string field masks by default and reveals on toggle", () => {
  function Harness() {
    const [doc, setDoc] = React.useState<Record<string, unknown>>({});
    return <SchemaForm schema={secretSchema} value={doc} onChange={setDoc} />;
  }
  render(<Harness />);

  const input = screen.getByLabelText("API key") as HTMLInputElement;
  // Masked by default.
  expect(input.type).toBe("password");

  fireEvent.change(input, { target: { value: "sk-secret" } });
  expect((screen.getByLabelText("API key") as HTMLInputElement).value).toBe(
    "sk-secret",
  );

  // Toggle reveals, then hides again.
  fireEvent.click(screen.getByRole("button", { name: "Show" }));
  expect((screen.getByLabelText("API key") as HTMLInputElement).type).toBe("text");
  fireEvent.click(screen.getByRole("button", { name: "Hide" }));
  expect((screen.getByLabelText("API key") as HTMLInputElement).type).toBe(
    "password",
  );
});

test("string field description renders the Russian overlay under ru locale", () => {
  setLocale("ru");
  function Harness() {
    const [doc, setDoc] = React.useState<Record<string, unknown>>({});
    return <SchemaForm schema={secretSchema} value={doc} onChange={setDoc} />;
  }
  render(<Harness />);

  // Regression for the bug where the string branch rendered raw English
  // schema.description instead of the translated desc.
  expect(
    screen.getByText(/Можно задать ключ напрямую/),
  ).toBeTruthy();
  expect(screen.queryByText(/You may set a literal key/)).toBeNull();
});

// Both proxy fields (providers[].proxy and gateways.telegram.proxy) carry
// x-foxxycode-proxy-url in the Go UISchemaMap(): the form hides the password in
// them and offers the proxy editor.
test("a proxy URL field hides the password and offers the editor", () => {
  const proxySchema = {
    type: "object",
    properties: {
      proxy: {
        type: "string",
        title: "HTTP or SOCKS proxy",
        "x-foxxycode-proxy-url": true,
      },
    },
  } as unknown as JsonSchema;
  let latest: Record<string, unknown> = {};
  function Harness() {
    const [doc, setDoc] = React.useState<Record<string, unknown>>({
      proxy: "http://vlasov:p@ss@vm-squid3.corp:3128",
    });
    return (
      <SchemaForm
        schema={proxySchema}
        value={doc}
        onChange={(v) => {
          latest = v as Record<string, unknown>;
          setDoc(v as Record<string, unknown>);
        }}
      />
    );
  }
  render(<Harness />);
  const input = screen.getByLabelText("HTTP or SOCKS proxy") as HTMLInputElement;
  expect(input.type).toBe("text");
  expect(input.value).toBe("http://vlasov:••••@vm-squid3.corp:3128");

  fireEvent.click(
    screen.getByRole("button", {
      name: "Set up the proxy: protocol, host, port, login and password",
    }),
  );
  fireEvent.change(screen.getByTestId("proxy-editor-password"), {
    target: { value: "new@pw" },
  });
  fireEvent.submit(screen.getByTestId("proxy-editor"));
  expect(latest.proxy).toBe("http://vlasov:new%40pw@vm-squid3.corp:3128");
});
