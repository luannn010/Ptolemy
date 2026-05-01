#!/usr/bin/env python3
"""STDIO MCP bridge for a remote or local Ptolemy worker."""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Any


PROTOCOL_VERSION = "2025-06-18"
SERVER_NAME = "ptolemy-stdio-wrapper"
SERVER_VERSION = "0.1.0"
DEFAULT_BASE_URL = "http://127.0.0.1:8080"
DEFAULT_TIMEOUT_SECONDS = 30
DEFAULT_HEALTH_TIMEOUT_SECONDS = 10


def env_int(name: str, default: int) -> int:
    raw = os.getenv(name, "").strip()
    if raw == "":
        return default
    try:
        return int(raw)
    except ValueError:
        return default


def make_error(code: int, message: str, request_id: Any = None) -> dict[str, Any]:
    return {
        "jsonrpc": "2.0",
        "id": request_id,
        "error": {"code": code, "message": message},
    }


def make_result(result: dict[str, Any], request_id: Any = None) -> dict[str, Any]:
    return {"jsonrpc": "2.0", "id": request_id, "result": result}


def make_text_result(text: str, data: dict[str, Any] | None = None, is_error: bool = False) -> dict[str, Any]:
    result: dict[str, Any] = {
        "content": [{"type": "text", "text": text}],
        "isError": is_error,
    }
    if data is not None:
        result["structuredContent"] = data
    return result


class WorkerHTTPError(RuntimeError):
    pass


class WorkerClient:
    def __init__(self) -> None:
        base_url = os.getenv("PTOLEMY_BASE_URL", DEFAULT_BASE_URL).strip() or DEFAULT_BASE_URL
        self.base_url = base_url.rstrip("/")
        self.auth_token = os.getenv("PTOLEMY_AUTH_TOKEN", "").strip()
        self.default_session_id = os.getenv("PTOLEMY_DEFAULT_SESSION_ID", "").strip()
        self.default_timeout = env_int("PTOLEMY_HTTP_TIMEOUT_SECONDS", DEFAULT_TIMEOUT_SECONDS)
        self.health_timeout = env_int("PTOLEMY_HEALTH_TIMEOUT_SECONDS", DEFAULT_HEALTH_TIMEOUT_SECONDS)

    def _headers(self) -> dict[str, str]:
        headers = {"Accept": "application/json"}
        if self.auth_token:
            headers["Authorization"] = f"Bearer {self.auth_token}"
        return headers

    def _request(self, method: str, path: str, payload: dict[str, Any] | None, timeout: int) -> dict[str, Any]:
        url = f"{self.base_url}{path}"
        data = None
        headers = self._headers()
        if payload is not None:
            headers["Content-Type"] = "application/json"
            data = json.dumps(payload).encode("utf-8")

        request = urllib.request.Request(url=url, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(request, timeout=timeout) as response:
                body = response.read().decode("utf-8")
                parsed = json.loads(body) if body else {}
                return {
                    "status_code": response.status,
                    "url": url,
                    "body": parsed,
                }
        except urllib.error.HTTPError as exc:
            body_text = exc.read().decode("utf-8", errors="replace")
            try:
                parsed_body = json.loads(body_text) if body_text else {"error": exc.reason}
            except json.JSONDecodeError:
                parsed_body = {"error": body_text or exc.reason}
            raise WorkerHTTPError(f"worker error {exc.code}: {parsed_body}") from exc
        except urllib.error.URLError as exc:
            raise WorkerHTTPError(f"worker request failed: {exc.reason}") from exc

    def get(self, path: str, timeout: int | None = None) -> dict[str, Any]:
        return self._request("GET", path, None, timeout or self.default_timeout)

    def post(self, path: str, payload: dict[str, Any], timeout: int | None = None) -> dict[str, Any]:
        return self._request("POST", path, payload, timeout or self.default_timeout)


TOOLS: list[dict[str, Any]] = [
    {
        "name": "ptolemy_health",
        "description": "Check the configured Ptolemy worker health endpoint.",
        "inputSchema": {
            "type": "object",
            "properties": {},
        },
    },
    {
        "name": "ptolemy_create_session",
        "description": "Create a Ptolemy worker session over HTTP.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "name": {"type": "string"},
                "workspace": {"type": "string"},
                "description": {"type": "string"},
            },
            "required": ["name"],
        },
    },
    {
        "name": "ptolemy_execute",
        "description": "Execute a command through the Ptolemy worker /execute endpoint.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "session_id": {"type": "string"},
                "command": {"type": "string"},
                "cwd": {"type": "string"},
                "reason": {"type": "string"},
                "timeout": {"type": "integer"},
            },
            "required": ["command"],
        },
    },
    {
        "name": "ptolemy_run_task_file",
        "description": "Best-effort task execution helper for a remote Ptolemy worker. Uses /agent/run if present, otherwise reports the available fallback.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "task_file": {"type": "string"},
                "max_steps": {"type": "integer"},
                "allow_scripts": {"type": "boolean"},
                "session_id": {"type": "string"},
            },
            "required": ["task_file"],
        },
    },
]


def handle_health(client: WorkerClient, _: dict[str, Any]) -> dict[str, Any]:
    response = client.get("/health", timeout=client.health_timeout)
    text = json.dumps(response["body"], indent=2, sort_keys=True)
    data = {
        "base_url": client.base_url,
        "status_code": response["status_code"],
        "health": response["body"],
    }
    return make_text_result(text, data=data)


def handle_create_session(client: WorkerClient, args: dict[str, Any]) -> dict[str, Any]:
    name = str(args.get("name", "")).strip()
    if not name:
        raise WorkerHTTPError("name is required")

    payload: dict[str, Any] = {"name": name}
    workspace = str(args.get("workspace", "")).strip()
    description = str(args.get("description", "")).strip()
    if workspace:
        payload["workspace"] = workspace
    if description:
        payload["description"] = description

    response = client.post("/sessions", payload)
    body = response["body"]
    text = json.dumps(body, indent=2, sort_keys=True)
    return make_text_result(text, data=body)


def handle_execute(client: WorkerClient, args: dict[str, Any]) -> dict[str, Any]:
    command = str(args.get("command", "")).strip()
    if not command:
        raise WorkerHTTPError("command is required")

    session_id = str(args.get("session_id", "")).strip() or client.default_session_id
    if not session_id:
        raise WorkerHTTPError("session_id is required or set PTOLEMY_DEFAULT_SESSION_ID")

    payload: dict[str, Any] = {
        "session_id": session_id,
        "command": command,
    }
    cwd = str(args.get("cwd", "")).strip()
    reason = str(args.get("reason", "")).strip()
    timeout = args.get("timeout")

    if cwd:
        payload["cwd"] = cwd
    if reason:
        payload["reason"] = reason
    if isinstance(timeout, int):
        payload["timeout"] = timeout

    response = client.post("/execute", payload)
    body = response["body"]
    text = json.dumps(body, indent=2, sort_keys=True)
    return make_text_result(text, data=body)


def handle_run_task_file(client: WorkerClient, args: dict[str, Any]) -> dict[str, Any]:
    task_file = str(args.get("task_file", "")).strip()
    if not task_file:
        raise WorkerHTTPError("task_file is required")

    payload: dict[str, Any] = {"task_file": task_file}
    session_id = str(args.get("session_id", "")).strip() or client.default_session_id
    if session_id:
        payload["session_id"] = session_id
    if isinstance(args.get("max_steps"), int):
        payload["max_steps"] = args["max_steps"]
    if isinstance(args.get("allow_scripts"), bool):
        payload["allow_scripts"] = args["allow_scripts"]

    try:
        response = client.post("/agent/run", payload)
        body = response["body"]
        text = json.dumps(body, indent=2, sort_keys=True)
        return make_text_result(text, data=body)
    except WorkerHTTPError as exc:
        message = (
            "The configured worker does not appear to expose POST /agent/run. "
            "Current repo docs show /tasks/run-inbox as the available task endpoint, "
            "so run the task file through your existing local agent/task-runner workflow instead."
        )
        data = {
            "requested_task_file": task_file,
            "base_url": client.base_url,
            "error": str(exc),
            "fallback_endpoint": "/tasks/run-inbox",
        }
        return make_text_result(message, data=data, is_error=True)


HANDLERS = {
    "ptolemy_health": handle_health,
    "ptolemy_create_session": handle_create_session,
    "ptolemy_execute": handle_execute,
    "ptolemy_run_task_file": handle_run_task_file,
}


def handle_request(client: WorkerClient, request: dict[str, Any]) -> dict[str, Any] | None:
    request_id = request.get("id")
    method = request.get("method")

    if method == "initialize":
        return make_result(
            {
                "protocolVersion": PROTOCOL_VERSION,
                "serverInfo": {"name": SERVER_NAME, "version": SERVER_VERSION},
                "capabilities": {"tools": {}},
            },
            request_id=request_id,
        )

    if method == "notifications/initialized":
        return None

    if method == "tools/list":
        return make_result({"tools": TOOLS}, request_id=request_id)

    if method == "tools/call":
        params = request.get("params", {})
        name = params.get("name")
        arguments = params.get("arguments", {})

        if not isinstance(arguments, dict):
            return make_error(-32602, "tool arguments must be an object", request_id=request_id)

        handler = HANDLERS.get(name)
        if handler is None:
            return make_error(-32601, f"unknown tool: {name}", request_id=request_id)

        try:
            result = handler(client, arguments)
            return make_result(result, request_id=request_id)
        except WorkerHTTPError as exc:
            return make_error(-32000, str(exc), request_id=request_id)
        except Exception as exc:  # pragma: no cover
            return make_error(-32001, f"internal error: {exc}", request_id=request_id)

    return make_error(-32601, "method not found", request_id=request_id)


def run_stdio_server() -> int:
    client = WorkerClient()
    for raw_line in sys.stdin:
        line = raw_line.strip()
        if not line:
            continue
        try:
            request = json.loads(line)
        except json.JSONDecodeError:
            response = make_error(-32700, "parse error")
        else:
            response = handle_request(client, request)
        if response is None:
            continue
        sys.stdout.write(json.dumps(response) + "\n")
        sys.stdout.flush()
    return 0


def run_self_test() -> int:
    client = WorkerClient()
    summary: dict[str, Any] = {
        "server": SERVER_NAME,
        "base_url": client.base_url,
        "default_session_id": bool(client.default_session_id),
        "auth_token_configured": bool(client.auth_token),
    }
    try:
        health = client.get("/health", timeout=client.health_timeout)
        summary["ok"] = True
        summary["health"] = health["body"]
        summary["status_code"] = health["status_code"]
    except WorkerHTTPError as exc:
        summary["ok"] = False
        summary["error"] = str(exc)
        print(json.dumps(summary, indent=2, sort_keys=True))
        return 1

    print(json.dumps(summary, indent=2, sort_keys=True))
    return 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Ptolemy STDIO MCP wrapper")
    parser.add_argument("--self-test", action="store_true", help="Run a health check against the configured worker and exit.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if args.self_test:
        return run_self_test()
    return run_stdio_server()


if __name__ == "__main__":
    raise SystemExit(main())
