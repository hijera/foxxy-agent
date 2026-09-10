import { describe, it, expect } from "vitest";
import * as path from "path";
import { execCapture } from "../src/binary/execCapture";

const node = process.execPath;

describe("execCapture", () => {
  it("merges stdout and stderr and resolves on a non-zero exit code", async () => {
    const { stdout } = await execCapture(node, [
      "-e",
      'process.stdout.write("out"); process.stderr.write("err"); process.exit(3);',
    ]);
    expect(stdout).toContain("out");
    expect(stdout).toContain("err");
  });

  it("rejects when the executable cannot be started", async () => {
    const missing = path.join(__dirname, "definitely-missing-binary-" + Date.now());
    await expect(execCapture(missing, ["-v"])).rejects.toThrow();
  });

  it("rejects when the process exceeds the timeout", async () => {
    await expect(
      execCapture(node, ["-e", "setTimeout(function(){}, 5000)"], 200),
    ).rejects.toThrow(/timed out/);
  });
});
