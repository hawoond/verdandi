#!/usr/bin/env python3
import json
import sys
import time
import urllib.error
import urllib.request


def post_json(url, payload, *, accept="application/json, text/event-stream", origin=None, token=None, session_id=None):
    body = json.dumps(payload).encode("utf-8")
    request = urllib.request.Request(url, data=body, method="POST")
    request.add_header("Content-Type", "application/json")
    if accept is not None:
        request.add_header("Accept", accept)
    if origin is not None:
        request.add_header("Origin", origin)
    if token is not None:
        request.add_header("Authorization", f"Bearer {token}")
    if session_id is not None:
        request.add_header("MCP-Session-Id", session_id)
    return urllib.request.urlopen(request, timeout=10)


def wait_for_server(url, token):
    deadline = time.time() + 10
    while time.time() < deadline:
        try:
            response = post_json(url, {"jsonrpc": "2.0", "id": "init", "method": "initialize", "params": {"protocolVersion": "2025-11-25"}}, token=token)
            body = response.read()
            session_id = response.headers.get("MCP-Session-Id")
            if response.status == 200 and session_id:
                payload = json.loads(body)
                if payload.get("id") != "init" or "error" in payload:
                    raise SystemExit(f"unexpected initialize response: {payload!r}")
                return session_id
        except Exception:
            time.sleep(0.1)
    raise SystemExit(f"server did not become ready at {url}")


def decode_sse(body):
    messages = []
    for block in body.strip().split("\n\n"):
        data = []
        for line in block.splitlines():
            if line.startswith("data:"):
                data.append(line.removeprefix("data:").strip())
        if data:
            messages.append(json.loads("".join(data)))
    return messages


def expect_http_error(fn, status):
    try:
        fn()
    except urllib.error.HTTPError as exc:
        if exc.code != status:
            raise SystemExit(f"expected HTTP {status}, got {exc.code}: {exc.read().decode('utf-8', 'replace')}")
        return
    raise SystemExit(f"expected HTTP {status}, got success")


def delete_session(url, token, session_id):
    request = urllib.request.Request(url, method="DELETE")
    request.add_header("Authorization", f"Bearer {token}")
    request.add_header("MCP-Session-Id", session_id)
    return urllib.request.urlopen(request, timeout=10)


def main():
    if len(sys.argv) != 3:
        raise SystemExit("usage: validate_mcp_http_smoke.py URL TOKEN")
    url = sys.argv[1]
    token = sys.argv[2]
    session_id = wait_for_server(url, token)

    expect_http_error(
        lambda: post_json(url, {"jsonrpc": "2.0", "id": "missing-session", "method": "ping"}, token=token),
        400,
    )

    response = post_json(url, {"jsonrpc": "2.0", "method": "notifications/initialized"}, token=token, session_id=session_id)
    if response.status != 202:
        raise SystemExit(f"notification returned HTTP {response.status}")
    if response.read().strip():
        raise SystemExit("notification returned a response body")

    response = post_json(url, {
        "jsonrpc": "2.0",
        "id": "progress",
        "method": "tools/call",
        "params": {
            "_meta": {"progressToken": "http-smoke-progress"},
            "name": "run_plan",
            "arguments": {
                "request": "Build a calculator app and verify it.",
                "stages": [{"stage": "code-writer", "keyword": "fixture"}],
            },
        },
    }, token=token, session_id=session_id)
    if "text/event-stream" not in response.headers.get("Content-Type", ""):
        raise SystemExit(f"progress request returned unexpected content type {response.headers.get('Content-Type')}")
    messages = decode_sse(response.read().decode("utf-8"))
    if not any(message.get("method") == "notifications/progress" for message in messages):
        raise SystemExit(f"progress request did not return progress notification: {messages!r}")
    if not any(message.get("id") == "progress" for message in messages):
        raise SystemExit(f"progress request did not return final response: {messages!r}")

    expect_http_error(
        lambda: post_json(url, {"jsonrpc": "2.0", "id": "bad-origin", "method": "ping"}, origin="https://attacker.example", token=token, session_id=session_id),
        403,
    )
    response = post_json(url, {"jsonrpc": "2.0", "id": "trusted-origin", "method": "ping"}, origin="https://trusted.example", token=token, session_id=session_id)
    response.read()
    expect_http_error(
        lambda: post_json(url, {"jsonrpc": "2.0", "id": "bad-accept", "method": "ping"}, accept="application/json", token=token, session_id=session_id),
        406,
    )
    expect_http_error(
        lambda: post_json(url, {"jsonrpc": "2.0", "id": "missing-token", "method": "ping"}),
        401,
    )
    expect_http_error(
        lambda: post_json(url, {"jsonrpc": "2.0", "id": "wrong-token", "method": "ping"}, token="wrong", session_id=session_id),
        401,
    )
    response = delete_session(url, token, session_id)
    if response.status != 204:
        raise SystemExit(f"session delete returned HTTP {response.status}")
    expect_http_error(
        lambda: post_json(url, {"jsonrpc": "2.0", "id": "deleted-session", "method": "ping"}, token=token, session_id=session_id),
        404,
    )

    print(f"mcp http smoke ok: {url}")


if __name__ == "__main__":
    main()
