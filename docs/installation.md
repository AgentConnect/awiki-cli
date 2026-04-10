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

awiki-cli 直接使用远端 ANP Go SDK 模块，版本固定为 `v0.8.3`：

```bash
go get github.com/agent-network-protocol/anp/golang@v0.8.3
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
> 若检测到旧的 `AWIKI_* / AVIKI_* / E2E_*` 配置环境变量，或检测到旧的 `config.yaml`，CLI 会直接报错并要求迁移。
>
> 工作区内容包括：
>
> - `config.json`
> - `identities/`
> - `data/awiki-cli.db`
> - `cache/`
> - `runtime/`
> - `logs/`
> - workspace upgrade 元数据
> - upgrade lock / journal
> - 备份快照

### 3.2 config.json

配置文件位于 `~/.awiki-cli/config.json`。推荐先执行 `awiki-cli init` 自动创建最小配置；如需手动创建，可参考仓库根目录的 `config.template.json`，或直接使用下面的模板：

```json
{
  "schema_version": 1,
  "identity": {
    "active": "default"
  },
  "runtime": {
    "mode": "websocket",
    "socket_path": ""
  },
  "output": {
    "format": "json",
    "no_color": false
  },
  "services": {
    "service_base_url": "https://awiki.ai",
    "did_domain": "awiki.ai",
    "anp_service_endpoint": "https://awiki.ai/anp-im/rpc",
    "anp_service_did": "did:wba:awiki.ai",
    "ca_bundle": ""
  }
}
```

默认值说明：

- `runtime.mode` 默认是 `websocket`
- `runtime.socket_path` 默认是 `<workspace>/runtime/message-daemon.sock`
- `output.format` 默认是 `json`
- `services.service_base_url` 默认是 `https://awiki.ai`
- `services.did_domain` 默认是 `awiki.ai`
- `services.anp_service_endpoint` 默认推导为 `https://<did_domain>/anp-im/rpc`
- `services.anp_service_did` 默认推导为 `did:wba:<did_domain>`

配置优先级固定为：

```text
flag > config.json > default
```

> 该文件可选。未创建时所有配置使用默认值。  
> `anp_service_endpoint` 和 `anp_service_did` 专门用于生成本地 DID 文档中的 `ANPMessageService`。它们和 `service_base_url` 的职责不同：
>
> - `service_base_url`：域内 user-service / content / group / message 的统一基础地址
> - 域内 message RPC：`<service_base_url>/im/rpc`
> - 域内 message WebSocket：`<service_base_url>/im/ws`
> - `anp_service_endpoint`：对外公开到 DID 文档里的 RPC 地址
> - `anp_service_did`：对外公开到 DID 文档里的 bare-domain service DID

### 3.3 本地开发配置

连接本地后端服务时，创建如下 `config.json`：

```json
{
  "schema_version": 1,
  "identity": {
    "active": "default"
  },
  "runtime": {
    "mode": "websocket"
  },
  "services": {
    "service_base_url": "https://awiki.test",
    "did_domain": "awiki.test",
    "anp_service_endpoint": "https://awiki.test/anp-im/rpc",
    "anp_service_did": "did:wba:awiki.test",
    "ca_bundle": ""
  }
}
```

服务地址、运行模式、输出格式、身份默认值都应通过 `config.json` 管理；除了 `AWIKI_CLI_WORKSPACE_HOME_DIR` 以外，不再支持环境变量覆盖这些业务配置。

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

当前 `awiki-cli` 不再保留本地 `k1` / secp256k1 `key-1` 兼容转换逻辑；活跃身份应使用当前生成的 `e1` / Ed25519 `key-1` 材料。若本地仍保留旧 `k1` 身份，请重新创建或通过 `id recover` 迁移到新的 `e1` 身份。

### 3.6 环境变量完整列表

| 环境变量 | 用途 | 默认值 |
|----------|------|--------|
| `AWIKI_CLI_WORKSPACE_HOME_DIR` | 工作区根目录 | `~/.awiki-cli` |

> 除 `AWIKI_CLI_WORKSPACE_HOME_DIR` 外，其他 awiki-cli 配置环境变量已停止支持。若检测到旧变量（例如 `AWIKI_WORKSPACE_HOME`、`AWIKI_USER_SERVICE_URL`、`AVIKI_*`、`E2E_*`），CLI 会直接报错并要求把业务配置迁移到 `config.json`。

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
# 切换到 WebSocket 模式
./awiki-cli runtime mode set websocket

# 启动后台监听器
./awiki-cli runtime listener start

# 查看监听器状态
./awiki-cli runtime listener status
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

确认当前模块依赖已成功下载，并且 `go.mod` 中使用的是远端版本 `github.com/agent-network-protocol/anp/golang v0.8.3`：

```bash
go get github.com/agent-network-protocol/anp/golang@v0.8.3
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

检查 `config.json` 是否正确指向本地服务地址：

```bash
./awiki-cli config show | jq '.data.service_base_url, .data.anp_service_endpoint'
```

### Q: v1 身份迁移

如果之前使用 Python 版 CLI（awiki-agent-id-message），可导入旧身份：

```bash
./awiki-cli id import-v1
```

旧身份目录默认扫描 `~/.openclaw/credentials/awiki-agent-id-message/`。
