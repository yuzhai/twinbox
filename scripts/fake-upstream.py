#!/usr/bin/env python3
"""最小假上游：验证 broker 真的注入了凭据，且路径被正确重写。

用法: fake-upstream.py <port> <expected-key>
成功时返回 200 与 {"injected": true, "path": ...}
"""
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer


class H(BaseHTTPRequestHandler):
    expected = ""

    def do_POST(self):
        n = int(self.headers.get("content-length", "0"))
        self.rfile.read(n)
        key = self.headers.get("x-api-key", "")
        ok = key == self.expected and self.path == "/v1/messages"
        body = json.dumps({"injected": ok, "path": self.path}).encode()
        self.send_response(200 if ok else 403)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *a):
        pass


if __name__ == "__main__":
    H.expected = sys.argv[2]
    srv = HTTPServer(("127.0.0.1", int(sys.argv[1])), H)
    with open(sys.argv[3], "w") as f:
        f.write(str(srv.server_address[1]))
    srv.serve_forever()
