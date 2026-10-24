#!/bin/sh
# 红队与端到端：全程离线，不接触真实上游。
set -u
cd "$(dirname "$0")/.." || exit 1

BIN="$PWD/bin/twinbox"
SHIM="$PWD/bin/twinbox-shim"
[ -x "$BIN" ] && [ -x "$SHIM" ] || { echo "先执行 make build" >&2; exit 1; }

TMP=$(mktemp -d)
export TWINBOX_SECRETS="$TMP/secrets.json"
export TWINBOX_AUDIT="$TMP/audit.jsonl"
export TWINBOX_SHIM="$SHIM"
fail=0

cleanup() { kill "$UP_PID" 2>/dev/null; wait "$UP_PID" 2>/dev/null; echo "workdir: $TMP"; }
trap cleanup EXIT

# 1) 启动假上游，并注册一个"真实"密钥（只在宿主侧）
python3 scripts/fake-upstream.py 0 test-key-123 "$TMP/port" &
UP_PID=$!
for _ in $(seq 1 50); do [ -s "$TMP/port" ] && break; sleep 0.1; done
UP_PORT=$(cat "$TMP/port" 2>/dev/null || echo 0)
"$BIN" secret set anthropic test-key-123 >/dev/null

echo "== L5 端到端：假 agent + 假上游 =="
export TWINBOX_UPSTREAM_ANTHROPIC=http://127.0.0.1:$UP_PORT
if out=$("$BIN" run fake 2>&1); then
  echo "$out" | sed 's/^/  /'
  if echo "$out" | grep -q '"injected": true'; then
    echo "PASS  e2e  凭据被注入，路径被重写"
  else
    echo "FAIL  e2e  上游未收到预期请求"; fail=1
  fi
else
  echo "$out" | sed 's/^/  /'
  echo "FAIL  e2e  agent 运行失败"; fail=1
fi

# 2) 审计对账
if grep -q '"decision":"allow"' "$TWINBOX_AUDIT" 2>/dev/null; then
  echo "PASS  audit  出站请求已记录"
else
  echo "FAIL  audit  缺少审计记录"; fail=1
fi

# 3) 撤销凭据后再跑一次，必须失败
"$BIN" secret rm anthropic >/dev/null
if "$BIN" run fake >/dev/null 2>&1; then
  echo "FAIL  revoke  删除凭据后仍然成功"; fail=1
else
  echo "PASS  revoke  删除凭据后调用失败"
fi

# 4) 沙箱行为与红队用例
echo
echo "== L3/L4 沙箱内断言 =="
if "$BIN" run fake --exec "sh scripts/sandbox-test.sh"; then
  echo "PASS  sandbox  全部断言通过"
else
  echo "FAIL  sandbox  存在未通过断言"; fail=1
fi

echo
[ "$fail" -eq 0 ] && echo "attack: 通过" || echo "attack: 失败"
exit "$fail"
