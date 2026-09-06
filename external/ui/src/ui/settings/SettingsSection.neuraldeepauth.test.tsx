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
  // The endpoint picker keeps its slot above the sign-in block.
  const base = screen.getByLabelText("API base URL") as HTMLSelectElement;
  expect(base.tagName).toBe("SELECT");
  // The status is read for the endpoint the picker shows (the default when
  // none is stored).
  expect(fetchMock).toHaveBeenCalledWith(
    "/foxxycode/providers/neuraldeep/neuraldeep-auth?api_base=" +
      encodeURIComponent("https://api.neuraldeep.ru/v1"),
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

test("NeuralDeep Sign In carries the endpoint picked in the form", async () => {
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") {
        return {
          ok: true,
          json: async () => ({
            login_id: "login-mirror",
            verification_url:
              "https://hub.neuraldeep.tech/app/device?code=MRRR-0001",
            user_code: "MRRR-0001",
            status: "pending",
          }),
        };
      }
      if (String(input).includes("/device/")) {
        return {
          ok: true,
          json: async () => ({ status: "pending", connected: false }),
        };
      }
      return {
        ok: true,
        json: async () => ({ connected: false, source: "none" }),
      };
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  vi.spyOn(window, "open").mockImplementation(() => null);

  render(<Harness />);
  fireEvent.click(screen.getByTestId("settings-master-item-0"));
  fireEvent.change(await screen.findByTestId("neuraldeep-api-base"), {
    target: { value: "https://api.neuraldeep.tech/v1" },
  });
  fireEvent.click(await screen.findByTestId("neuraldeep-auth-sign-in"));
  expect(await screen.findByText("MRRR-0001")).toBeInTheDocument();

  // The pick has not been saved, so the device start must carry it: the hub
  // that mints the key is decided by the endpoint, not by the saved row.
  const start = fetchMock.mock.calls.find(
    ([, init]) => (init as RequestInit | undefined)?.method === "POST",
  );
  expect(start?.[0]).toBe(
    "/foxxycode/providers/neuraldeep/neuraldeep-auth/device",
  );
  expect(JSON.parse(String((start?.[1] as RequestInit).body))).toEqual({
    api_base: "https://api.neuraldeep.tech/v1",
  });
});

test("NeuralDeep flags a stored login issued by the other deployment's hub", async () => {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    const forMirror = url.includes(
      encodeURIComponent("https://api.neuraldeep.tech/v1"),
    );
    return {
      ok: true,
      json: async () => ({
        connected: true,
        masked: "sk-nd…4321",
        source: "oauth",
        hub: "https://hub.neuraldeep.ru",
        endpoint_hub: forMirror
          ? "https://hub.neuraldeep.tech"
          : "https://hub.neuraldeep.ru",
      }),
    };
  });
  vi.stubGlobal("fetch", fetchMock);

  render(<Harness />);
  fireEvent.click(screen.getByTestId("settings-master-item-0"));
  expect(
    await screen.findByText(/Signed in to NeuralDeep \(sk-nd…4321\)/),
  ).toBeInTheDocument();
  // Login and endpoint agree: no complaint.
  expect(screen.queryByTestId("neuraldeep-auth-hub-mismatch")).toBeNull();

  // Picking the mirror re-reads the status for that endpoint and, since the
  // stored key came from the default hub, says the login will not be honored.
  fireEvent.change(screen.getByTestId("neuraldeep-api-base"), {
    target: { value: "https://api.neuraldeep.tech/v1" },
  });
  const note = await screen.findByTestId("neuraldeep-auth-hub-mismatch");
  expect(note).toHaveTextContent("https://hub.neuraldeep.ru");
  expect(note).toHaveTextContent("https://api.neuraldeep.tech/v1");
  expect(fetchMock).toHaveBeenCalledWith(
    "/foxxycode/providers/neuraldeep/neuraldeep-auth?api_base=" +
      encodeURIComponent("https://api.neuraldeep.tech/v1"),
    expect.anything(),
  );
});

test("NeuralDeep keeps polling a pending login when the endpoint changes", async () => {
  let polls = 0;
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (init?.method === "POST") {
        return {
          ok: true,
          json: async () => ({
            login_id: "login-pending",
            verification_url: "https://hub.neuraldeep.test/app/device?code=PEND-0001",
            user_code: "PEND-0001",
            status: "pending",
          }),
        };
      }
      if (url.includes("/device/login-pending")) {
        polls += 1;
        return {
          ok: true,
          json: async () => ({ status: "pending", connected: false }),
        };
      }
      return {
        ok: true,
        json: async () => ({ connected: false, source: "none" }),
      };
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  vi.spyOn(window, "open").mockImplementation(() => null);

  render(<Harness />);
  fireEvent.click(screen.getByTestId("settings-master-item-0"));
  fireEvent.click(await screen.findByTestId("neuraldeep-auth-sign-in"));
  expect(await screen.findByText("PEND-0001")).toBeInTheDocument();
  await waitFor(() => expect(polls).toBeGreaterThan(0), { timeout: 2000 });

  // The endpoint pick changes while the hub wait is still running: the code
  // stays on screen and the poll goes on, instead of the widget forgetting
  // the login and sitting in "Waiting for NeuralDeep…" forever.
  const before = polls;
  fireEvent.change(screen.getByTestId("neuraldeep-api-base"), {
    target: { value: "https://api.neuraldeep.tech/v1" },
  });
  expect(screen.getByText("PEND-0001")).toBeInTheDocument();
  await waitFor(() => expect(polls).toBeGreaterThan(before), {
    timeout: 3000,
  });
  expect(screen.getByText("PEND-0001")).toBeInTheDocument();
});

