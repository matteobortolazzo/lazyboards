#!/usr/bin/env python3
"""Fake cenci broadcast socket for the README GIF recording.

Serves scripted StateSnapshot NDJSON lines (the daemon's public wire format,
see internal/cenciwatch/snapshot.go) on
$XDG_RUNTIME_DIR/cenci/cenci.sock, so the demo board shows live
agent badges, status-bar counts, and the dispatch segment without running
real agents. Started by demo.tape's hidden setup inside a throwaway
XDG_RUNTIME_DIR; it dies with the recording shell.

Window names join demo-repo cards by ticket-number prefix: keep them in sync
with the issues seeded by demo-repo-seed.sh (#7/#8 Implementing, #4 Refined).
"""
import json
import os
import socket
import threading
import time

sock_dir = os.path.join(os.environ["XDG_RUNTIME_DIR"], "cenci")
os.makedirs(sock_dir, mode=0o700, exist_ok=True)
sock_path = os.path.join(sock_dir, "cenci.sock")
if os.path.exists(sock_path):
    os.unlink(sock_path)


def window(name, status, task):
    return {
        "session": "demo",
        "window_index": "1",
        "window_name": name,
        "task_name": task,
        "status": status,
        "agent": "claude",
        "manually_named": True,
    }


snapshot = {
    "timestamp": "2026-01-01T00:00:00Z",
    "windows": [
        window("7-implement", "running", "Implement conflict resolution UI"),
        window("8-implement", "need-input", "Fix flaky auth middleware test"),
        window("4-design", "done", "Design nested tags"),
    ],
    "summary": {
        "total": 3,
        "idle": 0,
        "running": 1,
        "done": 1,
        "stopped": 0,
        "need_input": 1,
        "failed": 0,
    },
    "dispatch": {
        "enabled": True,
        "daemon_running": True,
        "interval": "5m",
        "pass_running": False,
        "last_run_at": "2026-01-01T00:00:00Z",
        "last_dispatched": 2,
        "last_skipped": 1,
    },
}
line = (json.dumps(snapshot) + "\n").encode()


def serve(conn):
    try:
        while True:
            conn.sendall(line)
            time.sleep(1)
    except OSError:
        pass
    finally:
        conn.close()


srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
srv.bind(sock_path)
os.chmod(sock_path, 0o600)
srv.listen(4)
while True:
    conn, _ = srv.accept()
    threading.Thread(target=serve, args=(conn,), daemon=True).start()
