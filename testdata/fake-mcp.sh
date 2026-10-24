#!/bin/sh
# 假 MCP server：证明隧道连通、且凭据由宿主注入。
# 从 stdin 读一行请求，返回一行 JSON。
read -r line
if [ -n "${GITHUB_TOKEN:-}" ]; then
  printf '{"jsonrpc":"2.0","id":1,"result":{"hasCredential":true,"request":%s}}\n' "\"$line\""
else
  printf '{"jsonrpc":"2.0","id":1,"result":{"hasCredential":false}}\n'
fi
