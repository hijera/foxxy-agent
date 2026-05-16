#!/usr/bin/env python3
"""ACP plan mode: plan_write design file, then run via session/prompt _meta (Coddy extension)."""
from __future__ import annotations

import json
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "shared"))

from acp_rpc import acp_roundtrip, start_coddy_acp  # noqa: E402

PLAN_CONTENT = """---
name: Hello plan
overview: Demo design plan for ACP
todos:
  - content: Say hello
    status: pending
---
## Steps

1. Reply with a short greeting only.
"""


def main() -> int:
    proc, lines = start_coddy_acp()
    try:
        init = acp_roundtrip(
            lines,
            proc,
            "initialize",
            {"protocolVersion": 1, "clientInfo": {"name": "acp_plan_files", "version": "1"}},
        )
        if "error" in init:
            print("initialize error", init, file=sys.stderr)
            return 1

        sess = acp_roundtrip(
            lines,
            proc,
            "session/new",
            {"cwd": os.getcwd(), "mcpServers": []},
        )
        if "error" in sess:
            print("session/new error", sess, file=sys.stderr)
            return 1
        sid = sess["result"]["sessionId"]

        acp_roundtrip(
            lines,
            proc,
            "session/set_config_option",
            {"sessionId": sid, "configId": "mode", "value": "plan"},
        )

        plan_prompt = acp_roundtrip(
            lines,
            proc,
            "session/prompt",
            {
                "sessionId": sid,
                "prompt": [
                    {
                        "type": "text",
                        "text": (
                            "Create a design plan slug demo-plan for a hello task. "
                            "Use plan_write with the provided structure."
                        ),
                    }
                ],
            },
            timeout=120,
        )
        if "error" in plan_prompt:
            print("plan prompt error", plan_prompt, file=sys.stderr)
            return 1

        # Portable run: switch to agent and prompt with _meta runPlanSlug (Coddy).
        acp_roundtrip(
            lines,
            proc,
            "session/set_config_option",
            {"sessionId": sid, "configId": "mode", "value": "agent"},
        )
        run = acp_roundtrip(
            lines,
            proc,
            "session/prompt",
            {
                "sessionId": sid,
                "prompt": [{"type": "text", "text": "Implement the plan."}],
                "_meta": {"coddy.dev/runPlanSlug": "demo-plan"},
            },
            timeout=120,
        )
        if "error" in run:
            print("run plan error", run, file=sys.stderr)
            return 1
        print("run stopReason:", run.get("result", {}).get("stopReason"))
        return 0
    finally:
        proc.terminate()
        proc.wait(timeout=5)


if __name__ == "__main__":
    raise SystemExit(main())
