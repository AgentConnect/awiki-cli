# awiki-cli 安装说明

## 概述

awiki-cli 是 awiki 的命令行客户端，用 Go 编写，通过 CLI 命令编排对后端服务的 API 调用。支持 DID 身份管理、消息收发（私聊/群聊）、群组管理、WebSocket 实时监听等能力。

**技术栈**: Go 1.22 (pure Go, no CGO) + Cobra + SQLite + ANP SDK

---

## 1. 编译工具

### 1.1 Go 1.22

版本基线固定为 Go 1.22.x，不使用 CGO。

```bash
# macOS (Homebrew)
brew install go@1.22

# 或从官网下载
# https://go.dev/dl/

# 验证版本
go version
# go version go1.22.x darwin/arm64
```

### 1.2 ANP Go SDK（远端模块依赖）

awiki-cli 直接使用远端 ANP Go SDK 模块，版本固定为 `v0.8.4`：

```bash
go get github.com/agent-network-protocol/anp/golang@v0.8.4
```

首次拉取依赖时请确保本机可以访问公开 Go module proxy 或对应源码仓库。

### 1.3 Docker 备选（无本地 Go 时）

如本机未安装 Go，可使用 Docker 镜像：

```bash
docker run --rm -v "$PWD":/app -w /app golang:1.22 go build ./...
docker run --rm -v "$PWD":/app -w /app golang:1.22 go test ./...
```

---

## 2. 数据库

awiki-cli 使用 **pure Go SQLite** 作为本地存储，无需安装外部数据库。

### 2.1 自动初始化

数据库文件在首次运行时自动创建和初始化（`EnsureSchema`），位于工作区数据目录下：

```
~/.awiki-cli/data/awiki-cli.db
```

Schema 版本为 v11，包含以下本地表：

| 表名 | 用途 |
|------|------|
| `contacts` | 联系人（owner_did 分区） |
| `messages` | 消息本地缓存（私聊 + 群聊） |
| `e2ee_outbox` | E2EE 加密消息发件箱 |
| `groups` | 群组本地缓存 |
| `group_members` | 群成员本地缓存 |
| `relationship_events` | 关系事件记录 |
| `e2ee_sessions` | E2EE 会话密钥状态 |

视图：`threads`（会话列表）、`inbox`（收件箱）、`outbox`（发件箱）

### 2.2 SQLite 配置

自动设置以下 PRAGMA：

| PRAGMA | 值 | 说明 |
|--------|-----|------|
| `journal_mode` | WAL | 写前日志，支持并发读 |
| `foreign_keys` | ON | 外键约束 |
| `busy_timeout` | 5000ms | 锁等待超时 |

### 2.3 数据库路径覆盖

推荐优先使用工作区根目录覆盖：

```bash
awiki-cli init
```

如需显式切换工作区根目录，只支持：

```bash
export AWIKI_CLI_WORKSPACE_HOME_DIR=~/my-awiki
# 数据库将位于 ~/my-awiki/data/awiki-cli.db
```

`config / data / runtime / cache / logs / identities` 都会固定派生在该工作区下，不再支持单独的目录级环境变量覆盖。

---

## 3. 配置文件

### 3.1 工作区目录布局

awiki-cli 默认采用单根目录工作区模型，默认路径如下：

| 用途 | 默认路径 | 环境变量覆盖 |
|------|----------|-------------|
| 工作区目录 | `~/.awiki-cli/` | `AWIKI_CLI_WORKSPACE_HOME_DIR` |
| 配置目录 | `~/.awiki-cli/` | 无 |
| 数据目录 | `~/.awiki-cli/data/` | 无 |
| runtime 目录 | `~/.awiki-cli/runtime/` | 无 |
| 缓存目录 | `~/.awiki-cli/cache/` | 无 |
| 日志目录 | `~/.awiki-cli/logs/` | 无 |

> 说明：`~/.awiki-cli/` 是跨平台固定的工作区目录（Windows 对应 `%USERPROFILE%\.awiki-cli\`），也是默认唯一入口。  
> `AWIKI_CLI_WORKSPACE_HOME_DIR` 只负责切换整个工作区根目录；`config / data / runtime / cache` 不再允许分别配置。  
> `AWIKI_CLI_WORKSPACE_HOME_DIR` 之外的旧 `AWIKI_* / AVIKI_* / E2E_*` 业务环境变量不再驱动 awiki-cli；若工作区仍保留上一版的 `config.json`，CLI 会在首次访问工作区时自动迁移到 `config.yaml`。
>
> 工作区内容包括：
>
> - `config.yaml`
> - `identities/`
> - `data/awiki-cli.db`
> - `cache/`
> - `runtime/`
> - `logs/`
> - workspace upgrade 元数据
> - upgrade lock / journal
> - 备份快照

### 3.2 config.yaml

配置文件位于 `~/.awiki-cli/config.yaml`。推荐先执行 `awiki-cli init` 自动创建最小配置；如需手动创建，可参考仓库根目录的 `config.template.yaml`，或直接使用下面的模板：

```yaml
schema_version: 1
identity:
  active: default
runtime:
  mode: websocket
  socket_path: ""
  listener:
    enabled: true
    auto_install: true
    auto_start: true
  host_notify:
    enabled: true
    sink: log
    file_path: ""
    openclaw:
      hook_url: ""
      token: ""
output:
  format: json
  no_color: false
services:
  service_base_url: https://awiki.ai
  did_domain: awiki.ai
  anp_service_endpoint: https://awiki.ai/anp-im/rpc
  anp_service_did: did:wba:awiki.ai
  ca_bundle: ""
```

默认值说明：

- `runtime.mode` 默认是 `websocket`
- `runtime.socket_path` 默认是：
  - macOS / Linux: `<workspace>/runtime/message-daemon.sock`
  - Windows: `\\\\.\\pipe\\awiki-cli-<workspace-hash>`
- `runtime.listener.enabled` 默认是 `true`
- `runtime.listener.auto_install` 默认是 `true`
- `runtime.listener.auto_start` 默认是 `true`
- 在默认 websocket 模式下，`awiki-cli init` 和 `awiki-cli runtime setup` 会自动安装并启动 listener 系统服务
- `runtime.host_notify.enabled` 默认是 `true`
- `runtime.host_notify.sink` 在启用后默认是 `log`，可选 `noop | log | file | openclaw`
- `runtime.host_notify.file_path` 只在 `sink = file` 时生效；未填写时默认是 `<workspace>/runtime/host-notify.events.jsonl`
- `runtime.host_notify.openclaw.hook_url` 通常不需要手工填写；awiki-cli 会优先读取 `~/.openclaw/openclaw.json` 中的 `gateway.port` 和 `hooks.path` 自动推导有效的 webhook URL
- `runtime.host_notify.openclaw.token` 可直接写入 `config.yaml`，也可通过 `OPENCLAW_HOOK_TOKEN` 环境变量提供；两者都未设置时，awiki-cli 会回退读取 `~/.openclaw/openclaw.json` 中的 `hooks.token`
- `output.format` 默认是 `json`
- `services.service_base_url` 默认是 `https://awiki.ai`
- `services.did_domain` 默认是 `awiki.ai`
- `services.anp_service_endpoint` 默认推导为 `https://<did_domain>/anp-im/rpc`
- `services.anp_service_did` 默认推导为 `did:wba:<did_domain>`

配置优先级固定为：

```text
flag > config.yaml > default
```

> 该文件可选。未创建时所有配置使用默认值。  
> `anp_service_endpoint` 和 `anp_service_did` 用于生成本地 DID 文档中的 `ANPMessageService`，同时 `anp_service_did` 也是 group/attachment 控制面默认使用的 service DID。它们和 `service_base_url` 的职责不同：
>
> - `service_base_url`：域内 user-service / content / group / message 的统一基础地址
> - 域内 message RPC：`<service_base_url>/im/rpc`
> - 域内 message WebSocket：`<service_base_url>/im/ws`
> - `anp_service_endpoint`：对外公开到 DID 文档里的 RPC 地址
> - `anp_service_did`：对外公开到 DID 文档里的 bare-domain service DID

### 3.3 本地开发配置

连接本地后端服务时，创建如下 `config.yaml`：

```yaml
schema_version: 1
identity:
  active: default
runtime:
  mode: websocket
  listener:
    enabled: true
    auto_install: true
    auto_start: true
  host_notify:
    enabled: true
    sink: log
    openclaw:
      hook_url: ""
services:
  service_base_url: https://awiki.test
  did_domain: awiki.test
  anp_service_endpoint: https://awiki.test/anp-im/rpc
  anp_service_did: did:wba:awiki.test
  ca_bundle: ""
```

服务地址、运行模式、输出格式、身份默认值都应通过 `config.yaml` 管理；除了 `AWIKI_CLI_WORKSPACE_HOME_DIR` 以外，不再支持环境变量覆盖这些业务配置。

### 3.4 DID 文档中的 ANP Service 约束

`awiki-cli` 在生成 DID 文档时，会自动写入一个公开的 `ANPMessageService` 条目。为了避免把本地实现细节暴露到 DID 文档里，当前实现会拒绝以下配置：

- `localhost`
- `127.0.0.1` / `::1` 等 loopback 地址
- `ws://` / `wss://` URL
- 带 fragment 的 `serviceDid`
- 非 bare-domain 的 `did:wba` service DID（例如 `did:wba:example.com:services:message:e1_local`）

推荐做法：

- `anp_service_endpoint` 使用公开 HTTPS RPC 地址，例如 `https://awiki.ai/anp-im/rpc`
- `anp_service_did` 使用裸域名 DID，例如 `did:wba:awiki.ai`

### 3.5 身份文件布局

DID 身份存储在 `~/.awiki-cli/identities/` 下，每个身份一个子目录：

```
identities/
├── index.json                    # 身份索引（默认身份、凭证列表）
└── <identity-dir>/
    ├── identity.json             # 身份元数据
    ├── auth.json                 # JWT token 缓存
    ├── did_document.json         # DID 文档
    ├── key-1-private.pem         # Ed25519 身份私钥
    ├── key-1-public.pem          # Ed25519 身份公钥
    ├── e2ee-signing-private.pem  # E2EE 签名私钥
    ├── e2ee-agreement-private.pem # E2EE 密钥协商私钥
    └── e2ee-state.json           # E2EE 会话状态
```

> 私钥文件权限为 `0600`，目录权限为 `0700`。

当前 `awiki-cli` 的活跃身份规范为 `e1` / Ed25519 `key-1`。当你把 Python v1 `awiki-agent-id-message` 本地数据默认升级到 Go 版 workspace 时，CLI 会自动尝试把已导入的 handle `k1` DID 通过 `replace_did` 换绑为新的 `e1` DID，并同步重绑本地 SQLite 的 `owner_did`。替换前，旧 DID document、旧私钥和旧 identity 目录会备份到 `identities/.legacy-backup/replace-did/`；这些备份仍包含敏感密钥材料，不要上传或分享。若个别身份无法自动替换，升级会继续完成，但会把失败原因记录到 upgrade warning 与 `doctor` 输出中，后续需要手动处理。

同一 handle 在本地联系人缓存中若经历 DID 切换，`awiki-cli` 会保留对应的历史 DID 映射，并在按 handle 读取 direct inbox/history 时聚合这些历史 DID 关联的消息；如需排查本地记录，可使用 `awiki-cli debug db handle-history <handle>` 查看。

同一轮默认升级还会对旧 `awiki-agent-id-message` skill 做 best-effort 清理：停止并卸载旧 listener service，删除旧 skill 安装目录，并移除旧 OpenClaw `HEARTBEAT.md` 中引用 legacy skill 的 awiki section，避免新旧 skill 同时生效。

### 3.6 环境变量完整列表

| 环境变量 | 用途 | 默认值 |
|----------|------|--------|
| `AWIKI_CLI_WORKSPACE_HOME_DIR` | 工作区根目录 | `~/.awiki-cli` |

> 除 `AWIKI_CLI_WORKSPACE_HOME_DIR` 外，其他 awiki-cli 配置环境变量已停止支持；它们不会再覆盖 `config.yaml` 中的业务配置。

---

## 4. 编译与运行

### 4.1 安装依赖

```bash
cd awiki-cli
go mod tidy
```

### 4.2 编译

```bash
# pure Go 编译（必须 CGO_ENABLED=0）
CGO_ENABLED=0 go build -o awiki-cli ./cmd/awiki-cli/
```

### 4.3 验证

```bash
# 版本信息
./awiki-cli version

# 系统诊断（检查配置、身份、数据库、运行环境）
./awiki-cli doctor

# 查看某个 handle 在本地记录过哪些历史 DID
./awiki-cli debug db handle-history alice

# 查看当前配置
./awiki-cli config show
```

### 4.4 运行测试

```bash
CGO_ENABLED=0 go test ./...
```

### 4.5 代码格式化

```bash
gofmt -w $(find cmd internal -name '*.go')
```

---

## 5. 快速上手

### 5.1 创建身份

```bash
# 创建本地 DID 身份
./awiki-cli id create --name my-identity

# 查看身份列表
./awiki-cli id list

# 查看当前身份
./awiki-cli id current
```

### 5.2 注册 Handle

```bash
# 注册 handle（需要后端服务可用）
./awiki-cli id register --handle myname
```

### 5.3 发送消息

```bash
# 私聊
./awiki-cli msg send --to <handle> --text "hello"

# 群聊
./awiki-cli msg send --group <group-id> --text "hello"

# 发送附件（caption 可选）
./awiki-cli msg send --to <handle> --file ./hello.txt --text "hello attachment"

# 下载附件
./awiki-cli msg attachment download --with <handle> --message-id <msg-id> --output ./downloads/hello.txt

# 查看收件箱
./awiki-cli msg inbox
```

### 5.4 WebSocket 模式

```bash
# 初始化工作区，并自动安装/启动 listener 系统服务
./awiki-cli init

# 显式执行 runtime bootstrap（也会按配置自动 install/start）
./awiki-cli runtime setup --mode websocket

# 按当前 config.yaml 重新收敛 runtime / listener 真实状态
./awiki-cli runtime apply

# 查看监听器状态
./awiki-cli runtime listener status

# 可选：只安装服务定义，不自动启动
./awiki-cli runtime listener install

# 启动 listener 服务；若服务尚未安装，会自动补 install
./awiki-cli runtime listener start

# 停止 / 重启 / 卸载
./awiki-cli runtime listener stop
./awiki-cli runtime listener restart
./awiki-cli runtime listener uninstall

# 查看 / 修改 listener 配置
./awiki-cli runtime listener config show
./awiki-cli runtime listener config set --enabled false
./awiki-cli runtime listener config set --auto-install false --auto-start false

# 高阶快捷开关：改配置后自动 apply
./awiki-cli runtime listener enable
./awiki-cli runtime listener disable

# 查看 / 修改 host notify 配置
./awiki-cli runtime host-notify config show
./awiki-cli runtime host-notify enable
./awiki-cli runtime host-notify disable
./awiki-cli runtime host-notify config set --sink openclaw
./awiki-cli runtime host-notify openclaw set --hook-url http://127.0.0.1:18789/hooks/agent
./awiki-cli runtime host-notify openclaw set-token --value <token>
./awiki-cli runtime host-notify openclaw clear-token
./awiki-cli runtime host-notify openclaw route add --session-key <session-key>
./awiki-cli runtime host-notify openclaw route list
./awiki-cli runtime host-notify openclaw route remove --session-key <session-key>
```

系统服务形态：

- macOS：LaunchAgent
- Linux：systemd
- Windows：Windows Service + Named Pipe

如果你想关闭 realtime listener，改成通过 agent 心跳 / HTTP 轮询收消息，可配置：

```yaml
runtime:
  mode: http
  listener:
    enabled: false
```

或者保留 websocket 配置但不自动管理 listener 服务：

```yaml
runtime:
  mode: websocket
  listener:
    enabled: true
    auto_install: false
    auto_start: false
```

---

## 6. 依赖服务

awiki-cli 是纯客户端，不需要本地数据库服务，但需要连接以下后端：

| 服务 | 用途 | 默认地址 |
|------|------|----------|
| user-service | 用户认证、DID 注册、Handle 管理、群组管理 | `https://awiki.ai` |
| message-service (molt-message) | 消息收发、WebSocket 推送 | `https://awiki.ai` |

本地开发时需先启动这两个后端服务，参考各自的安装说明：
- [user-service 安装说明](../../user-service/docs/installation.md)
- [molt-message 安装说明](../../molt-message/docs/installation.md)

---

## 7. 常见问题

### Q: 编译报错 `CGO_ENABLED` 相关

本项目必须以 `CGO_ENABLED=0` 编译。SQLite 使用的是 pure Go 实现 (`modernc.org/sqlite`)，不依赖 C 库：

```bash
CGO_ENABLED=0 go build ./cmd/awiki-cli/
```

### Q: 编译报错找不到 ANP SDK

确认当前模块依赖已成功下载，并且 `go.mod` 中使用的是远端版本 `github.com/agent-network-protocol/anp/golang v0.8.4`：

```bash
go get github.com/agent-network-protocol/anp/golang@v0.8.4
```

### Q: `go mod tidy` 报错

可能是远端依赖下载失败，或 Go 版本不匹配。确认使用 Go 1.22.x：

```bash
go version
```

### Q: doctor 命令报数据库异常

数据库在首次使用相关命令时自动创建。如需重置：

```bash
rm ~/.awiki-cli/data/awiki-cli.db
```

下次运行会自动重建 schema。

### Q: 连接本地后端服务失败

检查 `config.yaml` 是否正确指向本地服务地址：

```bash
./awiki-cli config show | jq '.data.service_base_url, .data.anp_service_endpoint'
```

### Q: v1 身份迁移

如果之前使用 Python 版 CLI（awiki-agent-id-message），可导入旧身份：

```bash
./awiki-cli id import-v1
```

旧身份目录默认扫描 `~/.openclaw/credentials/awiki-agent-id-message/`。
