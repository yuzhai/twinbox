# 架构

状态：与实现对齐（2026-10-25）。本文描述**已实现**的结构；未实现的部分单列在 §6。

## 1. 目标

1. 沙箱内不存在任何凭据，凭据只在宿主侧被引用。
2. 沙箱内的环境足够完整：主流 agent 不改代码即可运行。
3. 配置成本接近零：不写策略文件，不重复录入已有的密钥。
4. 每条安全承诺都能用命令验证。

## 2. 组件

| 组件 | 包 / 路径 | 运行位置 | 职责 | 禁止 |
| --- | --- | --- | --- | --- |
| CLI | `cmd/twinbox` | 宿主 | `run/doctor/secret/log/prove` | 长期持有明文密钥 |
| 沙箱启动器 | `internal/sandbox` | 宿主 | 生成 bwrap 参数 | 挂载宿主 home、docker.sock、ssh socket |
| 入箱审查 | `internal/admission` + `internal/scan` | 宿主 | 挂载前扫描工作区，命中即拒绝，覆盖需留痕 | 静默放行 |
| 出站代理 | `internal/broker` | 宿主 | 主机与路径判定、header 注入、CONNECT 透传、审计 | 信任请求体自报的目标 |
| 策略 | `internal/policy` | 宿主 | 内置规则，零配置文件 | 读用户手写的策略文件 |
| 密钥库 | `internal/store` | 宿主 | 0600 的 JSON，按名字取值 | 把值下发到沙箱 |
| 审计 | `internal/audit` | 宿主 | 追加式 JSONL | 可被沙箱写入 |
| 适配器 | `internal/agent` | 宿主 | 生成沙箱侧配置与环境变量 | 写入真实密钥 |
| 沙箱内桥 | `cmd/twinbox-shim` | 沙箱 | 127.0.0.1 → unix socket；MCP stdio 隧道 | 持有任何秘密 |
| MCP 网关 | `internal/mcp` | 宿主 | 拉起 MCP server、注入凭据、按名字分发、按 glob 判定工具（deny 优先） | 把原始工具入参交给 shell |

## 3. 数据流

1. **LLM 调用**：agent → `127.0.0.1:8899/anthropic/v1/messages` → shim → unix socket →
   broker 判定并注入 `x-api-key` → 上游。
2. **包管理器**：`CONNECT` → shim → broker → 仅对白名单主机做 TCP 透传，不注入。
3. **审计**：broker 对每次请求写一条 JSONL（host、method、path、status、decision、bytes）。

## 4. 接口（当前实现，provisional）

### 沙箱内环境变量

| 变量 | 含义 |
| --- | --- |
| `TWINBOX=1` | 标记当前进程在沙箱内 |
| `TWINBOX_SOCK` | broker 的 unix socket（沙箱内路径） |
| `TWINBOX_ROUTES` | `端口=虚拟主机` 列表，例如 `8899=api.anthropic.com` |
| `ANTHROPIC_BASE_URL` | 指向 `http://127.0.0.1:8899/anthropic` |
| `ANTHROPIC_API_KEY` | **占位符**，真值从不出现 |

### 宿主侧文件

| 路径 | 内容 | 权限 |
| --- | --- | --- |
| `~/.config/twinbox/secrets.json` | 密钥键值 | 0600 |
| `~/.local/state/twinbox/audit/audit.jsonl` | 审计 | 0600 |
| `~/.local/share/twinbox/homes/<hash>` | 每工作区持久 home | 0700 |

### 审计记录

```json
{"ts":"...","host":"api.anthropic.com","method":"POST","path":"/v1/messages",
 "status":200,"bytes_out":123,"decision":"allow","reason":""}
```

## 5. 生命周期

```
twinbox run <agent>
  ├─ store.Load          读密钥（不传给沙箱）
  ├─ admission.Check     入箱审查：命中即拒绝，--allow-risky 覆盖需留痕
  ├─ broker.ServeUnix    在临时目录启动 unix socket
  ├─ agent.Write         生成沙箱 home 里的配置（只含占位符与本地地址）
  ├─ sandbox.Args        生成 bwrap 参数（unshare-all / clearenv / cap-drop）
  └─ exec bwrap → shim bridge → agent
```

进程随终端退出即销毁；`homes/<hash>` 与审计保留在宿主。

## 6. 未实现（路线图）

| 项 | 现状 | 计划 |
| --- | --- | --- |
| MCP 参数级策略 | 已实现按工具名 glob 判定，deny 优先 | 参数级判定与审批闸门 |
| profile 解包 | 直接用宿主只读根 | OCI 镜像解包 + 版本目录 + 原子切换 |
| macOS / Windows 后端 | 无 | 系统容器 / WSL2，语义不变 |
| 入箱审查 | 已实现：文件名黑名单 + 内容扫描 + 覆盖留痕 | 增加对嵌套目录与二进制的覆盖 |
| 会话命令 | 无 | `clean`、`--new` |
| SSH / 云 API 鉴权 | 默认全禁 | 宿主侧执行器 + 受限 agent 代理 |

## 7. 分工

单人项目。按模块顺序推进，每个阶段单独走 PR：

| 阶段 | 内容 | 状态 |
| --- | --- | --- |
| 1 | CLI、沙箱启动器 | 完成 |
| 2 | broker、策略、审计 | 完成 |
| 3 | 适配器 | 完成（claude / codex / opencode / fake） |
| 4 | MCP 网关 | 完成（stdio 隧道 + 宿主注入） |
| 5 | 红队脚本、文档 | 完成（12 条沙箱断言） |
