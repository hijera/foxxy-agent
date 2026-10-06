#!/usr/bin/env python3
"""HTTP e2e: the session mode and the YAML backend stay in sync with a client.

Prerequisites: ``foxxycode http`` is already listening (same host/port as BASE_URL).

The composer's Mode and Model are session state, not client state. This drives
the same routes the SPA drives and checks both directions:

1. ``PATCH /foxxycode/sessions/{id}`` with ``mode`` stores the profile without a
   turn, and ``GET /foxxycode/sessions/{id}/messages`` reports it back - which is
   what makes a reload come back in ``plan`` / ``ask`` instead of ``agent``.
2. An unknown mode is refused with ``400`` and leaves the stored profile alone.
3. ``PATCH`` with ``selectedModelId`` is echoed and reported back on the same
   transcript read.
4. A turn sent with a top-level profile writes that profile onto the session.
5. Running a saved plan (``metadata.runPlanSlug``) switches the session back to
   ``agent`` **and** announces it on the stream as ``event: mode`` with
   ``currentModeId: agent``. A client that misses that frame keeps posting
   ``plan``, and step 4 shows what that would do to the session.

Steps 4 and 5 need a working LLM (the configured ``agent.model``).

Environment:

- ``BASE_URL`` - base for OpenAI-compatible routes (default ``http://127.0.0.1:19876/v1``).
"""

from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request
import uuid
from typing import Any, Tuple

PLAN_SLUG = "mode-sync-e2e"
PLAN_BODY = "## Summary\n\nSay hello.\n\n## Steps\n\n1. Reply with exactly: HI.\n"


def http_json(
    method: str, url: str, body: dict[str, Any] | None, headers: dict[str, str]
) -> Tuple[int, dict[str, Any]]:
    data = None if body is None else json.dumps(body).encode("utf-8")
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Accept", "application/json")
    if data is not None:
        req.add_header("Content-Type", "application/json")
    for k, v in headers.items():
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=180) as resp:
            raw = resp.read().decode("utf-8", errors="replace")
            return resp.status, (json.loads(raw) if raw.strip() else {})
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", errors="replace")
        try:
            return e.code, (json.loads(raw) if raw.strip() else {})
        except json.JSONDecodeError:
            return e.code, {"_raw": raw}


def http_sse(url: str, body: dict[str, Any], headers: dict[str, str]) -> Tuple[int, str]:
    data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(url, data=data, method="POST")
    req.add_header("Content-Type", "application/json")
    req.add_header("Accept", "text/event-stream")
    for k, v in headers.items():
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=300) as resp:
            return resp.status, resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", errors="replace")


def mode_frames(sse: str) -> list[str]:
    """Profile ids announced by `event: mode` frames, in arrival order."""
    out: list[str] = []
    for block in sse.split("\n\n"):
        if "event: mode" not in block:
            continue
        for line in block.split("\n"):
            if not line.startswith("data: "):
                continue
            try:
                payload = json.loads(line[len("data: ") :])
            except json.JSONDecodeError:
                continue
            mode = payload.get("currentModeId")
            if isinstance(mode, str) and mode:
                out.append(mode)
    return out


def main() -> int:
    base = os.environ.get("BASE_URL", "http://127.0.0.1:19876/v1").rstrip("/")
    root = base[: -len("/v1")] if base.endswith("/v1") else base
    sid = "sess_" + uuid.uuid4().hex[:24]
    hdr = {"X-FoxxyCode-Session-ID": sid}
    session_url = f"{root}/foxxycode/sessions/{sid}"

    # A turn is the only way to create the session; it also proves the profile a
    # client posts lands on the session.
    code, _ = http_json(
        "POST",
        f"{base}/responses",
        {"model": "plan", "input": "Reply with exactly: HI.", "stream": False},
        hdr,
    )
    if code != 200:
        print("plan turn want 200, got", code, file=sys.stderr)
        return 1
    code, msgs = http_json("GET", f"{session_url}/messages", None, hdr)
    if code != 200 or msgs.get("mode") != "plan":
        print("a posted profile did not reach the session:", code, msgs.get("mode"), file=sys.stderr)
        return 1

    # A mode stored without a turn: this is the composer switching Mode.
    code, patched = http_json("PATCH", session_url, {"mode": "ask"}, hdr)
    if code != 200 or patched.get("mode") != "ask":
        print("patch mode want 200/ask, got", code, patched, file=sys.stderr)
        return 1
    code, msgs = http_json("GET", f"{session_url}/messages", None, hdr)
    if code != 200 or msgs.get("mode") != "ask":
        print("stored mode not reported back:", code, msgs.get("mode"), file=sys.stderr)
        return 1

    code, bad = http_json("PATCH", session_url, {"mode": "wizard"}, hdr)
    if code != 400:
        print("unknown mode want 400, got", code, bad, file=sys.stderr)
        return 1
    code, msgs = http_json("GET", f"{session_url}/messages", None, hdr)
    if msgs.get("mode") != "ask":
        print("a refused patch changed the mode:", msgs.get("mode"), file=sys.stderr)
        return 1

    # The YAML backend rides the same PATCH and the same transcript read.
    code, models = http_json("GET", f"{base}/models", None, {})
    if code != 200:
        print("models want 200, got", code, file=sys.stderr)
        return 1
    backends = [
        row.get("id")
        for row in (models.get("data") or [])
        if row.get("owned_by") != "foxxycode" and row.get("id")
    ]
    if not backends:
        print("no YAML backends configured", file=sys.stderr)
        return 1
    pick = backends[0]
    code, patched = http_json("PATCH", session_url, {"selectedModelId": pick}, hdr)
    if code != 200 or patched.get("selectedModelId") != pick:
        print("patch selectedModelId want 200/", pick, "got", code, patched, file=sys.stderr)
        return 1
    code, msgs = http_json("GET", f"{session_url}/messages", None, hdr)
    if msgs.get("selectedModelId") != pick or msgs.get("model") != pick:
        print("stored model not reported back:", msgs.get("selectedModelId"), msgs.get("model"), file=sys.stderr)
        return 1

    # Back to plan, write a runnable plan, and run it: the server switches the
    # session to agent and must say so on the stream.
    code, _ = http_json("PATCH", session_url, {"mode": "plan"}, hdr)
    if code != 200:
        print("restore plan mode want 200, got", code, file=sys.stderr)
        return 1
    code, _ = http_json(
        "POST", f"{session_url}/plans", {"slug": PLAN_SLUG, "content": PLAN_BODY}, hdr
    )
    if code not in (200, 201, 409):
        print("create plan want 200/201, got", code, file=sys.stderr)
        return 1

    status, sse = http_sse(
        f"{base}/responses",
        {
            "model": "plan",
            "input": "Implement the plan.",
            "stream": True,
            "metadata": {"runPlanSlug": PLAN_SLUG},
        },
        hdr,
    )
    if status != 200:
        print("run plan want 200, got", status, sse[:400], file=sys.stderr)
        return 1
    frames = mode_frames(sse)
    if "agent" not in frames:
        print("the plan run never announced the switch to agent; frames:", frames, file=sys.stderr)
        return 1
    code, msgs = http_json("GET", f"{session_url}/messages", None, hdr)
    if msgs.get("mode") != "agent":
        print("session did not end in agent mode:", msgs.get("mode"), file=sys.stderr)
        return 1

    print("ok http mode/model sync e2e")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
