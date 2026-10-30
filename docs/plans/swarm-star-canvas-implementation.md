# Swarm Star Canvas Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a persistent force-directed Swarm layout, image-viewer-style canvas navigation, an always-available environment selector, and reliable transit-relay entry through the existing mount chain.

**Architecture:** Keep the existing tiered `layoutTopology` as the default tree and add a deterministic pure `layoutTopologyStar` that returns the same `TopologyLayout`. Put camera arithmetic in a pure module and DOM gesture state in a focused hook, while `TopologyGraph` remains the shared SVG renderer. Preserve the mount protocol; cover a real nested relay first, then make a relay join advertise its own configured client credential when no dedicated join token was supplied.

**Tech Stack:** TypeScript 6, React 19, hand-written SVG, Vitest + Testing Library, Go 1.25, godog, Playwright browser tools, CSS custom properties.

---

## Baseline note

The isolated worktree is `.foxxycode/worktrees/feat-swarm-star-canvas` on branch `feat/swarm-star-canvas`. Before feature code, direct Swarm UI tests were green: 73 tests in `external/ui/src/ui/swarm`. The full clean `make test` passed all 2,484 direct Vitest tests and every Go package except the existing `external/ui` Go BDD wrapper, where `TestBackgroundWakeWebUIFeature` left its child Vitest process waiting until the package's 10-minute timeout. The exact isolated test then passed all 5 scenarios and 8 steps in 23.65 seconds, so this is a non-reproducing full-run harness timeout rather than a Swarm failure. Keep that baseline event separate from this feature; use the exact diagnostic command below if it recurs in the final full run:

```bash
go test -tags=http,ui ./external/ui -run '^TestBackgroundWakeWebUIFeature$' -count=1 -v
```

## File structure

### New files

- `external/ui/src/ui/swarm/layoutMode.ts` — validates and persists `tree | star` in browser storage.
- `external/ui/src/ui/swarm/layoutMode.test.ts` — persistence/default tests.
- `external/ui/src/ui/swarm/forceLayout.ts` — deterministic rooted force solver returning `TopologyLayout`.
- `external/ui/src/ui/swarm/forceLayout.test.ts` — geometry, determinism, collision, ring, and empty-state tests.
- `external/ui/src/ui/swarm/graphViewport.ts` — pure fit/zoom/pan/clamp arithmetic.
- `external/ui/src/ui/swarm/graphViewport.test.ts` — camera math tests.
- `external/ui/src/ui/swarm/useGraphViewport.ts` — `ResizeObserver`, pointer, wheel, pinch, keyboard, and reset orchestration.
- `docs/assets/swarm/map-star-canvas-dark-1280.png` — screenshot of the real running surface.

### Modified files

- `internal/swarm/joinset.go` — resolve the credential a relay advertises to its parent.
- `internal/swarm/swarm_test.go` — relay-token fallback and explicit-token precedence tests.
- `external/swarm/bdd_mount_test.go` — real outer/child relay mount fixture.
- `features/swarm_mount.feature` — transit-relay happy path.
- `external/ui/src/ui/swarm/layout.ts` — export node bounds helpers needed by the force solver without changing tree behavior.
- `external/ui/src/ui/swarm/TopologyGraph.tsx` — select layout, render controls, and attach viewport interactions.
- `external/ui/src/ui/swarm/SwarmView.tsx` — own layout mode, keep a common header in success and error states.
- `external/ui/src/ui/swarm/SwarmView.test.tsx` — controls, persistence, gestures, error-header, and nested-relay callback tests.
- `external/ui/src/ui/env/swarmEnv.test.ts` — nested mounted relay URL test.
- `external/ui/src/ui/App.swarmNode.test.tsx` — full transit path and always-present header selector.
- `external/ui/src/ui/App.tsx` — pass `EnvironmentChip` on every Swarm screen.
- `external/ui/src/ui/i18n/messages/en.ts` — English canvas labels.
- `external/ui/src/ui/i18n/messages/ru.ts` — Russian canvas labels.
- `external/ui/src/styles.css` — bounded viewport, top-left layout switch, bottom-right camera controls, cursors, themes, and reduced motion.
- `features/swarm_web_ui.feature` — layout-selection happy path.
- `external/ui/bdd_swarm_ui_test.go` — map the feature step to a named Vitest test.
- `DESIGN.md` — authoritative canvas contract.
- `docs/operate/swarm.md` — operator workflow and transit-relay behavior.
- `docs/surfaces/web-ui.md` — controls and persistence checklist.
- `docs/assets/INDEX.md` — regenerated asset inventory.

## Task 1: Prove and repair the transit-relay credential chain

**Files:**
- Modify: `features/swarm_mount.feature`
- Modify: `external/swarm/bdd_mount_test.go`
- Modify: `internal/swarm/swarm_test.go`
- Modify: `internal/swarm/joinset.go`
- Modify: `docs/operate/swarm.md`

- [ ] **Step 1: Add the failing relay-join credential tests**

Add table-driven coverage next to `TestStartJoinsGivesEveryParentTheSameIdentity`:

```go
func TestStartJoinsChoosesTheCredentialAParentUses(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		authToken string
		joinToken string
		want      string
	}{
		{name: "relay falls back to its own client token", kind: KindRelay, authToken: "relay-client", want: "relay-client"},
		{name: "dedicated join token wins", kind: KindRelay, authToken: "relay-client", joinToken: "parent-only", want: "parent-only"},
		{name: "agent does not leak an unrelated relay token", kind: KindAgent, authToken: "relay-client", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Swarm.AuthToken = tt.authToken
			cfg.Swarm.Join = []config.SwarmJoin{{
				URL: "http://127.0.0.1:1", Name: "child", Token: tt.joinToken,
			}}
			set, err := StartJoins(context.Background(), cfg, StartJoinsOptions{
				Kind: tt.kind, Handler: http.NotFoundHandler(), Log: quietLogger(),
			})
			if err != nil { t.Fatal(err) }
			defer set.Stop()
			if got := set.Clients()[0].opts.NodeToken; got != tt.want {
				t.Fatalf("NodeToken = %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the unit test and verify RED**

Run:

```bash
go test ./internal/swarm -run TestStartJoinsChoosesTheCredentialAParentUses -count=1 -v
```

Expected: the relay fallback case fails with `NodeToken = "", want "relay-client"`; the explicit and agent cases already pass.

- [ ] **Step 3: Implement the smallest credential resolver**

In `StartJoins`, resolve each join token before `NewClient`:

```go
nodeToken := strings.TrimSpace(j.Token)
if nodeToken == "" && kind == KindRelay {
	nodeToken = strings.TrimSpace(cfg.Swarm.AuthToken)
}
client, err := NewClient(JoinOptions{
	RelayURL: j.URL,
	Name: j.Name,
	Kind: kind,
	PairingToken: j.PairingToken,
	AdvertiseURL: j.AdvertiseURL,
	NodeToken: nodeToken,
	// keep the remaining existing fields unchanged
})
```

Do not apply the fallback to agents. Do not forward the parent's pairing token or the browser's outer client token.

- [ ] **Step 4: Verify the unit test is GREEN**

Run the same command. Expected: all three subtests pass.

- [ ] **Step 5: Add the transit-relay BDD scenario**

Append to `features/swarm_mount.feature`:

```gherkin
  Scenario: A relay behind another relay opens through the established trust chain
    Given a child relay "inner" with client token "inner-client" is registered under this relay
    When I read the child relay info and topology through its mount with the client token
    Then the mounted relay identifies itself as "inner"
    And the mounted relay received "Bearer inner-client", not the outer client token
    And the mounted relay topology is returned
```

Extend `mountFeatureState` with a child `*Server`, `*httptest.Server`, and recorded child authorization. Build the child with `cfg.Swarm.AuthToken = "inner-client"`, register it in the outer registry as `KindRelay` with `Token: "inner-client"`, and issue both requests through `s.relay.URL + "/swarm/nodes/inner/swarm/..."`. Reuse `writeJSON`; close the child in `reset`.

- [ ] **Step 6: Run the BDD scenario**

Run:

```bash
go test -tags=swarm ./external/swarm -run TestSwarmMountFeature -count=1 -v
```

Expected: the new scenario passes through two auth gates; all existing mount security scenarios remain green.

- [ ] **Step 7: Document the fallback without changing the schema shape**

Update the `swarm.join[].token` paragraph in `docs/operate/swarm.md`: an explicit token remains recommended and wins; for a relay only, an omitted value falls back to that relay's configured `swarm.auth_token`. State that agents receive no fallback.

- [ ] **Step 8: Commit the transit slice**

```bash
git add features/swarm_mount.feature external/swarm/bdd_mount_test.go internal/swarm/joinset.go internal/swarm/swarm_test.go docs/operate/swarm.md
git commit -m "fix(swarm): carry relay credentials through transit mounts"
```

## Task 2: Add the browser-only layout preference

**Files:**
- Create: `external/ui/src/ui/swarm/layoutMode.ts`
- Create: `external/ui/src/ui/swarm/layoutMode.test.ts`
- Modify: `external/ui/src/ui/swarm/SwarmView.tsx`

- [ ] **Step 1: Write the failing persistence tests**

```ts
import { describe, expect, it } from "vitest";
import {
  SWARM_LAYOUT_STORAGE_KEY,
  readSwarmLayoutMode,
  writeSwarmLayoutMode,
} from "./layoutMode";

class MapStorage {
  private readonly values = new Map<string, string>();
  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }
  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }
}

describe("swarm layout mode", () => {
  it("defaults missing and unknown values to tree", () => {
    const storage = new MapStorage();
    expect(readSwarmLayoutMode(storage)).toBe("tree");
    storage.setItem(SWARM_LAYOUT_STORAGE_KEY, "radial");
    expect(readSwarmLayoutMode(storage)).toBe("tree");
  });

  it("persists star in browser storage", () => {
    const storage = new MapStorage();
    writeSwarmLayoutMode("star", storage);
    expect(storage.getItem(SWARM_LAYOUT_STORAGE_KEY)).toBe("star");
    expect(readSwarmLayoutMode(storage)).toBe("star");
  });
});
```

Use the test's small `MapStorage` implementing only `getItem` and `setItem`; do not depend on global jsdom storage for the pure tests.

- [ ] **Step 2: Verify RED**

```bash
cd external/ui
npx vitest run src/ui/swarm/layoutMode.test.ts
```

Expected: module-not-found for `./layoutMode`.

- [ ] **Step 3: Implement the complete preference module**

```ts
export type SwarmLayoutMode = "tree" | "star";
export const SWARM_LAYOUT_STORAGE_KEY = "foxxycode_swarm_layout";

type LayoutStorage = Pick<Storage, "getItem" | "setItem">;

export function readSwarmLayoutMode(
  storage: LayoutStorage | undefined = typeof localStorage === "undefined" ? undefined : localStorage,
): SwarmLayoutMode {
  return storage?.getItem(SWARM_LAYOUT_STORAGE_KEY) === "star" ? "star" : "tree";
}

export function writeSwarmLayoutMode(
  mode: SwarmLayoutMode,
  storage: LayoutStorage | undefined = typeof localStorage === "undefined" ? undefined : localStorage,
): void {
  storage?.setItem(SWARM_LAYOUT_STORAGE_KEY, mode);
}
```

- [ ] **Step 4: Verify GREEN**

Run the same Vitest command. Expected: two tests pass.

- [ ] **Step 5: Wire state into `SwarmView` without rendering controls yet**

Use lazy initialization and one setter:

```ts
const [layoutMode, setLayoutMode] = useState<SwarmLayoutMode>(readSwarmLayoutMode);
const chooseLayoutMode = (next: SwarmLayoutMode) => {
  setLayoutMode(next);
  writeSwarmLayoutMode(next);
};
```

Pass `layoutMode` and `onLayoutModeChange={chooseLayoutMode}` to `TopologyGraph`. The next task adds those props, so keep this edit in the working tree until Task 3 is green rather than committing uncompilable code.

## Task 3: Implement deterministic rooted force layout

**Files:**
- Create: `external/ui/src/ui/swarm/forceLayout.ts`
- Create: `external/ui/src/ui/swarm/forceLayout.test.ts`
- Modify: `external/ui/src/ui/swarm/layout.ts`

- [ ] **Step 1: Export shared node geometry from `layout.ts`**

Change `halfWidth` and `halfHeight` to exported `nodeHalfWidth` and `nodeHalfHeight`, updating their existing callers. No behavior changes.

- [ ] **Step 2: Write failing force-layout tests**

Use the existing `chain` and ring-shaped fixtures from `layout.test.ts`. Assert:

```ts
const one = layoutTopologyStar(chain, { client: { name: "laptop" } });
const two = layoutTopologyStar(chain, { client: { name: "laptop" } });
expect(one).toEqual(two);
const root = one.nodes.find((n) => n.uuid === CLIENT_UUID)!;
expect(root.y).toBe(Math.min(...one.nodes.map((n) => n.y)));
expect(one.nodes.filter((n) => n.uuid !== root.uuid).every((n) => n.y > root.y)).toBe(true);
expect(one.edges.map((e) => e.id).sort()).toEqual(
  layoutTopology(chain, { client: { name: "laptop" } }).edges.map((e) => e.id).sort(),
);
expect(overlappingPairs(one.nodes)).toEqual([]);
expect(Number.isFinite(one.width) && Number.isFinite(one.height)).toBe(true);
```

Add cases for an empty relay, a ring, a long label, and no synthetic client.

- [ ] **Step 3: Verify RED**

```bash
cd external/ui
npx vitest run src/ui/swarm/forceLayout.test.ts
```

Expected: module-not-found for `./forceLayout`.

- [ ] **Step 4: Implement the deterministic solver**

Create `layoutTopologyStar(topology, opts)` with this fixed pipeline:

```ts
const ITERATIONS = 180;
const SPRING = 0.018;
const REPULSION = 18_000;
const DOWNWARD = 0.012;
const DAMPING = 0.82;
const GAP = 24;

export function layoutTopologyStar(
  topology: SwarmTopology,
  opts: LayoutOptions = {},
): TopologyLayout {
  const base = layoutTopology(topology, opts);
  const rootUUID = opts.client ? CLIENT_UUID : topology.root.uuid;
  const state = seedNodes(base.nodes, rootUUID);
  for (let i = 0; i < ITERATIONS; i += 1) {
    applyPairRepulsion(state, REPULSION);
    applyEdgeSprings(state, base.edges, SPRING);
    applyDownwardBias(state, rootUUID, DOWNWARD);
    integrate(state, rootUUID, DAMPING);
    separateCollisions(state, rootUUID, GAP);
  }
  return normaliseLayout(base, state, rootUUID);
}
```

Implementation rules:

- `seedNodes` hashes UUID with a 32-bit FNV-1a helper and uses base route depth only as an initial downward bias.
- Store `{x, y, vx, vy}` per UUID; never call `Math.random`.
- Pair repulsion iterates sorted UUID pairs so map insertion order cannot change the result.
- Edge springs resolve endpoints by UUID and use a 170 px target length.
- `applyDownwardBias` adds a small positive y force proportional to `Math.max(1, depth + (opts.client ? 1 : 0))`, not a fixed row coordinate.
- Pin the root velocity and position on every iteration.
- `separateCollisions` uses `nodeHalfWidth`, `nodeHalfHeight`, and `GAP`, moving both nodes except the root.
- `normaliseLayout` shifts all nodes into positive coordinates with 48 px top/bottom and 56 px side padding, recenters the pinned root horizontally, rebuilds edge endpoints from the new node map, and returns `tiers: []`.
- Round final coordinates to one decimal place to keep snapshots stable.

- [ ] **Step 5: Verify force layout GREEN and tree regressions**

```bash
cd external/ui
npx vitest run src/ui/swarm/forceLayout.test.ts src/ui/swarm/layout.test.ts
```

Expected: all new tests and the existing 26 tree-layout tests pass.

- [ ] **Step 6: Commit layout preference and force layout**

```bash
git add external/ui/src/ui/swarm/layout.ts external/ui/src/ui/swarm/forceLayout.ts external/ui/src/ui/swarm/forceLayout.test.ts external/ui/src/ui/swarm/layoutMode.ts external/ui/src/ui/swarm/layoutMode.test.ts
git commit -m "feat(ui): add deterministic swarm star layout"
```

## Task 4: Build pure canvas camera arithmetic

**Files:**
- Create: `external/ui/src/ui/swarm/graphViewport.ts`
- Create: `external/ui/src/ui/swarm/graphViewport.test.ts`

- [ ] **Step 1: Write failing fit, zoom, and pan tests**

Cover these exact values:

```ts
const graph = { x: 0, y: 0, width: 1000, height: 500 };
const viewport = { width: 500, height: 300 };
const fitted = fitCamera(graph, viewport, 20);
expect(fitted.scale).toBeCloseTo(0.46);
expect(screenPoint(fitted, { x: 500, y: 250 })).toEqual({ x: 250, y: 150 });

const zoomed = zoomCameraAt(fitted, 2, { x: 125, y: 100 }, graph, viewport, 20);
expect(graphPoint(zoomed, { x: 125, y: 100 })).toEqual(
  graphPoint(fitted, { x: 125, y: 100 }),
);
expect(zoomed.scale).toBeCloseTo(fitted.fitScale * 2);

const panned = panCamera(zoomed, 10_000, -10_000, graph, viewport, 20);
expect(cameraShowsGraphEdge(panned, graph, viewport, 20)).toBe(true);
```

Also assert that fit never scales a small graph above 1, zoom never drops below `fitScale`, zoom caps at `fitScale * 3`, and invalid/zero dimensions return a finite identity camera.

- [ ] **Step 2: Verify RED**

```bash
cd external/ui
npx vitest run src/ui/swarm/graphViewport.test.ts
```

Expected: module-not-found for `./graphViewport`.

- [ ] **Step 3: Implement the pure camera API**

```ts
export type Point = { x: number; y: number };
export type Size = { width: number; height: number };
export type Bounds = Point & Size;
export type GraphCamera = {
  x: number;
  y: number;
  scale: number;
  fitScale: number;
  userAdjusted: boolean;
};

export function fitCamera(bounds: Bounds, viewport: Size, padding: number): GraphCamera;
export function zoomCameraAt(
  camera: GraphCamera,
  factor: number,
  focus: Point,
  bounds: Bounds,
  viewport: Size,
  padding: number,
): GraphCamera;
export function panCamera(
  camera: GraphCamera,
  dx: number,
  dy: number,
  bounds: Bounds,
  viewport: Size,
  padding: number,
): GraphCamera;
export function clampCamera(
  camera: GraphCamera,
  bounds: Bounds,
  viewport: Size,
  padding: number,
): GraphCamera;
export function graphPoint(camera: GraphCamera, screen: Point): Point;
export function screenPoint(camera: GraphCamera, graph: Point): Point;
```

Use `nextScale = clamp(camera.scale * factor, camera.fitScale, camera.fitScale * 3)`. Preserve the focus with `x = focus.x - (focus.x - camera.x) * nextScale / camera.scale`, then clamp. When a scaled graph is smaller than an axis, center it on that axis; otherwise keep at least `padding` pixels reachable at each edge.

- [ ] **Step 4: Verify GREEN**

Run the same test. Expected: all camera tests pass.

- [ ] **Step 5: Commit the camera math**

```bash
git add external/ui/src/ui/swarm/graphViewport.ts external/ui/src/ui/swarm/graphViewport.test.ts
git commit -m "feat(ui): add swarm canvas camera math"
```

## Task 5: Integrate layout controls and image-style canvas gestures

**Files:**
- Create: `external/ui/src/ui/swarm/useGraphViewport.ts`
- Modify: `external/ui/src/ui/swarm/TopologyGraph.tsx`
- Modify: `external/ui/src/ui/swarm/SwarmView.tsx`
- Modify: `external/ui/src/ui/swarm/SwarmView.test.tsx`

- [ ] **Step 1: Write failing component tests**

Add named tests for:

```ts
it("starts as a tree and remembers the star layout in this browser", ...)
it("fits again when the layout mode changes", ...)
it("zooms around the pointer without scrolling the page", ...)
it("pans after four pixels without entering the node", ...)
it("keeps a click on a node when the pointer did not become a drag", ...)
it("keeps the manual camera across an unchanged topology poll", ...)
it("offers keyboard zoom and fit controls with accessible names", ...)
```

Stub `ResizeObserver` with the existing Vitest pattern and give `.swarm-graph-viewport` a deterministic `getBoundingClientRect`. Assert `localStorage.foxxycode_swarm_layout`, `aria-pressed`, the graph group's `transform`, `preventDefault` on wheel, and the callback count.

- [ ] **Step 2: Verify RED**

```bash
cd external/ui
npx vitest run src/ui/swarm/SwarmView.test.tsx -t 'starts as a tree|fits again|zooms around|pans after|keeps a click|manual camera|keyboard zoom'
```

Expected: tests fail because layout controls, transformed graph group, and gesture handling do not exist.

- [ ] **Step 3: Implement `useGraphViewport`**

Use this public hook shape:

```ts
export function useGraphViewport(opts: {
  bounds: Bounds;
  resetKey: string;
  padding?: number;
}): {
  viewportRef: RefObject<HTMLDivElement | null>;
  camera: GraphCamera;
  transform: string;
  isPanning: boolean;
  fit: () => void;
  zoomIn: () => void;
  zoomOut: () => void;
  stageProps: {
    onPointerDown: PointerEventHandler<HTMLDivElement>;
    onPointerMove: PointerEventHandler<HTMLDivElement>;
    onPointerUp: PointerEventHandler<HTMLDivElement>;
    onPointerCancel: PointerEventHandler<HTMLDivElement>;
    onKeyDown: KeyboardEventHandler<HTMLDivElement>;
  };
  consumeGestureClick: () => boolean;
};
```

Implementation requirements:

- attach a non-passive native `wheel` listener to `viewportRef.current`;
- use the same `DRAG_SLOP_PX = 4`, pointer map, midpoint, and pointer-capture pattern as `ImageLightbox`;
- update `userAdjusted` only on wheel, pinch, or a drag beyond the slop;
- a `ResizeObserver` calls fit only while `userAdjusted` is false;
- changing `resetKey` always calls fit and clears `userAdjusted`;
- `consumeGestureClick` suppresses exactly the click ending a drag or pinch;
- `transform` is `translate(${camera.x} ${camera.y}) scale(${camera.scale})`.

- [ ] **Step 4: Select the requested layout in `TopologyGraph`**

Add props:

```ts
layoutMode: SwarmLayoutMode;
onLayoutModeChange: (mode: SwarmLayoutMode) => void;
resetKey: string;
```

Compute:

```ts
const layout = useMemo(
  () => layoutMode === "star"
    ? layoutTopologyStar(props.topology, clientName ? { client: { name: clientName } } : {})
    : layoutTopology(props.topology, clientName ? { client: { name: clientName } } : {}),
  [props.topology, clientName, layoutMode],
);
```

Render the spine only for `tree`. Replace the horizontal scroller with a focusable `.swarm-graph-viewport`, make the SVG fill it, and wrap edges/nodes in:

```tsx
<g className="swarm-graph-camera" transform={viewport.transform}>
  {layoutMode === "tree" ? <Spine ... /> : null}
  <Edges ... />
  <Nodes ... />
</g>
```

Before entering a node, call `viewport.consumeGestureClick()` and return when it reports a gesture.

- [ ] **Step 5: Render controls**

Inside `.swarm-graph-panel`, add:

```tsx
<div className="swarm-layout-switch" role="group" aria-label={t("swarm.layout.label")}>
  <button aria-pressed={layoutMode === "tree"} onClick={() => props.onLayoutModeChange("tree")} ...><TreeIcon /></button>
  <button aria-pressed={layoutMode === "star"} onClick={() => props.onLayoutModeChange("star")} ...><StarIcon /></button>
</div>
<div className="swarm-viewport-controls">
  <button onClick={viewport.zoomOut} ...>−</button>
  <button onClick={viewport.fit} ...><FitIcon /></button>
  <button onClick={viewport.zoomIn} ...>+</button>
</div>
```

Icons are inline SVGs. Do not use text glyphs for tree/star/fit.

- [ ] **Step 6: Complete `SwarmView` wiring**

Pass `layoutMode`, `chooseLayoutMode`, and a reset key built from the relay identity:

```ts
const graphResetKey = `${pictureKey(relayBase)}:${layoutMode}`;
```

Do not include the five-second topology object in the reset key.

- [ ] **Step 7: Verify GREEN**

Run the narrow component selection, then all Swarm UI tests:

```bash
cd external/ui
npx vitest run src/ui/swarm/SwarmView.test.tsx -t 'starts as a tree|fits again|zooms around|pans after|keeps a click|manual camera|keyboard zoom'
npx vitest run src/ui/swarm
```

Expected: new interaction tests pass and the previous 73 tests remain green.

- [ ] **Step 8: Commit the interactive canvas**

```bash
git add external/ui/src/ui/swarm/useGraphViewport.ts external/ui/src/ui/swarm/TopologyGraph.tsx external/ui/src/ui/swarm/SwarmView.tsx external/ui/src/ui/swarm/SwarmView.test.tsx
git commit -m "feat(ui): add interactive swarm canvas"
```

## Task 6: Keep EnvironmentChip visible and enter full transit paths

**Files:**
- Modify: `external/ui/src/ui/App.tsx`
- Modify: `external/ui/src/ui/App.swarmNode.test.tsx`
- Modify: `external/ui/src/ui/swarm/SwarmView.tsx`
- Modify: `external/ui/src/ui/swarm/SwarmView.test.tsx`
- Modify: `external/ui/src/ui/env/swarmEnv.test.ts`

- [ ] **Step 1: Write the failing nested relay URL test**

In `swarmEnv.test.ts`:

```ts
it("opens a transit relay through every parent mount", () => {
  connectSwarmRelay(
    "http://outer.test",
    ["middle", "transit"],
    "outer-client",
    "transit",
  );
  expect(getEnv()).toMatchObject({
    mode: "remote",
    baseUrl: "http://outer.test/swarm/nodes/middle/swarm/nodes/transit",
    token: "outer-client",
    name: "transit",
  });
  expect(window.location.hash).toBe("#/swarm");
});
```

- [ ] **Step 2: Write the failing App and error-header tests**

Extend the topology fixture in `App.swarmNode.test.tsx` with a relay at route `middle/transit`. Assert clicking it calls:

```ts
expect(switched).toHaveBeenCalledWith(
  "relay",
  RELAY,
  ["middle", "transit"],
  "client",
  "transit",
);
```

Add a test that opens the map over a node and finds `composer-env-btn` in `.swarm-header-actions`.

In `SwarmView.test.tsx`, make `/swarm/info` fail and render a header slot; assert both the error and slot remain visible.

- [ ] **Step 3: Verify RED**

```bash
cd external/ui
npx vitest run src/ui/env/swarmEnv.test.ts src/ui/App.swarmNode.test.tsx src/ui/swarm/SwarmView.test.tsx -t 'transit relay|environment selector|header slot'
```

Expected: the URL test may document already-correct path composition; the App/header tests fail because the header slot is conditional and the early error return omits it. At least one asserted user-visible behavior must be RED before implementation.

- [ ] **Step 4: Pass `EnvironmentChip` unconditionally on the Swarm screen**

In `App.tsx`, separate `rootCurrent` from `headerSlot`:

```tsx
<SwarmView
  ...
  headerSlot={<EnvironmentChip />}
  {...(atSwarmRoot ? { rootCurrent: true } : {})}
/>
```

Do not add a second selector component or map-specific environment list.

- [ ] **Step 5: Remove `SwarmView`'s headerless error return**

Render one common `<header className="swarm-header">` before loading/error/topology branches. When `!info && !loading`, render the localized error beneath the header and skip the graph/search results. This keeps recovery reachable without showing remembered topology as the new relay's.

- [ ] **Step 6: Verify GREEN**

Run the same focused command, then:

```bash
cd external/ui
npx vitest run src/ui/App.swarmNode.test.tsx src/ui/swarm/SwarmView.test.tsx src/ui/env/swarmEnv.test.ts
```

Expected: all focused files pass.

- [ ] **Step 7: Commit navigation and recovery**

```bash
git add external/ui/src/ui/App.tsx external/ui/src/ui/App.swarmNode.test.tsx external/ui/src/ui/swarm/SwarmView.tsx external/ui/src/ui/swarm/SwarmView.test.tsx external/ui/src/ui/env/swarmEnv.test.ts
git commit -m "fix(ui): keep swarm navigation reachable"
```

## Task 7: Add localized controls, CSS contracts, and UI BDD

**Files:**
- Modify: `external/ui/src/ui/i18n/messages/en.ts`
- Modify: `external/ui/src/ui/i18n/messages/ru.ts`
- Modify: `external/ui/src/styles.css`
- Modify: `external/ui/src/ui/swarm/SwarmView.test.tsx`
- Modify: `features/swarm_web_ui.feature`
- Modify: `external/ui/bdd_swarm_ui_test.go`

- [ ] **Step 1: Add failing localization and CSS assertions**

Add dictionary keys in tests before values exist:

```text
swarm.layout.label
swarm.layout.tree
swarm.layout.star
swarm.viewport.zoomIn
swarm.viewport.zoomOut
swarm.viewport.fit
```

Extend `SwarmView.test.tsx` to read `styles.css` and assert:

- `.swarm-graph-viewport` has `overflow: hidden`, `touch-action: none`, and a bounded height;
- `.swarm-layout-switch` is top-left;
- `.swarm-viewport-controls` is bottom-right;
- buttons use shared glass tokens;
- `.is-panning` uses `cursor: grabbing`;
- reduced motion disables control/camera transitions;
- no new width media query is introduced.

- [ ] **Step 2: Verify RED**

```bash
cd external/ui
npx vitest run src/ui/i18n/messagesParity.test.ts src/ui/swarm/SwarmView.test.tsx -t 'canvas controls|dictionary'
```

Expected: missing dictionary keys and CSS contracts fail.

- [ ] **Step 3: Add English and Russian copy**

Use:

```ts
// English
"swarm.layout.label": "Graph layout",
"swarm.layout.tree": "Tree layout",
"swarm.layout.star": "Star layout",
"swarm.viewport.zoomIn": "Zoom in",
"swarm.viewport.zoomOut": "Zoom out",
"swarm.viewport.fit": "Fit graph",

// Russian
"swarm.layout.label": "Расположение графа",
"swarm.layout.tree": "Дерево",
"swarm.layout.star": "Звезда",
"swarm.viewport.zoomIn": "Приблизить",
"swarm.viewport.zoomOut": "Отдалить",
"swarm.viewport.fit": "Уместить граф",
```

- [ ] **Step 4: Implement the CSS**

Keep styles in the existing Swarm block. Use absolute overlay controls with `z-index: 2`; reserve no graph coordinates for them. Use a bounded canvas such as:

```css
.swarm-graph-viewport {
  position: relative;
  height: clamp(360px, 64dvh, 760px);
  overflow: hidden;
  overscroll-behavior: contain;
  touch-action: none;
  border: 1px solid var(--foxxycode-glass-panel-border);
  border-radius: var(--foxxycode-glass-panel-radius, var(--radius));
  background: var(--swarm-ground);
  cursor: grab;
}
.swarm-graph-viewport.is-panning { cursor: grabbing; }
.swarm-graph { width: 100%; height: 100%; }
.swarm-layout-switch { position: absolute; top: 12px; left: 12px; z-index: 2; }
.swarm-viewport-controls { position: absolute; right: 12px; bottom: 12px; z-index: 2; }
```

Controls use 36 px square hit areas on desktop and the existing stacked-shell rule may increase them to 40 px without introducing a new breakpoint. Keep the screen-reader summary visually hidden and the legend in HTML below the viewport.

- [ ] **Step 5: Extend the UI feature**

Append to `features/swarm_web_ui.feature`:

```gherkin
  Scenario: The browser remembers the free graph layout
    Then the Swarm canvas starts as a tree, can switch to a star, and keeps that choice in this browser
```

Map this step in `external/ui/bdd_swarm_ui_test.go` to the exact Vitest test name `SwarmView starts as a tree and remembers the star layout in this browser`.

- [ ] **Step 6: Verify GREEN, wording, and BDD**

```bash
cd external/ui
npx vitest run src/ui/i18n/messagesParity.test.ts src/ui/i18n/ruWording.test.ts src/ui/swarm/SwarmView.test.tsx
cd ../..
go test -tags=http,ui ./external/ui -run TestSwarmWebUIFeature -count=1 -v
git grep -n -i 'агентск' -- external/ui/src/ui/i18n/messages/ru.ts || true
git grep -n -i 'сабагент' -- external/ui/src/ui/i18n/messages/ru.ts || true
```

Expected: tests pass and the two forbidden-word searches return no matches.

- [ ] **Step 7: Commit UI polish and BDD**

```bash
git add external/ui/src/ui/i18n/messages/en.ts external/ui/src/ui/i18n/messages/ru.ts external/ui/src/styles.css external/ui/src/ui/swarm/SwarmView.test.tsx features/swarm_web_ui.feature external/ui/bdd_swarm_ui_test.go
git commit -m "feat(ui): finish swarm canvas controls"
```

## Task 8: Update documentation and capture the real UI

**Files:**
- Modify: `DESIGN.md`
- Modify: `docs/operate/swarm.md`
- Modify: `docs/surfaces/web-ui.md`
- Create: `docs/assets/swarm/map-star-canvas-dark-1280.png`
- Modify: `docs/assets/INDEX.md`
- Modify: embedded UI assets generated by `make build TAGS="http ui"`

- [ ] **Step 1: Update the authoritative design contract**

Document in `DESIGN.md` under **Swarm screen**:

- tree default and `foxxycode_swarm_layout` localStorage persistence;
- deterministic star root and no rigid tiers;
- fit reset rules versus polling preservation;
- wheel, pinch, drag, `+`, `-`, and `0`;
- top-left layout switch, bottom-right camera controls, top-right EnvironmentChip;
- drag slop preventing node activation;
- transit relay credential substitution and recovery header.

- [ ] **Step 2: Update user-facing docs**

In `docs/operate/swarm.md`, explain choosing layouts, canvas controls, and transit relay entry. In `docs/surfaces/web-ui.md`, add the functional checklist and localStorage key. Keep the security wording: explicit `join.token` wins and is recommended; relay auth fallback applies only when it is omitted.

- [ ] **Step 3: Build the embedded UI**

```bash
make build TAGS="http ui"
```

Expected: Vite build and Go build succeed; generated embedded assets reflect the new UI.

- [ ] **Step 4: Run the real surface for screenshots**

Start the project server in the background with a temporary test config or use the existing Vite UI with Playwright route interception for `/swarm/info`, `/swarm/nodes`, `/swarm/sessions`, `/swarm/topology`, `/foxxycode/config`, and `/foxxycode/info`. The rendered component must be the repository's real SPA, not a hand-written mockup.

At 1280 px dark theme:

1. open `#/swarm`;
2. select star mode;
3. confirm the whole graph is fitted;
4. capture `docs/assets/swarm/map-star-canvas-dark-1280.png`.

Also capture 390 px and before/after images for the PR description outside `docs/assets/` unless a documentation paragraph references them.

- [ ] **Step 5: Verify canvas behavior in the browser**

With Playwright at 1280 px and 390 px:

- wheel changes the camera transform and does not scroll the page;
- drag changes translation and a dragged node does not switch environments;
- a plain node click still switches;
- fit restores the complete graph;
- the top-right environment menu opens inside the viewport;
- `document.documentElement.scrollWidth === document.documentElement.clientWidth`;
- light and dark controls remain readable.

- [ ] **Step 6: Regenerate and check documentation**

```bash
make docs
make docs-check
```

Expected: generated asset inventory includes the new screenshot, every link resolves, and no asset is orphaned.

- [ ] **Step 7: Commit docs and built assets**

```bash
git add DESIGN.md docs/operate/swarm.md docs/surfaces/web-ui.md docs/assets/swarm/map-star-canvas-dark-1280.png docs/assets/INDEX.md external/ui
git commit -m "docs: explain the interactive swarm canvas"
```

## Task 9: Cross-review, verification, and cleanup

**Files:**
- Review all changes from base `537a1d1b` to branch HEAD.
- Modify only files implicated by confirmed review findings.

- [ ] **Step 1: Run focused suites before review**

```bash
cd external/ui
npx vitest run src/ui/swarm src/ui/App.swarmNode.test.tsx src/ui/env/swarmEnv.test.ts src/ui/i18n/messagesParity.test.ts
cd ../..
go test ./internal/swarm -run 'TestStartJoinsChoosesTheCredentialAParentUses' -count=1
go test -tags=swarm ./external/swarm -run 'TestSwarmMountFeature|TestMount' -count=1
go test -tags=http,ui ./external/ui -run TestSwarmWebUIFeature -count=1
```

Expected: all focused suites pass.

- [ ] **Step 2: Request independent cross-review**

Dispatch a fresh reviewer with:

- description: deterministic star layout, viewport gestures, always-visible EnvironmentChip, transit-relay credential fallback;
- requirements: `docs/plans/swarm-star-canvas.md` and this implementation plan;
- base SHA: `537a1d1b`;
- head SHA: current `git rev-parse HEAD`.

Require findings grouped as Critical, Important, and Minor, with special checks for credential leakage, mount control-plane widening, non-deterministic layout, gesture/click races, polling resets, accessibility, localization parity, and missing docs.

- [ ] **Step 3: Verify each review finding before changing code**

For every finding, reproduce it with the narrowest existing or new test. Fix Critical and Important findings before proceeding. Apply Minor findings when they improve correctness or clarity without unrelated refactoring. Record technically incorrect findings with evidence rather than changing working code.

- [ ] **Step 4: Run full project verification**

```bash
make build TAGS="http ui"
make test
make docs-check
make lint
```

Expected: all commands exit zero. If `TestBackgroundWakeWebUIFeature` repeats the clean-baseline timeout, run its exact isolated command, capture the output, and report it separately rather than attributing it to Swarm changes. Do not call the full suite green unless `make test` itself exits zero.

- [ ] **Step 5: Run race checks only if concurrent Go code changed beyond token selection**

If the final fix modifies proxy, registry, tunnel, or goroutine code rather than only `StartJoins` token selection:

```bash
make test-race
```

Expected: no races.

- [ ] **Step 6: Confirm clean branch state and commit review fixes**

```bash
git diff --check
git status --short
git log --oneline 537a1d1b..HEAD
```

If review fixes exist:

```bash
git add <only-reviewed-files>
git commit -m "fix: address swarm canvas review"
```

Expected: no uncommitted files except intentionally untracked PR screenshots outside the repository.
