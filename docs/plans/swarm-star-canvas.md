# Swarm Star Canvas Design

**Date:** 2026-09-28
**Status:** Approved in design review
**Scope:** Swarm web UI canvas, environment navigation, and transit-relay connectivity

## Context

The Swarm screen currently renders a hand-written SVG topology in strict hop tiers. The tiered tree remains useful for reading route depth, but larger or cyclic swarms become a wide, linear structure that is difficult to explore. The canvas also scrolls only horizontally: it cannot zoom, pan freely, or fit the complete graph into the available viewport.

The screen exposes the composer's environment selector only when the relay itself is the page's home. A map opened over an agent does not carry that selector in its header. Clicking a relay behind another relay is intended to open the nested relay's own map, but this path lacks an end-to-end test through both authentication gates and is reported not to work reliably.

## Goals

1. Keep the existing tiered tree as the default layout.
2. Add a browser-persisted `star` layout that renders a deterministic force-directed graph rooted at the computer serving the UI.
3. Add fit, zoom, pinch, and pan behavior matching the shared image lightbox.
4. Show the existing `EnvironmentChip` at the top right of every Swarm screen.
5. Preserve direct navigation by clicking agents and relays on the graph.
6. Make a relay behind another relay enterable through the established mount and per-hop credential chain.
7. Keep the configuration and public route set unchanged.

## Non-goals

- Replacing the current SVG renderer with a graph library.
- Adding a YAML setting or server-side preference for the layout.
- Making unreachable or route-less nodes enterable by guessing a route.
- Exposing child-relay credentials to the browser.
- Allowing relay registration, tunnel creation, node eviction, or relay settings writes through a mount.
- Adding a node details panel.

## Layout modes and persistence

The UI defines one explicit mode type:

```ts
type SwarmLayoutMode = "tree" | "star";
```

The current `layoutTopology` behavior is the `tree` mode and remains unchanged. The new `star` mode uses a separate pure layout module and returns the same `TopologyLayout` shape. The shared renderer therefore keeps node cards, agent discs, wires, route highlighting, activity state, tooltips, keyboard activation, and screen-reader summary behavior.

The selected mode is stored in browser `localStorage` under `foxxycode_swarm_layout`. Missing, malformed, and unknown values resolve to `tree`. The preference is never read from or written to FoxxyCode configuration.

A two-button segmented control sits in the top-left corner of the graph canvas:

- a tree icon selects `tree`;
- a star icon selects `star`;
- each button has a localized tooltip and accessible name;
- `aria-pressed` communicates the selected mode.

## Deterministic force layout

The force layout is synchronous and runs for a fixed number of iterations. It does not animate toward equilibrium in the browser. This avoids a graph that drifts after every five-second topology poll and makes coordinates deterministic in unit tests.

### Root

When the map carries the synthetic client node, that node is pinned at the horizontal center of the top edge. It represents the computer from which the UI is opened. The topology root relay is connected beneath it through the existing synthetic edge.

When no client node is available, `topology.root` is pinned at the top center.

Every other node is constrained below the root. A weak downward force encourages a rooted composition without imposing rigid hop rows.

### Initial positions and forces

- Stable initial positions are derived from node UUIDs, route depth, and a deterministic hash.
- Linked nodes attract one another toward a target edge length.
- All nodes repel one another.
- A weak centering force keeps the graph around the vertical axis beneath the root.
- A collision pass uses the actual relay-card and agent-disc bounds plus label clearance.
- The root remains fixed throughout all iterations.
- Empty and single-node topologies produce finite bounds.
- Rings and alternate edges participate as edges but cannot cause unbounded traversal or iteration.

The same topology, client identity, and layout mode always produce the same coordinates. A new node may deterministically move existing nodes, but the layout never depends on timer timing or previous simulation state.

## Canvas viewport

Viewport arithmetic lives in a focused pure module. The renderer owns DOM events and applies the resulting translation and scale to a shared SVG graph group.

### Fit model

The camera begins in semantic `fit` state:

- graph bounds are fitted into the visible canvas with padding;
- a graph smaller than the viewport is not enlarged above its natural scale;
- fit is the minimum zoom level;
- the maximum zoom is three times the fitted scale.

Automatic fit occurs:

1. when topology first becomes available;
2. after switching `tree` and `star`;
3. after entering another relay;
4. after panel resize while the user has not manually adjusted the camera.

Ordinary polling preserves manual zoom and pan. If the user has not touched the camera, a topology change may recompute fit so newly added nodes remain visible.

### Input behavior

The interaction follows `ImageLightbox` conventions:

- wheel and trackpad zoom continuously around the pointer;
- a two-finger pinch zooms around the midpoint;
- pointer drag pans the canvas;
- `+` and `-` change zoom;
- `0` returns to fit;
- visible zoom-in, zoom-out, and fit controls sit at the bottom right of the canvas;
- zoom controls have localized labels and tooltips.

A pointer press may start on empty canvas or a node. Movement up to and including four pixels remains a click. Movement beyond four pixels becomes a pan and suppresses the node activation click. Pointer capture keeps a drag active after leaving the original element.

Pan is clamped so the graph cannot be lost completely outside the viewport. Reduced-motion mode removes transitions without removing any controls or state.

## Environment selector and node navigation

The existing `EnvironmentChip` is rendered in the Swarm header on every Swarm screen:

- relay home;
- a map opened over an agent;
- local mode when a swarm is available;
- loading and error states where topology is not available.

The component remains the same source of truth as the composer selector. It lists Local, configured remotes, reachable relays, and the agents those relays expose. Opening the menu refreshes configured remotes and probes their reachability as it does today. No map-specific station registry is introduced.

Graph navigation remains available in parallel:

- clicking an agent calls `connectSwarmNode`;
- clicking a relay calls `connectSwarmRelay`;
- clicking the active node does nothing;
- a node with no route remains visible but non-interactive;
- keyboard Enter and Space match pointer activation;
- a drag never activates a node.

## Transit relay contract

A browser enters a child relay through the parent's mount. It authenticates only to the outer relay:

```text
browser --Bearer outer-client--> outer relay
outer relay --Bearer child-node-token--> child relay
```

The mounted relay answers its existing read routes:

- `/swarm/info`;
- `/swarm/topology`;
- `/swarm/nodes`;
- `/swarm/sessions`;
- nested `/swarm/nodes/...` mounts.

For deeper chains, each relay removes the incoming authorization header and substitutes the registered token for the next hop. Browser credentials and downstream node credentials never cross their respective trust boundary.

The implementation begins with a real integration test using outer and child relay handlers. If the mounted info and topology requests already pass, the server proxy remains unchanged and the failure is reproduced in UI environment/path handling. If the test fails, the fix remains in the mount or topology merge layer and preserves per-hop credential substitution.

The full relay route is retained when constructing mounted URLs. For example:

```text
outer/swarm/nodes/middle/swarm/nodes/transit
```

Entering that environment opens `#/swarm` and probes the mounted relay as a relay rather than treating it as an agent.

## Security boundaries

A mount continues to refuse:

- `/swarm/register`;
- `/swarm/tunnel`;
- child node eviction;
- writes to a mounted relay's settings.

The outer relay owns the browser-facing CORS response. CORS headers returned by child relays are removed before the response reaches the browser.

Tokens must not appear in URLs, errors, rendered copy, screenshots, or newly persisted storage. Existing browser-only token storage used by `EnvironmentChip` is unchanged.

## Error handling and recovery

A failed transit hop produces a localized error that identifies the hop without printing credentials. The environment selector remains visible in the header so the operator can immediately return to Local or select another remote.

A failed switch must not display a remembered topology as though it belonged to the new relay. Route-less and offline relays remain non-interactive. An unauthorized mounted child is reported as a hop failure rather than prompting the browser for the child token: the parent relay is responsible for authenticating to its registered child.

## Testing strategy

### BDD

`features/swarm_web_ui.feature`, through `external/ui/bdd_swarm_ui_test.go`, covers the user-visible happy path:

- tree is the first-use default;
- selecting star renders the rooted free graph;
- the mode survives reopening in the same browser;
- the environment selector is present at the top right.

`features/swarm_mount.feature`, through `external/swarm/bdd_mount_test.go`, covers the transit-relay happy path:

- outer and child relays use different credentials;
- mounted child info and topology succeed with only the outer client token at the browser;
- the outer relay substitutes the child token;
- the mounted target identifies itself as the child relay and returns its topology.

### Unit tests

Force-layout tests cover determinism, root placement, downward constraint, collision clearance, retained edges, rings, and empty topologies.

Viewport tests cover fit bounds, scale limits, zoom focus, pinch midpoint, clamped pan, the drag slop, resize behavior, polling preservation, and fit resets.

Component and integration tests cover localStorage parsing, toggle accessibility, wheel cancellation, pointer gesture behavior, full nested relay paths, selector visibility in every state, current-node behavior, and route-less nodes.

Security regression tests retain mount denials for registration, tunnel establishment, child eviction, and relay settings writes.

### Browser verification

The running UI is checked with Playwright at 1280 px and 390 px. Verification includes:

- tree and star layouts;
- fit, wheel zoom, pointer pan, and node click;
- no page-level horizontal overflow;
- controls above the mobile backdrop;
- the environment menu opening from the header;
- a transit relay opening its own map;
- dark and light themes when control colors or tokens change.

## Documentation and assets

Update:

- `DESIGN.md`, **Swarm screen**;
- `docs/operate/swarm.md`, **The UI**;
- `docs/surfaces/web-ui.md`, **Swarm screen**;
- a real dark 1280 px star-layout screenshot under `docs/assets/swarm/`;
- generated `docs/assets/INDEX.md` through `make docs`.

The implementation does not add a configuration key or a route, so no config schema or OpenAPI change is expected.

## Verification commands

Narrow RED/GREEN commands are run after each test. Before completion:

```bash
make build TAGS="http ui"
make test
make docs-check
make lint
```

Run `make test-race` as well if the server fix changes concurrent proxy, tunnel, or registry code.

## Acceptance criteria

1. Tree remains the default layout and retains its current behavior.
2. Star layout is stored only in browser localStorage.
3. Star layout is deterministic, rooted at the local computer, and free of rigid hop rows.
4. Initial fit shows the complete graph.
5. Manual zoom and pan survive normal topology polling.
6. Wheel, pinch, drag, keyboard, and visible controls provide the agreed camera behavior.
7. EnvironmentChip is always available at the top right of the Swarm screen.
8. Agents and relays remain directly enterable from the graph.
9. A transit relay opens through its parent with per-hop credential substitution.
10. Mount control-plane and settings-write restrictions remain intact.
11. UI, Go, documentation, and lint gates pass.
12. The documentation contains a current screenshot of the real star canvas.
