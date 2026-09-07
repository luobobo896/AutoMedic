#!/usr/bin/env python3
"""AutoMedic ingest sidecar：把 Sentry / Grafana Loki webhook 映射为平台事件。

环境变量：
  AUTOMEDIC_INGEST_URL    默认 http://127.0.0.1:8080/api/v1/ingest/events
  AUTOMEDIC_INGEST_TOKEN  投递令牌（X-AM-Token）
  SIDECAR_LISTEN          默认 :8091

不转发密钥到日志。不是平台内置插件，可不部署。
"""
from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

INGEST_URL = os.environ.get("AUTOMEDIC_INGEST_URL", "http://127.0.0.1:8080/api/v1/ingest/events")
INGEST_TOKEN = os.environ.get("AUTOMEDIC_INGEST_TOKEN", "")
LISTEN = os.environ.get("SIDECAR_LISTEN", ":8091")


def _s(v: Any) -> str:
    if v is None:
        return ""
    if isinstance(v, str):
        return v
    return json.dumps(v, ensure_ascii=False)


def _omit_empty(event: dict, *keys: str) -> dict:
    for k in keys:
        if not _s(event.get(k)).strip():
            event.pop(k, None)
    return event


def _level(raw: str) -> str:
    v = (raw or "error").lower()
    if v in ("fatal", "error", "warn", "info"):
        return v
    if v in ("warning", "warn"):
        return "warn"
    if v in ("critical", "panic", "emerg"):
        return "fatal"
    return "error"


def map_sentry(body: dict) -> dict:
    data = body.get("data") if isinstance(body.get("data"), dict) else body
    event = data.get("event") if isinstance(data.get("event"), dict) else data
    issue = data.get("issue") if isinstance(data.get("issue"), dict) else {}
    title = _s(event.get("title") or issue.get("title") or body.get("title") or body.get("message"))
    message = _s(event.get("message") or event.get("culprit") or issue.get("culprit") or "")
    stack = _s(event.get("stacktrace") or "")
    if not stack:
        exc = event.get("exception") if isinstance(event.get("exception"), dict) else {}
        values = exc.get("values") if isinstance(exc.get("values"), list) else []
        parts = []
        for item in values:
            if not isinstance(item, dict):
                continue
            parts.append(_s(item.get("type")) + ": " + _s(item.get("value")))
            frames = (((item.get("stacktrace") or {}).get("frames")) or [])
            for fr in frames[-12:]:
                if isinstance(fr, dict):
                    parts.append("  at %s (%s:%s)" % (fr.get("function"), fr.get("filename"), fr.get("lineno")))
        stack = "\n".join(parts)
    fp = _s(event.get("event_id") or issue.get("id") or body.get("id"))
    repo = _s(event.get("transaction") or issue.get("project") or body.get("project") or event.get("project"))
    occurred = _s(event.get("timestamp") or issue.get("lastSeen") or "")
    return _omit_empty({
        "source": "sentry",
        "level": _level(_s(event.get("level") or issue.get("level"))),
        "title": title,
        "message": message,
        "stack": stack,
        "fingerprint": fp,
        "repo_hint": repo,
        "occurred_at": occurred,
        "payload": body,
    }, "occurred_at", "fingerprint", "repo_hint", "stack", "message")


def map_loki(body: dict) -> list[dict]:
    alerts = body.get("alerts")
    if not isinstance(alerts, list) or not alerts:
        labels = body.get("commonLabels") if isinstance(body.get("commonLabels"), dict) else body.get("labels") or {}
        ann = body.get("commonAnnotations") if isinstance(body.get("commonAnnotations"), dict) else body.get("annotations") or {}
        alerts = [{"labels": labels, "annotations": ann, "fingerprint": body.get("groupKey") or body.get("fingerprint")}]
    out = []
    for a in alerts:
        if not isinstance(a, dict):
            continue
        labels = a.get("labels") if isinstance(a.get("labels"), dict) else {}
        ann = a.get("annotations") if isinstance(a.get("annotations"), dict) else {}
        title = _s(labels.get("alertname") or ann.get("summary") or body.get("title") or "loki alert")
        message = _s(ann.get("summary") or ann.get("message") or "")
        stack = _s(ann.get("description") or ann.get("stack") or "")
        out.append(_omit_empty({
            "source": "loki",
            "level": _level(_s(labels.get("severity") or labels.get("level"))),
            "title": title,
            "message": message,
            "stack": stack,
            "fingerprint": _s(a.get("fingerprint") or body.get("groupKey") or title),
            "repo_hint": _s(labels.get("service") or labels.get("repo") or labels.get("job")),
            "occurred_at": _s(a.get("startsAt") or ""),
            "payload": a,
        }, "occurred_at", "fingerprint", "repo_hint", "stack", "message"))
    return out


def forward(event: dict) -> tuple[int, str]:
    if not INGEST_TOKEN:
        return 500, "SIDECAR missing AUTOMEDIC_INGEST_TOKEN"
    data = json.dumps(event, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(
        INGEST_URL,
        data=data,
        method="POST",
        headers={
            "Content-Type": "application/json",
            "X-AM-Token": INGEST_TOKEN,
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.status, resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace")
    except Exception as e:  # noqa: BLE001 — sidecar 边界，把失败回给采集器
        return 502, json.dumps({"code": 502, "message": str(e)})


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt: str, *args: Any) -> None:
        sys.stderr.write("[sidecar] " + (fmt % args) + "\n")

    def _read_json(self) -> dict:
        n = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(n) if n else b"{}"
        if not raw:
            return {}
        return json.loads(raw.decode("utf-8"))

    def _write(self, code: int, body: str) -> None:
        b = body.encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)

    def do_GET(self) -> None:  # noqa: N802
        if self.path in ("/healthz", "/"):
            self._write(200, json.dumps({"status": "ok", "ingest": INGEST_URL}))
            return
        self._write(404, '{"message":"not found"}')

    def do_POST(self) -> None:  # noqa: N802
        path = self.path.split("?", 1)[0]
        try:
            body = self._read_json()
        except json.JSONDecodeError as e:
            self._write(400, json.dumps({"message": "invalid json: %s" % e}))
            return
        events: list[dict]
        if path in ("/sentry", "/sentry/"):
            events = [map_sentry(body)]
        elif path in ("/loki", "/grafana", "/loki/"):
            events = map_loki(body)
        elif path in ("/", "/raw", "/ingest"):
            events = [body]
        else:
            self._write(404, '{"message":"use POST /sentry, /loki or /raw"}')
            return
        last_code, last_body = 200, ""
        results = []
        for ev in events:
            if not _s(ev.get("title")) and not _s(ev.get("message")):
                results.append({"action": "error", "reason": "title and message empty"})
                last_code = 400
                continue
            code, resp = forward(ev)
            last_code, last_body = code, resp
            results.append({"status": code, "body": resp})
        if len(results) == 1 and last_body:
            self._write(last_code, last_body)
            return
        self._write(last_code if last_code >= 400 else 200, json.dumps({"results": results}, ensure_ascii=False))


def main() -> None:
    host, _, port = LISTEN.rpartition(":")
    if not host:
        host = "0.0.0.0"
    httpd = ThreadingHTTPServer((host, int(port)), Handler)
    sys.stderr.write("[sidecar] listen %s%s -> %s\n" % (host, ":" + port, INGEST_URL))
    httpd.serve_forever()


if __name__ == "__main__":
    main()
