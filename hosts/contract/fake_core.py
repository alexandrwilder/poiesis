#!/usr/bin/env python3
"""The core's side of docs/HOST.md, played inside a host in place of the real core: meet,
show the picture with a look, record five seconds, stop, rest the picture. What the host
answered goes to a JSON report; check.py checks the report and the recording. Every host
runs this same file."""
import json
import os
import socket
import sys
import time

report_path, out_path = sys.argv[1], sys.argv[2]
report = {"levels": 0, "errors": []}
sock = socket.socket(socket.AF_UNIX)
pending = b""


def send(message):
    sock.sendall((json.dumps(message) + "\n").encode())


def receive_until(kind, seconds):
    """Read messages until one of this type arrives; count levels and keep errors on the way.
    Plain recv with a deadline: a file object over a socket breaks after its first timeout."""
    global pending
    end = time.time() + seconds
    while True:
        while b"\n" in pending:
            line, pending = pending.split(b"\n", 1)
            message = json.loads(line)
            if message.get("t") == "level":
                report["levels"] += 1
            if message.get("t") == "error":
                report["errors"].append(message)
            if kind and message.get("t") == kind:
                return message
        left = end - time.time()
        if left <= 0:
            return None
        sock.settimeout(left)
        try:
            chunk = sock.recv(65536)
        except socket.timeout:
            return None
        if not chunk:
            return None
        pending += chunk


def run():
    # text in the view, so a screenshot shows it over the picture
    print("\n  the contract test: text over the picture\n", flush=True)
    sock.connect(os.environ["POIESIS_HOST"])
    send({"t": "hello", "versions": [1], "app": "contract test"})
    report["hello"] = receive_until("hello", 5)
    warm = [1.04, 0, 0, 0.012, 0, 0.97, 0, 0, 0, 0, 0.86, 0]
    send({"t": "picture", "on": True, "look": "warm", "strength": 0.5, "matrix": warm, "fps": 15})
    receive_until(None, 3)
    send({"t": "record", "action": "start", "file": out_path})
    t0 = time.time()
    report["started"] = receive_until("recording", 15)
    report["started_after_s"] = round(time.time() - t0, 2)
    open(report_path + ".recording", "w").close()  # a host's test may look at the window now
    receive_until(None, 5)
    send({"t": "record", "action": "stop"})
    t1 = time.time()
    report["stopped"] = receive_until("recording", 20)
    report["stop_took_s"] = round(time.time() - t1, 2)
    send({"t": "picture", "on": False})
    time.sleep(0.5)


try:
    run()
except Exception:
    import traceback
    report["crash"] = traceback.format_exc()
with open(report_path, "w") as f:
    json.dump(report, f, indent=1)
