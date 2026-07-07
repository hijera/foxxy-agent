import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { AppErrorBoundary } from "./AppErrorBoundary";

function Bomb(): never {
  throw new Error("kaboom from render");
}

describe("AppErrorBoundary", () => {
  it("renders children when nothing throws", () => {
    render(
      <AppErrorBoundary>
        <div data-testid="ok-child">fine</div>
      </AppErrorBoundary>,
    );
    expect(screen.getByTestId("ok-child")).toBeInTheDocument();
  });

  it("shows the fallback with the error message instead of a blank tree", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    try {
      render(
        <AppErrorBoundary>
          <Bomb />
        </AppErrorBoundary>,
      );
    } finally {
      spy.mockRestore();
    }
    expect(screen.getByTestId("app-error-boundary")).toBeInTheDocument();
    expect(screen.getByText("kaboom from render")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reload" })).toBeInTheDocument();
  });
});
