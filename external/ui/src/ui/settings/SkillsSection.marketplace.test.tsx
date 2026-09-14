import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { SkillsSection } from "./SkillsSection";
import type { JsonSchema } from "./SchemaForm";
import { setLocale } from "../i18n/i18n";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  setLocale("en");
});

const skillsSchema = {
  type: "object",
  title: "Skills",
  properties: {
    dirs: {
      type: "array",
      title: "Skill directories",
      items: { type: "string" },
    },
    sources: {
      type: "array",
      title: "Remote skill sources",
      items: { type: "string" },
    },
    auto_discovery: {
      type: "boolean",
      title: "Skill auto-discovery",
    },
  },
} as unknown as JsonSchema;

type Reply = { status?: number; body: unknown };

/** Stubs fetch with a router over the skills routes; anything unrouted answers an empty list. */
function stubFetch(route: (url: string, init?: RequestInit) => Reply | undefined) {
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const reply = route(String(input), init) ?? { body: { items: [] } };
    const status = reply.status ?? 200;
    return {
      ok: status >= 200 && status < 300,
      status,
      json: async () => reply.body,
    } as Response;
  });
  vi.stubGlobal("fetch", fn);
  return fn;
}

function callsTo(fn: ReturnType<typeof stubFetch>, url: string): number {
  return fn.mock.calls.filter(([u]) => String(u) === url).length;
}

const AVAILABLE = "/foxxycode/skills/available";

function searchFor(query: string): HTMLElement {
  const input = screen.getByTestId("skills-install-input");
  fireEvent.focus(input);
  fireEvent.change(input, { target: { value: query } });
  return input;
}

test("with no marketplace source the search points to Remote skill sources", () => {
  setLocale("ru");
  const fetchMock = stubFetch(() => undefined);
  render(
    <SkillsSection schema={skillsSchema} value={{ sources: [] }} onChange={() => {}} />,
  );

  expect(screen.getByTestId("skills-install-no-sources").textContent).toMatch(
    /Удалённые источники скилов/,
  );

  searchFor("demo");
  const results = screen.getByTestId("skills-install-results");
  expect(within(results).getByText(/Удалённые источники скилов/)).toBeTruthy();
  expect(within(results).queryByText("Подходящих скилов не найдено.")).toBeNull();
  // There is nothing to search yet, so the server is not asked.
  expect(callsTo(fetchMock, AVAILABLE)).toBe(0);
});

test("saved sources that publish nothing say so instead of 'no matches'", async () => {
  setLocale("ru");
  stubFetch((url) => (url === AVAILABLE ? { body: { items: [] } } : undefined));
  render(
    <SkillsSection
      schema={skillsSchema}
      value={{ sources: ["owner/repo"] }}
      onChange={() => {}}
    />,
  );
  expect(screen.queryByTestId("skills-install-no-sources")).toBeNull();

  searchFor("demo");
  await waitFor(() =>
    expect(
      within(screen.getByTestId("skills-install-results")).getByText(
        /сохраните настройки/,
      ),
    ).toBeTruthy(),
  );
});

test("a query that misses a non-empty marketplace keeps the no-matches line", async () => {
  stubFetch((url) =>
    url === AVAILABLE
      ? {
          body: {
            items: [
              { name: "demo", description: "Demo", source: "owner/repo", installed: false },
            ],
          },
        }
      : undefined,
  );
  render(
    <SkillsSection
      schema={skillsSchema}
      value={{ sources: ["owner/repo"] }}
      onChange={() => {}}
    />,
  );

  searchFor("zzz");
  await waitFor(() =>
    expect(screen.getByText("No matching marketplace skills.")).toBeTruthy(),
  );
});

test("after Save the search asks the server again instead of reusing the old list", async () => {
  let published: unknown[] = [];
  const fetchMock = stubFetch((url) =>
    url === AVAILABLE ? { body: { items: published } } : undefined,
  );
  const { rerender } = render(
    <SkillsSection
      schema={skillsSchema}
      value={{ sources: ["file:///srv/shop"] }}
      onChange={() => {}}
    />,
  );

  // Searched while the new source was still unsaved: the server knows no source yet.
  const input = searchFor("demo");
  await waitFor(() => expect(screen.getByText(/Just added a source\?/)).toBeTruthy());
  expect(callsTo(fetchMock, AVAILABLE)).toBe(1);

  // Save: Settings reloads the document, so the tab gets a new sources array with
  // the same entry, and from now on the server lists what the source publishes.
  published = [
    {
      name: "demo",
      description: "Demo skill demo",
      version: "1.0.0",
      source: "file:///srv/shop",
      installed: false,
    },
  ];
  rerender(
    <SkillsSection
      schema={skillsSchema}
      value={{ sources: ["file:///srv/shop"] }}
      onChange={() => {}}
    />,
  );

  fireEvent.focus(input);
  expect(await screen.findByTestId("skills-install-demo")).toBeTruthy();
  expect(callsTo(fetchMock, AVAILABLE)).toBe(2);
});

test("a finished Sync all drops the cached list as well", async () => {
  const fetchMock = stubFetch((url) => {
    if (url === AVAILABLE) return { body: { items: [] } };
    if (url === "/foxxycode/skills/sync") {
      return { body: { ok: true, added: [], updated: [], failed: [] } };
    }
    return undefined;
  });
  render(
    <SkillsSection
      schema={skillsSchema}
      value={{ sources: ["owner/repo"] }}
      onChange={() => {}}
    />,
  );

  const input = searchFor("demo");
  await waitFor(() => expect(callsTo(fetchMock, AVAILABLE)).toBe(1));

  fireEvent.click(screen.getByTestId("skills-sync-all"));
  await screen.findByText("Completed");

  fireEvent.focus(input);
  await waitFor(() => expect(callsTo(fetchMock, AVAILABLE)).toBe(2));
});

test("install and update report in the interface language", async () => {
  setLocale("ru");
  stubFetch((url) => {
    switch (url) {
      case "/foxxycode/skills":
        return {
          body: {
            items: [
              {
                name: "tool",
                description: "",
                file_path: "/home/u/.foxxycode/skills/tool/SKILL.md",
                enabled: true,
                version: "1.0.0",
                source: "owner/repo",
              },
            ],
          },
        };
      case AVAILABLE:
        return {
          body: {
            items: [
              { name: "demo", description: "Demo", source: "owner/repo", installed: false },
            ],
          },
        };
      case "/foxxycode/skills/updates":
        return {
          body: {
            items: [
              {
                name: "tool",
                source: "owner/repo",
                version: "1.0.0",
                latest: "2.0.0",
                update_available: true,
              },
            ],
          },
        };
      case "/foxxycode/skills/install":
      case "/foxxycode/skills/tool/update":
        return { body: { ok: true, added: [], updated: [], failed: [] } };
      default:
        return undefined;
    }
  });
  render(
    <SkillsSection
      schema={skillsSchema}
      value={{ sources: ["owner/repo"] }}
      onChange={() => {}}
    />,
  );

  searchFor("demo");
  fireEvent.click(await screen.findByTestId("skills-install-demo"));
  expect(await screen.findByText("Скил demo установлен.")).toBeTruthy();

  fireEvent.click(await screen.findByTestId("skills-update-tool"));
  expect(await screen.findByText("Скил tool обновлён.")).toBeTruthy();
});
