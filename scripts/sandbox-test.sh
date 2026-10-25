#!/bin/sh
# 沙箱行为检查：这些断言必须在沙箱内成立，任何一条失败都视为不通过。
set -u

fail=0
pass=0

check() { # check <编号> <说明> <命令>
  n="$1"; desc="$2"; shift 2
  if sh -c "$*" >/dev/null 2>&1; then
    echo "PASS  $n  $desc"
    pass=$((pass + 1))
  else
    echo "FAIL  $n  $desc"
    fail=$((fail + 1))
  fi
}

check 1 "环境变量中没有真实密钥" '! env | grep -qiE "REAL|ghp_[A-Za-z0-9]{20}|AKIA[0-9A-Z]{16}"'
check 2 "~/.ssh 不可见" '[ ! -e "$HOME/.ssh" ]'
check 3 "~/.aws 不可见" '[ ! -e "$HOME/.aws" ]'
check 4 "无法读取 /etc/shadow" '! cat /etc/shadow >/dev/null 2>&1'
check 5 "网络只有 loopback" '[ "$(ls /proc/net/dev | grep -c .)" -ge 0 ] && [ "$(grep -c ":" /proc/net/dev)" -le 4 ]'
check 6 "普通外网请求不被策略拒绝" '[ "$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 https://example.com)" != "403" ]'
check 7 "云元数据地址被拒绝" '[ "$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 http://169.254.169.254/)" = "403" ]'
check 8 "docker socket 不存在" '[ ! -e /var/run/docker.sock ]'
check 9 "ssh-agent socket 未传入" '[ -z "${SSH_AUTH_SOCK:-}" ]'
check 10 "宿主 home 未挂载" '[ ! -e /home/'"$(id -un)"' ] || [ "$(ls /home 2>/dev/null | wc -l)" -le 1 ]'
check 11 "工作区可写" 'touch .twinbox-probe && rm -f .twinbox-probe'
check 12 "broker socket 类型正确" '[ -S /run/twinbox/broker.sock ]'

echo
echo "sandbox-test: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
