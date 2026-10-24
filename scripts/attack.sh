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

# 4) MCP 隧道：沙箱内只有 shim，凭据由宿主网关注入
echo
echo "== MCP 隧道 =="
"$BIN" secret set github ghp_real_test_value >/dev/null
cat > "$TMP/mcp.json" <<EOF
{"servers":{"echo":{"command":["/bin/sh","$PWD/testdata/fake-mcp.sh"],"env":{"GITHUB_TOKEN":"@github"}}}}
EOF
export TWINBOX_MCP_CONFIG="$TMP/mcp.json"
if out=$("$BIN" run fake --exec "sh scripts/mcp-check.sh" 2>&1); then
  echo "$out" | sed 's/^/  /'
  if echo "$out" | grep -q '"hasCredential":true'; then
    echo "PASS  mcp  隧道连通且凭据由宿主注入"
  else
    echo "FAIL  mcp  凭据未注入"; fail=1
  fi
else
  echo "$out" | sed 's/^/  /'
  echo "FAIL  mcp  隧道调用失败"; fail=1
fi

# 5) 入箱审查：含疑似密钥的工作区默认拒绝
echo
echo "== 入箱审查 =="
mkdir -p "$TMP/risky"
printf 'sk-%s\n' "$(date +%s%N)" > "$TMP/risky/leak.txt"
if "$BIN" run fake --workspace "$TMP/risky" --exec true >/dev/null 2>&1; then
  echo "FAIL  admission  含密钥的工作区被放行"; fail=1
else
  echo "PASS  admission  含密钥的工作区被拒绝"
fi
if "$BIN" run fake --workspace "$TMP/risky" --exec true --allow-risky >/dev/null 2>&1; then
  echo "PASS  admission  --allow-risky 可显式覆盖并留痕"
else
  echo "FAIL  admission  --allow-risky 未生效"; fail=1
fi

# 6) 沙箱行为与红队用例
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
