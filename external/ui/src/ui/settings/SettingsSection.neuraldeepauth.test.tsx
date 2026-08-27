import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
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
        },
        "x-foxxycode-property-order": [
          "name",
          "type",
          "api_base",
          "api_key",
          "api_key_command",
        ],
      },
    },
  },
};

function Harness(props: { provider?: Record<string, unknown> }) {
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
      setDoc={setDoc}
    />
  );
}

test("NeuralDeep provider keeps the manual api_key and offers hub sign in", async () => {
  const fetchMock = vi.fn(async () => ({
    ok: true,
    json: async () => ({ connected: false, source: "none" }),
  }));
  vi.stubGlobal("fetch", fetchMock);

  render(<Harness />);
  fireEvent.click(screen.getByTestId("settings-master-item-0"));

  // The manual key entry stays available; sign-in is an alternative, not a
  // replacement (an explicit key would win over the login).
  expect(screen.getByLabelText("API key")).toBeInTheDocument();
  expect(
    await screen.findByTestId("neuraldeep-auth-sign-in"),
  ).toHaveTextContent("Sign In with NeuralDeep");
  const base = screen.getByLabelText("API base URL") as HTMLInputElement;
  expect(base.readOnly).toBe(true);
  expect(fetchMock).toHaveBeenCalledWith(
    "/foxxycode/providers/neuraldeep/neuraldeep-auth",
    expect.anything(),
  );
});

test("NeuralDeep Sign In opens the hub and completes device authorization", async () => {
  let approved = false;
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (init?.method === "POST") {
        return {
          ok: true,
          json: async () => ({
            login_id: "login-nd",
            verification_url: "https://hub.neuraldeep.test/app/device?code=BCDF-2345",
            user_code: "BCDF-2345",
            status: "pending",
          }),
        };
      }
      if (url.endsWith("/device/login-nd")) {
        approved = true;
        return {
          ok: true,
          json: async () => ({ status: "completed", connected: true }),
        };
      }
      // The widget re-reads the stored status after completion so the masked
      // key is real, not a locally invented placeholder.
      return {
        ok: true,
        json: async () =>
          approved
            ? { connected: true, masked: "sk-nd…4321", source: "oauth" }
            : { connected: false, source: "none" },
      };
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  const openMock = vi.spyOn(window, "open").mockImplementation(() => null);

  render(<Harness />);
  fireEvent.click(screen.getByTestId("settings-master-item-0"));
  fireEvent.click(await screen.findByTestId("neuraldeep-auth-sign-in"));

  expect(await screen.findByText("BCDF-2345")).toBeInTheDocument();
  expect(openMock).toHaveBeenCalledWith(
    "https://hub.neuraldeep.test/app/device?code=BCDF-2345",
    "_blank",
    "noopener,noreferrer",
  );
  expect(
    await screen.findByText(/Signed in to NeuralDeep \(sk-nd…4321\)/, {}, { timeout: 2000 }),
  ).toBeInTheDocument();
});

test("NeuralDeep explicit api_key reports that it shadows the login", async () => {
  const fetchMock = vi.fn(async () => ({
    ok: true,
    json: async () => ({
      connected: true,
      masked: "sk-ab…1234",
      source: "api_key",
    }),
  }));
  vi.stubGlobal("fetch", fetchMock);

  render(
    <Harness
      provider={{
        name: "neuraldeep",
        type: "neuraldeep",
        api_base: "",
        api_key: "sk-manual",
      }}
    />,
  );
  fireEvent.click(screen.getByTestId("settings-master-item-0"));

  expect(
    await screen.findByTestId("neuraldeep-auth-shadowed"),
  ).toHaveTextContent("requests use it instead of this login");
  // The stored login is still displayed, masked.
  expect(screen.getByText(/sk-ab…1234/)).toBeInTheDocument();
});

const modelsSection: SectionDescriptor = {
  id: "models",
  label: "Logical models",
  kind: "array",
  schemaKey: "models",
  labelField: "model",
};

const modelsSchema: JsonSchema = {
  type: "object",
  properties: {
    models: {
      type: "array",
      title: "Logical models",
      items: {
        type: "object",
        properties: {
          model: { type: "string", title: "Model id" },
        },
        "x-foxxycode-property-order": ["model"],
      },
    },
  },
};
