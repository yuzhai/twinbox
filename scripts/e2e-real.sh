#!/bin/sh
# 真实流程验收：真实 agent（opencode）+ 真实网关。
# 会消耗模型额度，默认不进 make verify，需显式执行 make e2e-real。
set -eu
cd "$(dirname "$0")/.." || exit 1

BIN="$PWD/bin/twinbox"
SHIM="$PWD/bin/twinbox-shim"
[ -x "$BIN" ] && [ -x "$SHIM" ] || { echo "先执行 make build" >&2; exit 1; }

MODEL="${TWINBOX_E2E_MODEL:-deepseek-v4-1-flash}"
WS="${TMPDIR:-/tmp}/twinbox-e2e/ws"
rm -rf "$WS" && mkdir -p "$WS"

export TWINBOX_SHIM="$SHIM"

echo "== 真实流程：opencode @ $MODEL =="
if ! out=$("$BIN" run opencode --derive-opencode --workspace "$WS" --exec \
  "cd /workspace/ws && opencode run 'Create a file named proof.txt whose exact content is PONG. Then reply with DONE.' -m twinbox/$MODEL --standalone --format json" 2>&1); then
  echo "$out" | tail -5 | sed 's/^/  /'
  echo "FAIL  e2e-real  agent 运行失败" >&2
  exit 1
fi

if echo "$out" | grep -q '"text":"DONE"'; then
  echo "PASS  e2e-real  agent 完成对话"
else
  echo "FAIL  e2e-real  agent 未返回 DONE" >&2
  exit 1
fi

if [ "$(cat "$WS/proof.txt" 2>/dev/null)" = "PONG" ]; then
  echo "PASS  e2e-real  工具调用写入了工作区文件"
else
  echo "FAIL  e2e-real  工作区文件缺失或内容不符" >&2
  exit 1
fi

echo "artifacts: $WS"
