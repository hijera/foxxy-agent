import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../i18n/I18nProvider";
import { initLocale } from "../i18n/i18n";
import { ProjectDialog } from "./ProjectDialog";
import type { ProjectInfo } from "./projectApi";

const recentPayload = {
  object: "list",
  data: [
    {
      path: "H:\\work\\alpha",
      name: "alpha",
      last_opened_at: "2026-07-06T10:00:00Z",
      exists: true,
    },
    {
      path: "H:\\work\\gone",
      name: "gone",
      last_opened_at: "2026-07-05T10:00:00Z",
      exists: false,
    },
  ],
};

function renderDialog(opts?: {
  project?: ProjectInfo | null;
  onOpened?: (info: ProjectInfo) => void;
  onClose?: () => void;
}) {
  return render(
    <I18nProvider>
      <ProjectDialog
        open
        project={
          opts?.project !== undefined
            ? opts.project
            : { path: "H:\\work\\alpha", source: "default", native_picker: false }
        }
        onClose={opts?.onClose ?? (() => {})}
        onOpened={opts?.onOpened ?? (() => {})}
      />
    </I18nProvider>,
  );
}

describe("ProjectDialog", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    initLocale("en");
    fetchMock.mockImplementation(async (url: string, init?: RequestInit) => {
      if (url === "/foxxycode/projects/recent") {
        return new Response(JSON.stringify(recentPayload), { status: 200 });
      }
      if (url === "/foxxycode/project" && init?.method === "PUT") {
        const body = JSON.parse(String(init.body)) as { path: string };
        if (body.path.indexOf("bad") >= 0) {
          return new Response(
            JSON.stringify({ error: { message: "not a directory" } }),
            { status: 400 },
          );
        }
        return new Response(
          JSON.stringify({
            path: body.path,
            source: "project",
            native_picker: false,
          }),
          { status: 200 },
        );
      }
      if (url === "/foxxycode/project/pick-folder") {
        return new Response(
          JSON.stringify({ path: "H:\\picked", cancelled: false }),
          { status: 200 },
        );
      }
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("renders recent projects and dims missing ones", async () => {
    renderDialog();
    await waitFor(() =>
      expect(screen.getByTestId("project-recent-alpha")).toBeTruthy(),
    );
    const missing = screen.getByTestId("project-recent-gone");
    expect(missing.className).toContain("project-dialog-recent-row--missing");
  });

  it("hides Browse without native picker and shows it with one", async () => {
    const first = renderDialog();
    expect(screen.queryByTestId("project-browse")).toBeNull();
    first.unmount();

    renderDialog({
      project: { path: "H:\\x", source: "default", native_picker: true },
    });
    expect(screen.getByTestId("project-browse")).toBeTruthy();
  });

  it("browse fills the path input from the picker", async () => {
    renderDialog({
      project: { path: "H:\\x", source: "default", native_picker: true },
    });
    fireEvent.click(screen.getByTestId("project-browse"));
    await waitFor(() =>
      expect(
        (screen.getByTestId("project-path-input") as HTMLInputElement).value,
      ).toBe("H:\\picked"),
    );
  });

  it("open PUTs the typed path and reports the new project", async () => {
    const onOpened = vi.fn();
    renderDialog({ onOpened });
    fireEvent.change(screen.getByTestId("project-path-input"), {
      target: { value: "H:\\work\\beta" },
    });
    fireEvent.click(screen.getByTestId("project-open"));
    await waitFor(() => expect(onOpened).toHaveBeenCalled());
    const info = onOpened.mock.calls[0][0] as ProjectInfo;
    expect(info.path).toBe("H:\\work\\beta");
    expect(info.source).toBe("project");
  });

  it("surfaces a 400 error inline", async () => {
    renderDialog();
    fireEvent.change(screen.getByTestId("project-path-input"), {
      target: { value: "H:\\bad" },
    });
    fireEvent.click(screen.getByTestId("project-open"));
    await waitFor(() =>
      expect(screen.getByTestId("project-error").textContent).toBe(
        "not a directory",
      ),
    );
  });

  it("clicking an existing recent project opens it directly", async () => {
    const onOpened = vi.fn();
    renderDialog({ onOpened });
    await waitFor(() =>
      expect(screen.getByTestId("project-recent-alpha")).toBeTruthy(),
    );
    fireEvent.click(screen.getByTestId("project-recent-alpha"));
    await waitFor(() => expect(onOpened).toHaveBeenCalled());
    const info = onOpened.mock.calls[0][0] as ProjectInfo;
    expect(info.path).toBe("H:\\work\\alpha");
  });

  it("clicking a missing recent project only fills the input", async () => {
    const onOpened = vi.fn();
    renderDialog({ onOpened });
    await waitFor(() =>
      expect(screen.getByTestId("project-recent-gone")).toBeTruthy(),
    );
    fireEvent.click(screen.getByTestId("project-recent-gone"));
    expect(
      (screen.getByTestId("project-path-input") as HTMLInputElement).value,
    ).toBe("H:\\work\\gone");
    expect(onOpened).not.toHaveBeenCalled();
  });
});
