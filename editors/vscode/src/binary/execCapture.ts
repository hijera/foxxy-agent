import { execFile } from "child_process";

/** Runs `cmd args` and returns its merged stdout+stderr, the way the IntelliJ
 *  plugin's `runCapture` does. A non-zero exit code still resolves: a lean
 *  `foxxycode http --help` prints its "not built" notice and may exit 1, and
 *  the caller wants that text, not an error. Rejects only when the process
 *  could not be run at all (ENOENT, EACCES, …) or when it exceeded `timeoutMs`.
 *  Pure Node; injected into `validateBinary` so the resolver stays testable. */
export function execCapture(
  cmd: string,
  args: string[],
  timeoutMs: number = 10_000,
): Promise<{ stdout: string }> {
  return new Promise((resolve, reject) => {
    execFile(
      cmd,
      args,
      { timeout: timeoutMs, windowsHide: true, maxBuffer: 4 * 1024 * 1024, encoding: "utf8" },
      (err, stdout, stderr) => {
        const e = err as (NodeJS.ErrnoException & { killed?: boolean }) | null;
        if (e && (e.killed || typeof e.code === "string")) {
          reject(new Error(e.killed ? `timed out after ${timeoutMs}ms` : e.message));
          return;
        }
        resolve({ stdout: String(stdout ?? "") + String(stderr ?? "") });
      },
    );
  });
}
