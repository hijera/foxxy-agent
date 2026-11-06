import { describe, expect, it } from "vitest";
import {
  clampCamera,
  fitCamera,
  graphPoint,
  panCamera,
  screenPoint,
  zoomCameraAt,
  type Bounds,
  type GraphCamera,
  type Point,
  type Size,
} from "./graphViewport";

const bounds: Bounds = { x: 0, y: 0, width: 1000, height: 500 };
const viewport: Size = { width: 500, height: 300 };
const padding = 20;
const fallbackCamera: GraphCamera = {
  x: 0,
  y: 0,
  scale: 1,
  fitScale: 1,
  userAdjusted: false,
};
const extremeBounds = [
  { axis: "x", bounds: { x: Number.MAX_VALUE, y: 0, width: 100, height: 100 } },
  { axis: "y", bounds: { x: 0, y: Number.MAX_VALUE, width: 100, height: 100 } },
];
const unpaddedViewport: Size = { width: 500, height: 300 };
const validCamera: GraphCamera = {
  x: 0,
  y: 0,
  scale: 2,
  fitScale: 1,
  userAdjusted: false,
};

function expectFinite(camera: GraphCamera): void {
  for (const value of Object.values(camera)) {
    if (typeof value === "number") expect(Number.isFinite(value)).toBe(true);
  }
}

describe("graph viewport camera math", () => {
  it("fits a graph within padding and maps its center to the viewport center", () => {
    const camera = fitCamera(bounds, viewport, padding);

    expect(camera.fitScale).toBeCloseTo(0.46);
    expect(camera.scale).toBeCloseTo(0.46);
    expect(screenPoint(camera, { x: 500, y: 250 })).toEqual({
      x: 250,
      y: 150,
    });
  });

  it("does not enlarge a graph that already fits", () => {
    const camera = fitCamera(
      { x: 0, y: 0, width: 100, height: 50 },
      viewport,
      padding,
    );

    expect(camera.fitScale).toBe(1);
    expect(camera.scale).toBe(1);
  });

  it("treats invalid padding as zero", () => {
    const camera = fitCamera(bounds, viewport, Number.NaN);

    expect(camera.fitScale).toBeCloseTo(0.5);
    expect(camera.x).toBe(0);
    expect(camera.y).toBe(25);
  });

  it("keeps the graph point beneath the zoom focus fixed", () => {
    const camera = fitCamera(bounds, viewport, padding);
    const focus: Point = { x: 250, y: 150 };
    const before = graphPoint(camera, focus);
    const zoomed = zoomCameraAt(camera, 2, focus, bounds, viewport, padding);

    expect(graphPoint(zoomed, focus).x).toBeCloseTo(before.x);
    expect(graphPoint(zoomed, focus).y).toBeCloseTo(before.y);
  });

  it("clamps zoom scale between the fit scale and three times that scale", () => {
    const camera = fitCamera(bounds, viewport, padding);

    expect(
      zoomCameraAt(camera, 0.01, { x: 250, y: 150 }, bounds, viewport, padding)
        .scale,
    ).toBeCloseTo(camera.fitScale);
    expect(
      zoomCameraAt(camera, 100, { x: 250, y: 150 }, bounds, viewport, padding)
        .scale,
    ).toBeCloseTo(camera.fitScale * 3);
  });

  it("centers a graph that fits without zooming", () => {
    const small: Bounds = { x: 0, y: 0, width: 100, height: 50 };
    const camera = fitCamera(small, viewport, padding);

    expect(camera.x).toBe(200);
    expect(camera.y).toBe(125);
  });

  it("pans a fitted small graph on both axes without snapping back to center", () => {
    const small: Bounds = { x: 0, y: 0, width: 100, height: 50 };
    const fitted = fitCamera(small, viewport, padding);

    const panned = panCamera(fitted, 30, -40, small, viewport, padding);
    expect(panned.x).toBe(230);
    expect(panned.y).toBe(85);
    expect(panned.userAdjusted).toBe(true);

    const reclamped = clampCamera(
      clampCamera(panned, small, viewport, padding),
      small,
      viewport,
      padding,
    );
    expect(reclamped.x).toBe(230);
    expect(reclamped.y).toBe(85);
  });

  it("lets a small graph's edges reach the canvas padding on both axes", () => {
    const small: Bounds = { x: 0, y: 0, width: 100, height: 50 };
    const fitted = fitCamera(small, viewport, padding);

    const right = panCamera(fitted, 10000, 0, small, viewport, padding);
    expect(right.x).toBe(viewport.width - padding);

    const left = panCamera(fitted, -10000, 0, small, viewport, padding);
    expect(left.x).toBe(padding - small.width);

    const down = panCamera(fitted, 0, 10000, small, viewport, padding);
    expect(down.y).toBe(viewport.height - padding);

    const up = panCamera(fitted, 0, -10000, small, viewport, padding);
    expect(up.y).toBe(padding - small.height);

    for (const camera of [right, left, down, up]) expectFinite(camera);
  });

  it("clamps panning so either content edge can reach the canvas padding", () => {
    const wideButShort: Bounds = { x: 0, y: 0, width: 1000, height: 100 };
    const camera: GraphCamera = {
      x: -100,
      y: 0,
      scale: 1,
      fitScale: fitCamera(wideButShort, viewport, padding).fitScale,
      userAdjusted: false,
    };

    const right = panCamera(camera, 10000, 50, wideButShort, viewport, padding);
    expect(right.x).toBe(480);
    expect(right.y).toBe(50);

    const left = panCamera(
      right,
      -10000,
      -200,
      wideButShort,
      viewport,
      padding,
    );
    expect(left.x).toBe(-980);
    expect(left.y).toBe(-80);
  });

  it("returns a finite fallback camera for invalid or nonfinite geometry", () => {
    const invalid: Bounds = {
      x: Number.NaN,
      y: Number.POSITIVE_INFINITY,
      width: 0,
      height: Number.NaN,
    };
    const invalidViewport: Size = {
      width: Number.NEGATIVE_INFINITY,
      height: 0,
    };
    const invalidCamera: GraphCamera = {
      x: Number.NaN,
      y: Number.POSITIVE_INFINITY,
      scale: Number.NaN,
      fitScale: 0,
      userAdjusted: true,
    };

    expect(fitCamera(invalid, invalidViewport, Number.NaN)).toEqual({
      x: 0,
      y: 0,
      scale: 1,
      fitScale: 1,
      userAdjusted: false,
    });
    expectFinite(
      clampCamera(invalidCamera, invalid, invalidViewport, Infinity),
    );
    expectFinite(
      zoomCameraAt(
        invalidCamera,
        Number.NaN,
        { x: Infinity, y: Number.NaN },
        invalid,
        invalidViewport,
        Number.NEGATIVE_INFINITY,
      ),
    );
    expectFinite(
      panCamera(
        invalidCamera,
        Infinity,
        Number.NaN,
        invalid,
        invalidViewport,
        Number.NaN,
      ),
    );
  });

  it.each(extremeBounds)(
    "fits $axis-axis geometry with overflowing scale to fallback",
    ({ bounds }) => {
      expect(fitCamera(bounds, unpaddedViewport, 0)).toEqual(fallbackCamera);
    },
  );

  it.each(extremeBounds)(
    "clamps $axis-axis geometry with overflowing scale to fallback",
    ({ bounds }) => {
      expect(clampCamera(validCamera, bounds, unpaddedViewport, 0)).toEqual(
        fallbackCamera,
      );
    },
  );

  it.each(extremeBounds)(
    "zooms $axis-axis geometry with overflowing scale to fallback",
    ({ bounds }) => {
      expect(
        zoomCameraAt(
          validCamera,
          2,
          { x: 250, y: 150 },
          bounds,
          unpaddedViewport,
          0,
        ),
      ).toEqual(fallbackCamera);
    },
  );

  it.each(extremeBounds)(
    "pans $axis-axis geometry with overflowing scale to fallback",
    ({ bounds }) => {
      expect(
        panCamera(validCamera, 10, 10, bounds, unpaddedViewport, 0),
      ).toEqual(fallbackCamera);
    },
  );

  it("marks fit cameras untouched and zoomed or panned cameras adjusted", () => {
    const fitted = fitCamera(bounds, viewport, padding);

    expect(fitted.userAdjusted).toBe(false);
    expect(
      zoomCameraAt(fitted, 2, { x: 250, y: 150 }, bounds, viewport, padding)
        .userAdjusted,
    ).toBe(true);
    expect(
      panCamera(fitted, 10, 10, bounds, viewport, padding).userAdjusted,
    ).toBe(true);
  });

  it("converts finite graph and screen coordinates inversely", () => {
    const camera: GraphCamera = {
      x: -120,
      y: 40,
      scale: 0.75,
      fitScale: 0.5,
      userAdjusted: true,
    };
    const graph: Point = { x: 900.25, y: -125.5 };
    const screen = screenPoint(camera, graph);

    expect(graphPoint(camera, screen).x).toBeCloseTo(graph.x);
    expect(graphPoint(camera, screen).y).toBeCloseTo(graph.y);
  });
});
