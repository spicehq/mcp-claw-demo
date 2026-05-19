"""Stub OpenAI-compatible /v1/chat/completions server.

Stands in for Acme's "private" LLM endpoint in the local demo stack. It accepts
any chat request and always returns the same canned response, so end-to-end
flows through `model: chat-private` work without needing real inference.

Endpoints:
  GET  /healthz                  → 200 "ok"
  GET  /v1/models                → list one model so list_models hints succeed
  POST /v1/chat/completions      → fixed completion (handles `stream: true` too)

The response shape matches the OpenAI Chat Completions API closely enough for
Spice's async-openai client to parse it without complaint.
"""
from __future__ import annotations

import json
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

PORT = 8081
CANNED_REPLY = (
    "[stub] Acme private LLM here. This response is a fixed string emitted by "
    "the local demo stub — no real inference happened. Wire `chat-private` to "
    "your actual private endpoint to see real responses."
)


def _completion_payload(model: str) -> dict[str, Any]:
    return {
        "id": f"chatcmpl-{uuid.uuid4().hex[:24]}",
        "object": "chat.completion",
        "created": int(time.time()),
        "model": model,
        "choices": [
            {
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": CANNED_REPLY,
                    "tool_calls": None,
                    "refusal": None,
                },
                "finish_reason": "stop",
                "logprobs": None,
            }
        ],
        "usage": {
            "prompt_tokens": 42,
            "completion_tokens": 24,
            "total_tokens": 66,
        },
        "system_fingerprint": "stub",
    }


def _stream_chunks(model: str):
    """Yield SSE-formatted chunks for `stream: true` requests."""
    chunk_id = f"chatcmpl-{uuid.uuid4().hex[:24]}"
    created = int(time.time())

    def frame(delta: dict[str, Any], finish: str | None) -> bytes:
        payload = {
            "id": chunk_id,
            "object": "chat.completion.chunk",
            "created": created,
            "model": model,
            "choices": [{"index": 0, "delta": delta, "finish_reason": finish}],
        }
        return f"data: {json.dumps(payload)}\n\n".encode()

    yield frame({"role": "assistant"}, None)
    yield frame({"content": CANNED_REPLY}, None)
    yield frame({}, "stop")
    yield b"data: [DONE]\n\n"


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt: str, *args: Any) -> None:
        print(f"[stub-llm] {self.address_string()} - {fmt % args}", flush=True)

    def _json(self, status: int, body: dict[str, Any]) -> None:
        encoded = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def do_GET(self) -> None:  # noqa: N802
        if self.path == "/healthz":
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.end_headers()
            self.wfile.write(b"ok")
            return
        if self.path == "/v1/models":
            self._json(
                200,
                {
                    "object": "list",
                    "data": [
                        {
                            "id": "acme-llama-3.1-70b-instruct",
                            "object": "model",
                            "created": 0,
                            "owned_by": "acme",
                        }
                    ],
                },
            )
            return
        self._json(404, {"error": {"message": "not found"}})

    def do_POST(self) -> None:  # noqa: N802
        if self.path != "/v1/chat/completions":
            self._json(404, {"error": {"message": "not found"}})
            return

        length = int(self.headers.get("Content-Length") or "0")
        raw = self.rfile.read(length) if length else b"{}"
        try:
            req = json.loads(raw or b"{}")
        except json.JSONDecodeError:
            self._json(400, {"error": {"message": "invalid json"}})
            return

        model = req.get("model") or "acme-llama-3.1-70b-instruct"
        stream = bool(req.get("stream"))

        if not stream:
            self._json(200, _completion_payload(model))
            return

        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "keep-alive")
        self.end_headers()
        for chunk in _stream_chunks(model):
            self.wfile.write(chunk)
            self.wfile.flush()


def main() -> None:
    server = ThreadingHTTPServer(("0.0.0.0", PORT), Handler)
    print(f"[stub-llm] listening on :{PORT}", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        server.shutdown()


if __name__ == "__main__":
    main()
