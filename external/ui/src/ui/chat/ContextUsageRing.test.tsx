import { render } from "@testing-library/react";
import { describe, expect, test } from "vitest";
import { ContextUsageRing } from "./ContextUsageRing";

describe("ContextUsageRing", () => {
  test("progress arc uses dash offset from fill ratio", () => {
    const { container } = render(<ContextUsageRing fill01={0.25} />);
    const fg = container.querySelector(".context-ring-fg") as SVGCircleElement;
    expect(fg).toBeTruthy();
    const c = 2 * Math.PI * 12;
    const off = Number.parseFloat(fg.getAttribute("stroke-dashoffset") || "0");
    expect(off).toBeCloseTo(c * 0.75, 1);
  });

  test("idle shows inner ring only, no outer arc or track", () => {
    const { container } = render(<ContextUsageRing fill01={0} />);
    expect(container.querySelector(".context-ring-inner")).toBeTruthy();
    expect(container.querySelector(".context-ring-track")).toBeNull();
    expect(container.querySelector(".context-ring-fg")).toBeNull();
  });

  test("non-zero fill renders outer arc", () => {
    const { container } = render(<ContextUsageRing fill01={0.01} />);
    expect(container.querySelector(".context-ring-fg")).toBeTruthy();
  });
});
