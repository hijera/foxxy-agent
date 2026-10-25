"""Plan guard-test runs for a port review; executes them only on request."""

import re
import subprocess

NO_TESTS = re.compile(r"no tests to run|no test files|\[no tests to run\]", re.IGNORECASE)


def package_of(path: str) -> str:
    directory = path.replace("\\", "/").rsplit("/", 1)[0] if "/" in path else ""
    return "./" + directory if directory else "."


def build_plan(divergences: list[dict]) -> dict:
    commands = [{"command": ["go", "test", "./internal/forkguard", "-count=1"],
                 "packages": ["./internal/forkguard"], "tests": [],
                 "reason": "Registry integrity, not behavior."}]
    for entry in divergences:
        packages = sorted({package_of(path) for path in entry["matched_files"]})
        if not entry["tests"]:
            commands.append({"command": ["go", "test", *packages, "-count=1"],
                             "packages": packages, "tests": [],
                             "reason": f"Divergence {entry['id']} names no guard tests; run the package suite."})
            continue
        pattern = "^(" + "|".join(entry["tests"]) + ")$"
        commands.append({"command": ["go", "test", *packages, "-run", pattern, "-count=1"],
                         "packages": packages, "tests": list(entry["tests"]),
                         "reason": f"Guard tests for divergence {entry['id']}."})
    return {"commands": commands}


def run_plan(commands: list[dict], repo, runner=None) -> list[dict]:
    results = []
    for item in commands:
        if runner is None:
            try:
                done = subprocess.run(item["command"], cwd=str(repo), capture_output=True, text=True,
                                      encoding="utf-8", errors="replace")
                outcome = (done.returncode, done.stdout, done.stderr)
            except OSError as exc:
                results.append({**item, "status": "unavailable", "output": str(exc)})
                continue
        else:
            outcome = runner(item["command"])
        code, out, err = outcome
        output = (out or "") + (err or "")
        if code == 0 and NO_TESTS.search(output):
            status = "no_tests"
        elif code == 0:
            status = "passed"
        else:
            status = "failed"
        results.append({**item, "status": status, "output": output[-2000:]})
    return results


def verify_markdown(report: dict) -> str:
    lines = ["# Upstream guard-test plan", "", f"Base: `{report['base']}`", ""]
    lines.extend("- " + warning for warning in report["warnings"])
    lines.extend(["", f"Divergences in scope: "
                  + (", ".join(report["divergences"]) or "none"), ""])
    lines.extend(["## Commands", ""])
    for item in report["commands"]:
        status = f" [{item['status']}]" if "status" in item else ""
        lines.append(f"- `{ ' '.join(item['command']) }`{status}")
        lines.append(f"  {item['reason']}")
        if item.get("tests"):
            lines.append("  Tests: " + ", ".join(item["tests"]))
    if report.get("tests_executed"):
        lines.extend(["", f"Result: **{report['result']}**"])
    else:
        lines.extend(["", "No tests were executed; run the commands above before accepting the port."])
    return "\n".join(lines) + "\n"
