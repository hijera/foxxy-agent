import { expect, test } from "vitest";
import {
  formatSeconds,
  pendingApprovalCount,
  shortDigest,
  showsSubagentTrustControl,
  subagentApprovalFacts,
  type SubagentCatalogEntry,
  type SubagentProjectTrust,
  type SubagentScope,
} from "./subagentCatalog";

function entry(over: Partial<SubagentCatalogEntry> = {}): SubagentCatalogEntry {
  return {
    name: "reviewer",
    description: "Reviews a diff.",
    scope: "project",
    builtin: false,
    hidden: false,
    trust: "needs_approval",
    trusted: false,
    needs_approval: true,
    ...over,
  };
}

test("the trust control is offered only for a project file under ask", () => {
  const scopes: SubagentScope[] = ["builtin", "user", "project"];
  const policies: SubagentProjectTrust[] = ["ask", "allow", "deny"];
  const offered: string[] = [];
  for (const scope of scopes) {
    for (const policy of policies) {
      const e = entry({ scope, builtin: scope === "builtin" });
      if (showsSubagentTrustControl(e, policy)) {
        offered.push(`${scope}/${policy}`);
      }
    }
  }
  // allow leaves no decision, deny never reads the files, and the operator's
  // own scopes are trusted by definition.
  expect(offered).toEqual(["project/ask"]);
});

test("a definition that declares nothing reports every bound as inherited", () => {
  const facts = subagentApprovalFacts(entry());
  const byLabel = Object.fromEntries(facts.map((f) => [f.labelKey, f]));
  expect(byLabel["settings.subagents.fact.model"]?.valueKey).toBe(
    "settings.subagents.fact.modelInherits",
  );
  expect(byLabel["settings.subagents.fact.tools"]?.valueKey).toBe(
    "settings.subagents.fact.toolsAll",
  );
  expect(byLabel["settings.subagents.fact.timeout"]?.valueKey).toBe(
    "settings.subagents.fact.timeoutDefault",
  );
  // Rows that only exist when declared stay out.
  expect(byLabel["settings.subagents.fact.denies"]).toBeUndefined();
  expect(byLabel["settings.subagents.fact.background"]).toBeUndefined();
  expect(byLabel["settings.subagents.fact.role"]).toBeUndefined();
});

test("declared bounds are reported verbatim", () => {
  const facts = subagentApprovalFacts(
    entry({
      model: "openai/gpt-4o",
      mode: "plan",
      permission_mode: "ask",
      tools: ["read", "grep"],
      disallowed_tools: ["run_command"],
      timeout_seconds: 600,
      max_turns: 12,
      background: true,
      role_bytes: 4210,
    }),
  );
  const byLabel = Object.fromEntries(facts.map((f) => [f.labelKey, f]));
  expect(byLabel["settings.subagents.fact.tools"]?.value).toBe("read, grep");
  expect(byLabel["settings.subagents.fact.denies"]?.value).toBe("run_command");
  expect(byLabel["settings.subagents.fact.permissions"]?.value).toBe("ask");
  expect(byLabel["settings.subagents.fact.timeout"]?.value).toBe("10m");
  expect(byLabel["settings.subagents.fact.maxTurns"]?.value).toBe("12");
  expect(byLabel["settings.subagents.fact.background"]?.valueKey).toBe(
    "settings.subagents.fact.backgroundAlways",
  );
  expect(byLabel["settings.subagents.fact.role"]?.value).toBe("4 KiB");
});

test("timeouts read as seconds under a minute and minutes above", () => {
  expect(formatSeconds(45)).toBe("45s");
  expect(formatSeconds(60)).toBe("1m");
  expect(formatSeconds(1800)).toBe("30m");
});

test("digests are shortened for display but never invented", () => {
  expect(shortDigest("9f2ca1b3d4e5f60718")).toBe("9f2ca1b3d4e5");
  expect(shortDigest(undefined)).toBe("");
});

test("pending approvals are counted off the trust decision", () => {
  expect(
    pendingApprovalCount([
      entry(),
      entry({ name: "ok", trust: "trusted", trusted: true, needs_approval: false }),
      entry({ name: "second" }),
    ]),
  ).toBe(2);
});
