import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { SettingsSection } from "./SettingsSection";
import type { JsonSchema } from "./SchemaForm";
import type { SectionDescriptor } from "./settingsSections";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const providersSection: SectionDescriptor = {
  id: "providers",
  label: "LLM providers",
  kind: "array",
  schemaKey: "providers",
  labelField: "name",
};

const rootSchema: JsonSchema = {
  type: "object",
  properties: {
    providers: {
      type: "array",
      title: "LLM providers",
      items: {
        type: "object",
        properties: {
          name: { type: "string", title: "Provider name" },
          type: {
            type: "string",
            title: "Provider type",
            enum: ["openai", "anthropic", "neuraldeep", "codex"],
          },
          api_base: { type: "string", title: "API base URL" },
          api_key: { type: "string", title: "API key" },
          api_key_command: { type: "string", title: "API key command" },
          proxy: { type: "string", title: "Proxy URL" },
        },
        "x-foxxycode-property-order": [
          "name",
          "type",
          "api_base",
          "api_key",
          "api_key_command",
          "proxy",
        ],
      },
    },
  },
};

function Harness(props: {
  provider?: Record<string, unknown>;
  /** Sees every document the section writes. */
  onDoc?: (doc: Record<string, unknown>) => void;
}) {
  const [doc, setDoc] = React.useState<Record<string, unknown>>({
    providers: [
      props.provider ?? {
        name: "neuraldeep",
        type: "neuraldeep",
        api_base: "",
        api_key: "",
      },
    ],
  });
  return (
    <SettingsSection
      section={providersSection}
      schema={rootSchema}
      doc={doc}
      setDoc={(next) => {
        props.onDoc?.(next);
        setDoc(next);
      }}
    />
  );
}

/** The proxy setting of the first provider row of a settings document. */
function firstProxy(doc: Record<string, unknown>): unknown {
  const rows = doc.providers as Record<string, unknown>[] | undefined;
  return rows?.[0]?.proxy;
}

test("the Ignore system proxy switch saves none and brings the URL back when turned off", () => {
  let doc: Record<string, unknown> = {};
  render(
    <Harness
      provider={{
        name: "corp",
        type: "openai",
        proxy: "http://127.0.0.1:3128",
      }}
      onDoc={(next) => {
        doc = next;
      }}
    />,
  );
  fireEvent.click(screen.getByTestId("settings-master-item-0"));

  const direct = screen.getByRole("switch", { name: "Ignore system proxy" });
  const url = screen.getByLabelText("Proxy URL") as HTMLInputElement;
  expect(direct).toHaveAttribute("aria-checked", "false");
  expect(url.value).toBe("http://127.0.0.1:3128");

  fireEvent.click(direct);
  expect(firstProxy(doc)).toBe("none");
  expect(direct).toHaveAttribute("aria-checked", "true");
  expect(url).toBeDisabled();
  expect(url.placeholder).toBe("Direct connection");

  fireEvent.click(direct);
  expect(firstProxy(doc)).toBe("http://127.0.0.1:3128");
  expect(url).not.toBeDisabled();
  expect(url.value).toBe("http://127.0.0.1:3128");
});

test("a provider saved as none shows the switch on and follows the system proxy once it is off", () => {
  let doc: Record<string, unknown> = {};
  render(
    <Harness
      provider={{ name: "local", type: "openai", proxy: "none" }}
      onDoc={(next) => {
        doc = next;
      }}
    />,
  );
  fireEvent.click(screen.getByTestId("settings-master-item-0"));

  const direct = screen.getByRole("switch", { name: "Ignore system proxy" });
  const url = screen.getByLabelText("Proxy URL") as HTMLInputElement;
  expect(direct).toHaveAttribute("aria-checked", "true");
  expect(url).toBeDisabled();
  expect(url.value).toBe("");

  fireEvent.click(direct);
  expect(firstProxy(doc)).toBe("");
  expect(url.placeholder).toBe("Follows the system proxy");
});

test("a typed proxy URL replaces the system proxy and clearing it follows the system proxy again", () => {
  let doc: Record<string, unknown> = {};
  render(
    <Harness
      provider={{ name: "corp", type: "anthropic" }}
      onDoc={(next) => {
        doc = next;
      }}
    />,
  );
  fireEvent.click(screen.getByTestId("settings-master-item-0"));

  const url = screen.getByLabelText("Proxy URL") as HTMLInputElement;
  expect(url.value).toBe("");
  expect(url.placeholder).toBe("Follows the system proxy");

  fireEvent.change(url, { target: { value: "socks5h://127.0.0.1:1080" } });
  expect(firstProxy(doc)).toBe("socks5h://127.0.0.1:1080");
  expect(
    screen.getByRole("switch", { name: "Ignore system proxy" }),
  ).toHaveAttribute("aria-checked", "false");

  fireEvent.change(url, { target: { value: "" } });
  expect(firstProxy(doc)).toBe("");
});

test("an explicit inherit reads as the system proxy and survives a round trip through the switch", () => {
  let doc: Record<string, unknown> = {};
  render(
    <Harness
      provider={{ name: "corp", type: "openai", proxy: "Inherit" }}
      onDoc={(next) => {
        doc = next;
      }}
    />,
  );
  fireEvent.click(screen.getByTestId("settings-master-item-0"));

  const direct = screen.getByRole("switch", { name: "Ignore system proxy" });
  const url = screen.getByLabelText("Proxy URL") as HTMLInputElement;
  expect(direct).toHaveAttribute("aria-checked", "false");
  expect(url.value).toBe("");

  fireEvent.click(direct);
  expect(firstProxy(doc)).toBe("none");
  fireEvent.click(direct);
  expect(firstProxy(doc)).toBe("Inherit");
});

test("a Codex provider keeps the proxy setting next to its ChatGPT sign in", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({
      ok: true,
      json: async () => ({ connected: false, source: "" }),
    })),
  );
  render(<Harness provider={{ name: "codex", type: "codex", proxy: "none" }} />);
  fireEvent.click(screen.getByTestId("settings-master-item-0"));

  expect(await screen.findByTestId("codex-auth-sign-in")).toBeInTheDocument();
  expect(
    screen.getByRole("switch", { name: "Ignore system proxy" }),
  ).toHaveAttribute("aria-checked", "true");
});

test("turning the switch off after none was pasted into the field brings back the URL typed before it", () => {
  let doc: Record<string, unknown> = {};
  render(
    <Harness
      provider={{ name: "corp", type: "openai", proxy: "http://a:3128" }}
      onDoc={(next) => {
        doc = next;
      }}
    />,
  );
  fireEvent.click(screen.getByTestId("settings-master-item-0"));

  const url = screen.getByLabelText("Proxy URL") as HTMLInputElement;
  fireEvent.change(url, { target: { value: "http://b:3128" } });
  fireEvent.change(url, { target: { value: "none" } });
  const direct = screen.getByRole("switch", { name: "Ignore system proxy" });
  expect(direct).toHaveAttribute("aria-checked", "true");

  fireEvent.click(direct);
  expect(firstProxy(doc)).toBe("http://b:3128");
});

test("renaming the provider while it connects directly keeps the URL the switch brings back", () => {
  let doc: Record<string, unknown> = {};
  render(
    <Harness
      provider={{ name: "corp", type: "openai", proxy: "http://a:3128" }}
      onDoc={(next) => {
        doc = next;
      }}
    />,
  );
  fireEvent.click(screen.getByTestId("settings-master-item-0"));

  fireEvent.click(screen.getByRole("switch", { name: "Ignore system proxy" }));
  expect(firstProxy(doc)).toBe("none");
  fireEvent.change(screen.getByLabelText("Provider name"), {
    target: { value: "corp2" },
  });
  fireEvent.click(screen.getByRole("switch", { name: "Ignore system proxy" }));
  expect(firstProxy(doc)).toBe("http://a:3128");
});

const systemSection: SectionDescriptor = {
  id: "system",
  label: "System",
  kind: "group",
  childKeys: ["gateways"],
};

const gatewaysRootSchema: JsonSchema = {
  type: "object",
  properties: {
    gateways: {
      type: "object",
      title: "Messenger gateways",
      properties: {
        telegram: {
          type: "object",
          title: "Telegram",
          properties: {
            enable: { type: "boolean", title: "Enabled" },
            token: { type: "string", title: "Bot token" },
            proxy: { type: "string", title: "Proxy URL" },
          },
          "x-foxxycode-property-order": ["enable", "token", "proxy"],
        },
      },
    },
  },
} as unknown as JsonSchema;

function GatewaysHarness(props: {
  proxy?: string;
  onDoc: (doc: Record<string, unknown>) => void;
}) {
  const [doc, setDoc] = React.useState<Record<string, unknown>>({
    gateways: {
      telegram: {
        enable: true,
        token: "123:abc",
        ...(props.proxy === undefined ? {} : { proxy: props.proxy }),
      },
    },
  });
  return (
    <SettingsSection
      section={systemSection}
      schema={gatewaysRootSchema}
      doc={doc}
      setDoc={(next) => {
        props.onDoc(next);
        setDoc(next);
      }}
    />
  );
}

function telegramProxy(doc: Record<string, unknown>): unknown {
  const gateways = doc.gateways as Record<string, unknown> | undefined;
  const telegram = gateways?.telegram as Record<string, unknown> | undefined;
  return telegram?.proxy;
}

test("the Telegram proxy has the Ignore system proxy switch too", () => {
  let doc: Record<string, unknown> = {};
  render(
    <GatewaysHarness
      proxy="socks5h://127.0.0.1:1080"
      onDoc={(next) => {
        doc = next;
      }}
    />,
  );

  const direct = screen.getByRole("switch", { name: "Ignore system proxy" });
  const url = screen.getByLabelText("Proxy URL") as HTMLInputElement;
  expect(url.value).toBe("socks5h://127.0.0.1:1080");

  fireEvent.click(direct);
  expect(telegramProxy(doc)).toBe("none");
  expect(url).toBeDisabled();
  expect(document.body.textContent).toContain("Telegram bot ignores");

  fireEvent.click(direct);
  expect(telegramProxy(doc)).toBe("socks5h://127.0.0.1:1080");
});
