import {
  render,
  screen,
  fireEvent,
  waitFor,
  cleanup,
} from "@testing-library/react";
import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { I18nProvider } from "../i18n/I18nProvider";
import { initLocale } from "../i18n/i18n";
import { ProviderPickerDialog } from "./ProviderPickerDialog";
import { shouldShowOnboarding } from "./onboardingStatus";

function renderPicker(props: { onSaved?: () => void; onSkip?: () => void }) {
  return render(
    <I18nProvider>
      <ProviderPickerDialog
        open
        onSaved={props.onSaved ?? (() => {})}
        onSkip={props.onSkip ?? (() => {})}
      />
    </I18nProvider>,
  );
}

describe("shouldShowOnboarding", () => {
  it("returns true when first_run", () => {
    expect(
      shouldShowOnboarding({
        first_run: true,
        has_config: false,
        has_providers: false,
        has_models: false,
        has_agent_model: false,
        missing_api_keys: [],
      }),
    ).toBe(true);
  });

  it("returns false when fully configured", () => {
    expect(
      shouldShowOnboarding({
        first_run: false,
        has_config: true,
        has_providers: true,
        has_models: true,
        has_agent_model: true,
        missing_api_keys: [],
      }),
    ).toBe(false);
  });
});

describe("ProviderPickerDialog", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    initLocale("en");
    fetchMock.mockClear();
    fetchMock.mockImplementation(async (url: string, init?: RequestInit) => {
      if (url === "/foxxycode/config" && (!init || init.method === undefined)) {
        return new Response(JSON.stringify({ agent: { max_turns: 30 } }), {
          status: 200,
        });
      }
      if (url === "/foxxycode/config/validate") {
        return new Response(JSON.stringify({ ok: true }), { status: 200 });
      }
      if (url === "/foxxycode/config" && init?.method === "PUT") {
        return new Response(JSON.stringify({ ok: true }), { status: 200 });
      }
      if (url === "/v1/models") {
        // Real shape: synthetic agent/plan/docs/ask pseudo-models (owned_by
        // "foxxycode") come first, then the configured provider models.
        return new Response(
          JSON.stringify({
            data: [
              { id: "agent", owned_by: "foxxycode" },
              { id: "plan", owned_by: "foxxycode" },
              { id: "docs", owned_by: "foxxycode" },
              { id: "openai/gpt-4o", owned_by: "openai" },
            ],
          }),
          { status: 200 },
        );
      }
      if (url === "/foxxycode/providers/models-probe") {
        const body = JSON.parse(String(init?.body || "{}"));
        if (body.type === "codex") {
          return new Response(
            JSON.stringify({
              ok: true,
              models: [{ id: "gpt-5.6-sol", name: "GPT-5.6 Sol" }],
            }),
            { status: 200 },
          );
        }
        return new Response(
          JSON.stringify({
            ok: true,
            models: [{ id: "neuraldeep-chat" }, { id: "qwen-3" }],
          }),
          { status: 200 },
        );
      }
      if (
        url === "/foxxycode/providers/codex/codex-auth" &&
        (!init || init.method === "GET" || init.method === undefined)
      ) {
        return new Response(JSON.stringify({ connected: false }), {
          status: 200,
        });
      }
      if (
        url === "/foxxycode/providers/codex/codex-auth/device" &&
        init?.method === "POST"
      ) {
        return new Response(
          JSON.stringify({
            login_id: "login-onboarding",
            verification_url: "https://auth.example.test/codex/device",
            user_code: "CHAT-CODE",
            status: "pending",
          }),
          { status: 200 },
        );
      }
      if (
        url === "/foxxycode/providers/codex/codex-auth/device/login-onboarding"
      ) {
        return new Response(
          JSON.stringify({ status: "completed", connected: true }),
          { status: 200 },
        );
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("renders when open and saves provider config", async () => {
    const onSaved = vi.fn();
    renderPicker({ onSaved });
    expect(screen.getByTestId("provider-picker-dialog")).toBeTruthy();
    fireEvent.change(screen.getByTestId("provider-api-key"), {
      target: { value: "sk-test-key" },
    });
    fireEvent.click(screen.getByTestId("provider-save"));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const putCall = fetchMock.mock.calls.find(
      (c) => c[0] === "/foxxycode/config" && c[1]?.method === "PUT",
    );
    expect(putCall).toBeTruthy();
    const body = JSON.parse(String(putCall![1]?.body));
    expect(body.providers[0].api_key).toBe("sk-test-key");
    expect(body.agent.model).toContain("openai/");
  });

  it("skip closes without save", async () => {
    const onSkip = vi.fn();
    renderPicker({ onSkip });
    fireEvent.click(screen.getByTestId("provider-skip"));
    expect(onSkip).toHaveBeenCalled();
  });

  it("test connection probes models", async () => {
    renderPicker({});
    fireEvent.change(screen.getByTestId("provider-api-key"), {
      target: { value: "sk-test" },
    });
    fireEvent.click(screen.getByTestId("provider-test"));
    await waitFor(() =>
      expect(screen.getByTestId("provider-test-ok")).toBeTruthy(),
    );
    expect(fetchMock).toHaveBeenCalledWith("/v1/models");
  });

  it("test connection does not auto-select the synthetic agent pseudo-model", async () => {
    // Uses the default (openai) preset, whose model field starts empty, so the
    // test-connection auto-select fires and must skip the synthetic pseudo-models.
    renderPicker({});
    fireEvent.change(screen.getByTestId("provider-api-key"), {
      target: { value: "sk-test" },
    });
    fireEvent.click(screen.getByTestId("provider-test"));
    await waitFor(() =>
      expect(screen.getByTestId("provider-test-ok")).toBeTruthy(),
    );
    const modelInput = screen.getByTestId(
      "provider-model-id",
    ) as HTMLInputElement;
    // Must skip agent/plan/docs/ask (owned_by "foxxycode") and pick the real model.
    expect(modelInput.value).not.toBe("agent");
    expect(modelInput.value).toBe("openai/gpt-4o");
  });

  it("pre-selects the neuraldeep default model in the combobox", async () => {
    renderPicker({});
    fireEvent.click(screen.getByTestId("provider-card-neuraldeep"));
    const modelInput = (await waitFor(() =>
      screen.getByTestId("provider-model-id"),
    )) as HTMLInputElement;
    // The neuraldeep preset seeds its recommended default so it is visible, not
    // just placeholder text; other presets start empty.
    expect(modelInput.value).toBe("neuraldeep/gpt-oss-120b");
  });

  it("renders translated strings for the ru locale", () => {
    initLocale("ru");
    renderPicker({});
    expect(screen.getByText("Выберите провайдера")).toBeTruthy();
    expect(screen.getByTestId("provider-save").textContent).toBe(
      "Сохранить и продолжить",
    );
    expect(screen.getByTestId("provider-skip").textContent).toBe(
      "Пропустить пока",
    );
    expect(screen.getByText("GPT-4o и совместимые модели")).toBeTruthy();
  });

  it("selects NeuralDeep preset with readonly api base (no /v1) and hub link", async () => {
    renderPicker({});
    expect(screen.getByTestId("provider-card-neuraldeep")).toBeTruthy();
    fireEvent.click(screen.getByTestId("provider-card-neuraldeep"));
    await waitFor(() => {
      const base = screen.getByTestId("provider-api-base") as HTMLInputElement;
      expect(base.value).toBe("https://api.neuraldeep.ru");
      expect(base.readOnly).toBe(true);
    });
    const hub = screen.getByTestId("provider-hub-link") as HTMLAnchorElement;
    expect(hub.href).toContain("hub.neuraldeep.ru");
    expect(hub.target).toBe("_blank");
  });

  it("fetches models into the combobox and saves neuraldeep/<id> without api_base", async () => {
    const onSaved = vi.fn();
    renderPicker({ onSaved });
    fireEvent.click(screen.getByTestId("provider-card-neuraldeep"));
    fireEvent.change(screen.getByTestId("provider-api-key"), {
      target: { value: "sk-nd" },
    });
    fireEvent.click(screen.getByTestId("provider-fetch-models"));
    const modelInput = await waitFor(() =>
      screen.getByTestId("provider-model-id"),
    );
    const probeCall = fetchMock.mock.calls.find(
      (c) => c[0] === "/foxxycode/providers/models-probe",
    );
    expect(probeCall).toBeTruthy();
    const probeBody = JSON.parse(String(probeCall![1]?.body));
    // The neuraldeep provider type pins its own endpoint server-side, so the
    // probe carries no api_base at all.
    expect(probeBody).toEqual({
      type: "neuraldeep",
      api_base: "",
      api_key: "sk-nd",
      proxy: "",
    });
    // Open the combobox dropdown and pick the fetched model from the list.
    fireEvent.focus(modelInput);
    const option = await waitFor(() =>
      screen.getByRole("option", { name: "qwen-3" }),
    );
    fireEvent.mouseDown(option);
    expect((modelInput as HTMLInputElement).value).toBe("qwen-3");
    fireEvent.click(screen.getByTestId("provider-save"));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const putCall = fetchMock.mock.calls.find(
      (c) => c[0] === "/foxxycode/config" && c[1]?.method === "PUT",
    );
    const body = JSON.parse(String(putCall![1]?.body));
    expect(body.providers[0].type).toBe("neuraldeep");
    expect(body.providers[0].api_base).toBeUndefined();
    expect(body.models[0].model).toBe("neuraldeep/qwen-3");
    expect(body.models[0].multimodal).toBe(true);
    expect(body.agent.model).toBe("neuraldeep/qwen-3");
  });

  // A hub serves vision and text-only models side by side under one provider, so
  // the per-provider preset flag is the wrong granularity: the model the user
  // actually picked has to decide. Getting this wrong is how a vision-capable
  // model ends up saved with multimodal: false and silently refuses screenshots.
  it.each([
    ["vision-model", true],
    ["text-only-model", false],
  ])("seeds multimodal from the picked model's advertised vision (%s)", async (
    modelId,
    wantMultimodal,
  ) => {
    fetchMock.mockImplementation(async (url: string, init?: RequestInit) => {
      if (url === "/foxxycode/providers/models-probe") {
        return new Response(
          JSON.stringify({
            ok: true,
            models: [
              { id: "vision-model", vision: true },
              { id: "text-only-model", vision: false },
            ],
          }),
          { status: 200 },
        );
      }
      if (url === "/foxxycode/config" && init?.method === "PUT") {
        return new Response(JSON.stringify({ ok: true }), { status: 200 });
      }
      if (url === "/foxxycode/config/validate") {
        return new Response(JSON.stringify({ ok: true }), { status: 200 });
      }
      return new Response(JSON.stringify({}), { status: 200 });
    });

    const onSaved = vi.fn();
    renderPicker({ onSaved });
    // neuraldeep's preset declares multimodal: true for the whole provider.
    fireEvent.click(screen.getByTestId("provider-card-neuraldeep"));
    fireEvent.change(screen.getByTestId("provider-api-key"), {
      target: { value: "sk-nd" },
    });
    fireEvent.click(screen.getByTestId("provider-fetch-models"));
    const modelInput = await waitFor(() =>
      screen.getByTestId("provider-model-id"),
    );
    fireEvent.focus(modelInput);
    const option = await waitFor(() =>
      screen.getByRole("option", { name: modelId }),
    );
    fireEvent.mouseDown(option);
    fireEvent.click(screen.getByTestId("provider-save"));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());

    const putCall = fetchMock.mock.calls.find(
      (c) => c[0] === "/foxxycode/config" && c[1]?.method === "PUT",
    );
    const body = JSON.parse(String(putCall![1]?.body));
    expect(body.models[0].model).toBe(`neuraldeep/${modelId}`);
    expect(body.models[0].multimodal).toBe(wantMultimodal);
  });

  it("sends the proxy in the probe and saves it on the provider", async () => {
    const onSaved = vi.fn();
    renderPicker({ onSaved });
    fireEvent.click(screen.getByTestId("provider-card-neuraldeep"));
    fireEvent.change(screen.getByTestId("provider-api-key"), {
      target: { value: "sk-nd" },
    });
    fireEvent.change(screen.getByTestId("provider-proxy"), {
      target: { value: "socks5h://127.0.0.1:1080" },
    });
    fireEvent.click(screen.getByTestId("provider-fetch-models"));
    await waitFor(() => {
      const probeCall = fetchMock.mock.calls.find(
        (c) => c[0] === "/foxxycode/providers/models-probe",
      );
      expect(probeCall).toBeTruthy();
      const probeBody = JSON.parse(String(probeCall![1]?.body));
      expect(probeBody.proxy).toBe("socks5h://127.0.0.1:1080");
    });
    fireEvent.click(screen.getByTestId("provider-save"));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const putCall = fetchMock.mock.calls.find(
      (c) => c[0] === "/foxxycode/config" && c[1]?.method === "PUT",
    );
    const body = JSON.parse(String(putCall![1]?.body));
    expect(body.providers[0].proxy).toBe("socks5h://127.0.0.1:1080");
  });

  it("omits proxy from the saved provider when left empty", async () => {
    const onSaved = vi.fn();
    renderPicker({ onSaved });
    fireEvent.change(screen.getByTestId("provider-api-key"), {
      target: { value: "sk-test-key" },
    });
    fireEvent.click(screen.getByTestId("provider-save"));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const putCall = fetchMock.mock.calls.find(
      (c) => c[0] === "/foxxycode/config" && c[1]?.method === "PUT",
    );
    const body = JSON.parse(String(putCall![1]?.body));
    expect(body.providers[0].proxy).toBeUndefined();
  });

  it("saves the anthropic preset as a non-multimodal model", async () => {
    const onSaved = vi.fn();
    renderPicker({ onSaved });
    fireEvent.click(screen.getByTestId("provider-card-anthropic"));
    fireEvent.change(screen.getByTestId("provider-api-key"), {
      target: { value: "sk-ant" },
    });
    fireEvent.click(screen.getByTestId("provider-save"));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const putCall = fetchMock.mock.calls.find(
      (c) => c[0] === "/foxxycode/config" && c[1]?.method === "PUT",
    );
    const body = JSON.parse(String(putCall![1]?.body));
    expect(body.providers[0].type).toBe("anthropic");
    expect(body.models[0].multimodal).toBe(false);
  });

  it("onboards Codex with ChatGPT OAuth and no API key in config", async () => {
    const onSaved = vi.fn();
    const openSpy = vi.spyOn(window, "open").mockImplementation(() => null);
    renderPicker({ onSaved });

    // Switching from a preset that seeds api_base/api_key must not leak those
    // fields into the Codex provider.
    fireEvent.click(screen.getByTestId("provider-card-ollama"));
    await waitFor(() =>
      expect(
        (screen.getByTestId("provider-api-base") as HTMLInputElement).value,
      ).toContain("11434"),
    );
    fireEvent.click(screen.getByTestId("provider-card-codex"));

    expect(screen.queryByTestId("provider-api-key")).toBeNull();
    expect(screen.getByTestId("codex-auth-sign-in")).toBeTruthy();
    fireEvent.click(screen.getByTestId("codex-auth-sign-in"));
    expect(await screen.findByText("CHAT-CODE")).toBeTruthy();
    expect(openSpy).toHaveBeenCalledWith(
      "https://auth.example.test/codex/device",
      "_blank",
      "noopener,noreferrer",
    );

    fireEvent.click(screen.getByTestId("provider-fetch-models"));
    const modelInput = await waitFor(() =>
      screen.getByTestId("provider-model-id"),
    );
    fireEvent.focus(modelInput);
    const option = await waitFor(() =>
      screen.getByRole("option", { name: "GPT-5.6 Sol" }),
    );
    fireEvent.mouseDown(option);

    const probeCall = fetchMock.mock.calls.find(
      (c) => c[0] === "/foxxycode/providers/models-probe",
    );
    expect(probeCall).toBeTruthy();
    expect(JSON.parse(String(probeCall![1]?.body))).toEqual({
      type: "codex",
      provider_name: "codex",
      api_base: "",
      api_key: "",
      proxy: "",
    });

    fireEvent.click(screen.getByTestId("provider-save"));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const putCall = fetchMock.mock.calls.find(
      (c) => c[0] === "/foxxycode/config" && c[1]?.method === "PUT",
    );
    const body = JSON.parse(String(putCall![1]?.body));
    expect(body.providers[0]).toEqual({
      name: "codex",
      type: "codex",
    });
    expect(body.models[0].model).toBe("codex/gpt-5.6-sol");
    expect(body.models[0].multimodal).toBe(false);
    expect(body.agent.model).toBe("codex/gpt-5.6-sol");
  });

  it("falls back to manual model entry when the probe fails", async () => {
    fetchMock.mockImplementation(async (url: string) => {
      if (url === "/foxxycode/config") {
        return new Response(JSON.stringify({}), { status: 200 });
      }
      if (url === "/foxxycode/providers/models-probe") {
        return new Response(
          JSON.stringify({ ok: false, error: "HTTP 401", models: [] }),
          { status: 200 },
        );
      }
      return new Response("not found", { status: 404 });
    });
    renderPicker({});
    fireEvent.click(screen.getByTestId("provider-card-neuraldeep"));
    fireEvent.change(screen.getByTestId("provider-api-key"), {
      target: { value: "sk-bad" },
    });
    fireEvent.click(screen.getByTestId("provider-fetch-models"));
    await waitFor(() =>
      expect(screen.getByTestId("provider-models-error")).toBeTruthy(),
    );
    // The combobox input is always present; on probe failure it acts as a
    // free-text field for manual model entry.
    expect(screen.getByTestId("provider-model-id")).toBeTruthy();
  });
});
