import React from "react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SettingsSection } from "./SettingsSection";
import type { JsonSchema } from "./SchemaForm";
import type { SectionDescriptor } from "./settingsSections";

afterEach(cleanup);

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
            enum: ["openai", "anthropic", "neuraldeep"],
          },
          api_base: { type: "string", title: "API base URL" },
          api_key: {
            type: "string",
            title: "API key",
            "x-foxxycode-provider-api-key-env-placeholder": true,
          },
        },
        "x-foxxycode-property-order": ["name", "type", "api_base", "api_key"],
      },
    },
  },
};

function Harness(props: { initial?: unknown[] }) {
  const [doc, setDoc] = React.useState<Record<string, unknown>>({
    providers: props.initial ?? [],
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

beforeEach(() => {
  Object.assign(navigator, {
    clipboard: {
      readText: vi.fn(),
      writeText: vi.fn().mockResolvedValue(undefined),
    },
  });
});

test("import from clipboard adds a provider and focuses the API key field", async () => {
  (navigator.clipboard.readText as ReturnType<typeof vi.fn>).mockResolvedValue(
    "foxxycode://provider?name=openrouter&type=openai&api_base=https://x/v1",
  );
  render(<Harness />);

  fireEvent.click(screen.getByTestId("provider-import-toggle"));
  fireEvent.click(screen.getByTestId("provider-import-clipboard"));

  const nameField = (await screen.findByLabelText(
    "Provider name",
  )) as HTMLInputElement;
  expect(nameField.value).toBe("openrouter");

  const apiKey = screen.getByLabelText("API key") as HTMLInputElement;
  await waitFor(() => {
    expect(document.activeElement).toBe(apiKey);
  });
});

test("import reconciles a colliding provider name with a -1 suffix", async () => {
  (navigator.clipboard.readText as ReturnType<typeof vi.fn>).mockResolvedValue(
    "foxxycode://provider?name=openai&type=openai",
  );
  render(<Harness initial={[{ name: "openai", type: "openai" }]} />);

  fireEvent.click(screen.getByTestId("provider-import-toggle"));
  fireEvent.click(screen.getByTestId("provider-import-clipboard"));

  const nameField = (await screen.findByLabelText(
    "Provider name",
  )) as HTMLInputElement;
  expect(nameField.value).toBe("openai-1");
});

test("export to clipboard copies a secret-free foxxycode:// query", async () => {
  render(
    <Harness
      initial={[
        {
          name: "openrouter",
          type: "openai",
          api_base: "https://x/v1",
          api_key: "sk-secret",
          proxy: "socks5://127.0.0.1:1080",
        },
      ]}
    />,
  );
  fireEvent.click(screen.getByTestId("settings-master-item-0"));
  fireEvent.click(screen.getByTestId("provider-export-clipboard"));

  await waitFor(() => {
    expect(navigator.clipboard.writeText).toHaveBeenCalledTimes(1);
  });
  const copied = (navigator.clipboard.writeText as ReturnType<typeof vi.fn>).mock
    .calls[0][0] as string;
  expect(copied.startsWith("foxxycode://provider?")).toBe(true);
  expect(copied).toContain("name=openrouter");
  expect(copied).not.toContain("sk-secret");
  expect(copied).not.toContain("socks5");
});
