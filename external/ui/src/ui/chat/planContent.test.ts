import { describe, expect, it } from "vitest";

import { planEditorBody, splitPlanFileContent } from "./planContent";

describe("splitPlanFileContent", () => {
  it("splits yaml frontmatter from markdown body", () => {
    const raw = `---
name: Demo
overview: Hi
---
# Title

Steps here
`;
    const { frontmatter, body } = splitPlanFileContent(raw);
    expect(frontmatter).toContain("name: Demo");
    expect(body).toBe("# Title\n\nSteps here");
  });

  it("splits body when todos use indented list items", () => {
    const raw =
      "---\nname: Meta title\noverview: Short overview\ntodos:\n  - content: Step A\n---\n# New body\n\nDone.\n";
    const { body } = splitPlanFileContent(raw);
    expect(body).toBe("# New body\n\nDone.");
  });

  it("returns full text as body when no frontmatter", () => {
    const raw = "# Only body\n";
    const { frontmatter, body } = splitPlanFileContent(raw);
    expect(frontmatter).toBe("");
    expect(body).toBe("# Only body");
  });
});

describe("planEditorBody", () => {
  it("prefers body field from API", () => {
    const full = "---\nname: X\n---\n# Hidden\n";
    expect(planEditorBody(full, "# Visible\n")).toBe("# Visible\n");
  });
});
