#!/bin/sh
# 在沙箱内验证 MCP 隧道：stdio -> shim -> 宿主网关 -> 假 MCP server。
set -eu
[ -n "${TWINBOX_MCP_SOCK:-}" ] || { echo "TWINBOX_MCP_SOCK 未设置" >&2; exit 2; }
printf '{"method":"initialize"}\n' | /run/twinbox/bin/twinbox-shim mcp echo

# 生成的 agent 配置里不应出现真实凭据
if grep -rq "ghp_real" "$HOME" 2>/dev/null; then
  echo "配置中出现了真实凭据" >&2
  exit 3
fi
