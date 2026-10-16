"""Minimal HTTP health endpoint for the AI worker.

The worker is a queue consumer, not a web service, but a tiny health listener
makes the process observable from outside (port 6161 per the deployment map):
liveness for the supervisor, plus the last pipeline outcome.
"""
from __future__ import annotations

import json
import logging
import threading
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

log = logging.getLogger(__name__)

_state = {
    "status": "starting",
    "jobs_processed": 0,
    "jobs_failed": 0,
    "last_result": None,
    "started_at": datetime.now(timezone.utc).isoformat(),
}


def record_success() -> None:
    _state["jobs_processed"] = int(_state["jobs_processed"]) + 1
    _state["status"] = "running"
    _state["last_result"] = datetime.now(timezone.utc).isoformat()


def record_failure() -> None:
    _state["jobs_failed"] = int(_state["jobs_failed"]) + 1
    _state["status"] = "running"
    _state["last_result"] = datetime.now(timezone.utc).isoformat()


def set_running() -> None:
    _state["status"] = "running"


class _HealthHandler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:  # noqa: N802 (http.server API)
        if self.path not in ("/health", "/"):
            self.send_response(404)
            self.end_headers()
            return
        body = json.dumps(_state).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args) -> None:  # silence per-request noise
        return


def start_health_server(port: int) -> ThreadingHTTPServer:
    server = ThreadingHTTPServer(("0.0.0.0", port), _HealthHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True, name="health-server")
    thread.start()
    log.info("Health server listening on 0.0.0.0:%d", port)
    return server
