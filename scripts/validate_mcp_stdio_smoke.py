#!/usr/bin/env python3
import json
import sys
from pathlib import Path


def id_key(value):
    if value is None:
        return None
    if isinstance(value, str):
        return "s:" + value
    if isinstance(value, (int, float)):
        return "n:" + ("%g" % value)
    return type(value).__name__ + ":" + str(value)


def fixture_expected_ids(path):
    expected = set()
    for line_number, raw in enumerate(Path(path).read_text(encoding="utf-8").splitlines(), start=1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        try:
            payload = json.loads(line)
        except json.JSONDecodeError as exc:
            raise SystemExit(f"fixture line {line_number} is not JSON: {exc}") from exc
        requests = payload if isinstance(payload, list) else [payload]
        for request in requests:
            key = id_key(request.get("id"))
            if key is not None:
                if key in expected:
                    raise SystemExit(f"duplicate fixture response id {key} on line {line_number}")
                expected.add(key)
    return expected


def decode_output(path):
    messages = []
    for line_number, raw in enumerate(Path(path).read_text(encoding="utf-8").splitlines(), start=1):
        line = raw.strip()
        if not line:
            continue
        try:
            payload = json.loads(line)
        except json.JSONDecodeError as exc:
            raise SystemExit(f"stdout line {line_number} is not JSON: {exc}\n{line}") from exc
        if isinstance(payload, list):
            messages.extend(payload)
        else:
            messages.append(payload)
    return messages


def main():
    if len(sys.argv) != 4:
        raise SystemExit("usage: validate_mcp_stdio_smoke.py FIXTURE STDOUT STDERR")

    fixture_path, stdout_path, stderr_path = sys.argv[1:]
    stderr = Path(stderr_path).read_text(encoding="utf-8")
    if stderr.strip():
        raise SystemExit(f"verdandi-mcp wrote to stderr:\n{stderr}")

    expected_ids = fixture_expected_ids(fixture_path)
    messages = decode_output(stdout_path)
    if not messages:
        raise SystemExit("verdandi-mcp produced no stdout messages")

    seen_ids = set()
    saw_progress = False
    for index, message in enumerate(messages, start=1):
        if message.get("jsonrpc") != "2.0":
            raise SystemExit(f"message {index} has unexpected jsonrpc version: {message!r}")
        if "method" in message:
            if message.get("method") != "notifications/progress":
                raise SystemExit(f"unexpected server notification in message {index}: {message!r}")
            params = message.get("params") or {}
            if params.get("progressToken") == "fixture-progress":
                saw_progress = True
            continue
        if "error" in message:
            raise SystemExit(f"message {index} returned JSON-RPC error: {message['error']!r}")
        key = id_key(message.get("id"))
        if key is None:
            raise SystemExit(f"response message {index} is missing id: {message!r}")
        seen_ids.add(key)

    missing = sorted(expected_ids - seen_ids)
    if missing:
        raise SystemExit(f"missing response ids: {missing}; saw {sorted(seen_ids)}")
    if not saw_progress:
        raise SystemExit("did not observe notifications/progress for fixture-progress")

    print(f"mcp stdio smoke ok: {len(seen_ids)} responses, {len(messages)} messages")


if __name__ == "__main__":
    main()
