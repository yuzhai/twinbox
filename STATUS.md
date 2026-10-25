# 状态

最后更新：2026-10-25。本文件在每次合并后更新，作为**无需读 PR 也能了解进展**的入口。

## 验收方式

一条命令，机器给结论：

```bash
cd ~/qbox/twinbox && make build && make verify
```

期望最后一行：`verify: 全部通过`。

真实流程另有一条（消耗额度，需显式执行）：

```bash
make e2e-real    # 期望：PASS e2e-real agent 完成对话 / 工具调用写入了工作区文件
```

**注意：完整流程只能在本地跑。** GitHub Actions 的 ubuntu-24.04 runner 设置了
`kernel.apparmor_restrict_unprivileged_userns=1`，实测 `bwrap` 启动失败（`BWRAP_FAIL`）。
CI 只覆盖 vet、单测、构建与 `prove`；沙箱与真实流程必须在开启非特权 userns 的 Linux 上执行。它依次执行 vet、全部单测、构建、红队脚本
（端到端注入、审计、撤销、MCP 隧道、入箱审查、12 条沙箱内断言）。

补充检查：

| 命令 | 期望 |
| --- | --- |
| `./bin/twinbox doctor` | bwrap 与目录 PASS；未安装的 agent 显示为不阻塞的 FAIL |
| `./bin/twinbox prove .` | `可公开性检查通过` |
| `git log --oneline` | 提交分布在开发窗口内，不是单次提交 |

## 已完成

| 能力 | 证据 |
| --- | --- |
| 沙箱隔离（unshare-all / clearenv / cap-drop / 不挂宿主 home） | 沙箱内 12 条断言 |
| 凭据不落地（占位符 + 边界注入） | 端到端断言上游收到真值、沙箱内只有占位符 |
| 出站默认放行但全审计 | 普通外网与 npm 可达；云元数据地址 403；凭据只注入白名单主机 |
| MCP 隧道（凭据在宿主） | `hasCredential:true` 且 `$HOME` 无真值 |
| MCP 参数级策略（allow/deny glob） | 单测 + 网关拦截 |
| 入箱审查（黑名单 + 内容扫描 + 覆盖留痕） | 拒绝与覆盖两条端到端断言 |
| 审计与对账 | JSONL 记录 + `twinbox log` |
| 可撤销 | 删除凭据后调用失败 |
| 可公开性检查 | `prove` 在 CI 中执行 |
| 真实 agent 端到端（opencode） | `make e2e-real`：真实网关 200、工具调用写入文件 |
| 环境 profile（镜像解包 + overlay 根） | `profile install ubuntu:24.04` 后沙箱内为 24.04，宿主为 26.04 |

## 队列（按价值排序）

| 优先级 | 项 | 前置 |
| --- | --- | --- |
| P0 | 真实 agent 端到端（claude / codex 安装并对接本地假上游） | 需要安装二进制 |
| P1 | profile 版本清理与引用保护 | 无 |
| P2 | macOS / Windows 后端 | 需要对应平台 |
| P3 | 会话命令（clean / --new） | 无 |
| P4 | 演示视频 | 需要有麦克风环境 |

## 阻塞项（需要人决定）

1. 是否安装 claude / codex 做真实 e2e（会下载数百 MB）。
2. 是否投入时间做 profile 解包；不做则 README 的"环境可复现"需降级表述。
3. 视频录制时间与出镜方式。

## 自主运行规则（我会遵守的边界）

- 只通过 PR 改 `main`；每个 PR 必须 CI 通过、`make verify` 本地通过后才合并。
- 不引入第三方依赖；不修改用户全局配置；不触碰 `~/.ssh`、`~/.aws`、各 agent 的 home。
- 不使用真实凭据；所有测试用假上游与占位符。
- 不 force-push `main`；不删除他人分支；不重写已合并历史。
- 每完成一个 PR 更新本文件；遇到需要决策的事，写入"阻塞项"并停下，不猜测。
