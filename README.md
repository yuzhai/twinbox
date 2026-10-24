# TwinBox

**Your machine's twin — with none of your keys.**

让编程 agent（Claude Code、Codex、opencode2）在桌面沙箱里获得**完整**的工作环境，
但沙箱内**不含任何凭据**：API key、token、密码全部留在宿主侧。

> 状态：v0，Linux 后端可用，MCP 隧道与镜像 profile 未实现。
> 目标：2026-10-26 产出首个可用版本。

## 问题

我们已经有能力让 agent 替我们操作真实系统，却没有一种方式能让它拿不到我们的秘密：
要么把完整的机器和凭据都交给它，要么把它关进一个干不了活的笼子。

完整的问题拆解、10 个使用场景（含 SSH）与成功判据见 **[docs/problem.md](docs/problem.md)**。

## 快速开始

```bash
make build
./bin/twinbox doctor          # 检查 bwrap、目录、agent 二进制
./bin/twinbox secret set anthropic <你的 key>   # 只写宿主侧，0600
./bin/twinbox run claude      # 进入沙箱；agent 看到的是占位符
./bin/twinbox log             # 查看出站审计
./bin/twinbox prove .         # 仓库可公开性检查

# 可复现环境（需要 docker，仅安装期使用）
./bin/twinbox profile install ubuntu:24.04
./bin/twinbox run claude --profile ubuntu-24.04
# 工作区里含疑似密钥时，run 会拒绝挂载；确认无害可加 --allow-risky（记入审计）
```

真实流程验证（用本机已有的 opencode 配置，密钥只在宿主内存中派生）：

```bash
make e2e-real        # 真实 agent + 真实网关，会消耗模型额度
```

离线验证（不需要网络与账号）：

```bash
make verify                   # vet + 单测 + 构建 + 红队
make attack                   # 12 条沙箱断言 + 端到端 + 撤销
```

## 设计方向

选型依据见 **[docs/solution-analysis.md](docs/solution-analysis.md)**（需求基线、业界做法差异、匹配度矩阵）。

- **隔离**：Linux 用 bubblewrap（非特权、独立网络命名空间、出站默认拒绝）。
- **环境**：宿主只读根（v1）；镜像 profile 解包为路线图。
- **凭据**：沙箱内只出现占位符，真实值由宿主侧 broker 在边界注入。
- **MCP**：服务端运行在宿主，沙箱内只有 `twinbox-shim mcp <name>`，配置零敏感信息。
- **会话**：状态持久在宿主磁盘，进程随终端退出即销毁。
- **易用**：不写配置文件，不重复录入机器上已有的 key。

## 文档

- [问题定义](docs/problem.md)
- [威胁模型](docs/threat-model.md)
- [方案分析](docs/solution-analysis.md)
- [架构](docs/architecture.md)
- [测试与验收](docs/testing.md)
- [未决问题](docs/open-questions.md)
- [演示脚本](docs/demo-script.md)

## 当前限制

- 仅 Linux 后端（macOS / Windows 为路线图）
- MCP 已支持 stdio 隧道与工具级 allow/deny；参数级判定与审批尚未实现
- 环境默认使用宿主只读根；`profile install` 已支持镜像解包（overlay 只读根），版本清理未做

## 来源说明

本项目从零开始，未引入任何第三方 codebase，因此不需要 `v0` tag。
若后续必须引入外部代码，该导入必须单独成为一个 commit 并打 `v0` tag。
