# awiki-cli 安装说明

## 概述

awiki-cli 是 awiki 的命令行客户端，用 Go 编写，通过 CLI 命令编排对后端服务的 API 调用。支持 DID 身份管理、消息收发（私聊 / 群聊）、群组管理、WebSocket 实时监听等能力。

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

awiki-cli 直接使用远端 ANP Go SDK 模块，版本固定为 `v0.7.2`：

```bash
go get github.com/agent-network-protocol/anp/golang@v0.7.2
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

数据库文件在首次运行时自动创建和初始化（`EnsureSchema`），位于 awiki-cli 工作目录下：

- 默认工作目录（AWIKI_HOME）：
  - macOS / Linux：`$HOME/.awiki-cli`
  - Windows：`%LOCALAPPDATA%\AwikiCli`
- 数据库路径：`$AWIKI_HOME/db/awiki-cli.db`

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

### 2.2 工作目录覆盖（AWIKI_HOME / init）

awiki-cli 使用一个统一的“工作目录”存放所有本地数据、配置和日志。

日常使用推荐通过 `awiki-cli init` 来初始化工作目录：

```bash
# 使用默认工作目录（例如 ~/.awiki-cli）
./awiki-cli init

# 使用自定义工作目录，并在默认目录下写入 home.json 指针
./awiki-cli init --home "$HOME/my-awiki"
```

高级场景（如 CI、系统测试或一次性试验）仍可以通过环境变量 `AWIKI_HOME` 覆盖默认位置：

```bash
export AWIKI_HOME="$HOME/my-awiki"
# 数据库:     $AWIKI_HOME/db/awiki-cli.db
# 配置文件:   $AWIKI_HOME/config.json
# 身份数据:   $AWIKI_HOME/identities/
# 运行日志:   $AWIKI_HOME/logs/
# 缓存与临时: $AWIKI_HOME/cache/ / $AWIKI_HOME/tmp/
```

> 提示：awiki-cli 会在需要时自动创建上述目录，权限为 `0700`，数据库和敏感文件权限为 `0600`。普通用户不需要长期在 shell rc 中设置 `AWIKI_HOME`；一旦通过 `init --home` 选择工作目录后，后续调用无需再记住路径。

---

## 3. 配置与工作目录

### 3.1 目录布局

本轮改造后，awiki-cli 的本地文件全部收敛到单一工作目录（`AWIKI_HOME`）：

```text
$AWIKI_HOME/
  config.json          # 运行期主配置
  db/awiki-cli.db      # SQLite 数据库
  identities/          # 本地身份与密钥
  logs/                # 运行日志
  cache/               # 缓存数据
  tmp/                 # 临时文件 / runtime 状态
```

- macOS / Linux 默认工作目录根：`$HOME/.awiki-cli`
- Windows 默认工作目录根：`%LOCALAPPDATA%\AwikiCli`
- 运行时解析工作目录根的规则：
  1. 若设置环境变量 `AWIKI_HOME`，本次运行优先使用该路径（高级/临时覆写入口）；
  2. 否则，如果默认根目录下存在 `home.json` 指针文件，则读取其中的 `root_dir` 字段作为真实工作目录根（由 `awiki-cli init --home` 生成）；
  3. 否则使用默认根本身。

### 3.2 config.json 结构

配置文件为标准 JSON（不支持注释、尾逗号），路径为：`$AWIKI_HOME/config.json`。

示例：

```json
{
  "services": {
    "domain": "awiki.ai"
  },
  "identity": {
    "active": "default"
  },
  "runtime": {
    "mode": "http"
  },
  "output": {
    "format": "json",
    "no_color": false
  },
  "update": {
    "disable_strict_version": false,
    "metadata_cache_ttl_seconds": 0
  }
}
```

字段说明（与实现保持一致）：

- `services.domain`：后端域名（例如 `awiki.ai` / `awiki.test`），CLI 内部据此推导各服务 URL：
  - user-service：`https://<domain>`
  - message-service：`https://<domain>/message-service`
  - WebSocket：`wss://<domain>/message-service/ws`
- `identity.active`：当前活跃身份名称（如 `default`）。
- `runtime.mode`：运行模式，`"http"` 或 `"websocket"`。
- `output.format`：输出格式，如 `"json"` / `"table"` 等。
- `output.no_color`：是否禁用彩色输出。
- `update.disable_strict_version`：是否关闭严格版本校验（目前仅作为配置入口）。
- `update.metadata_cache_ttl_seconds`：版本元数据缓存 TTL（0 表示使用内部默认值）。

首次运行如果没有 `config.json`，awiki-cli 使用内置默认值；当通过后续命令需要持久化配置时，会自动创建该文件并写入当前生效值。

### 3.3 运行期环境变量覆盖

运行期只保留少量 `AWIKI_*` 环境变量，用于临时覆盖配置（优先级：**命令行 flag > 环境变量 > config.json > 默认值**）：

| 环境变量 | 用途 | 默认值 |
|----------|------|--------|
| `AWIKI_HOME` | 临时覆盖工作目录根路径（高级/CI/测试用） | 见 3.1 |
| `AWIKI_IDENTITY` | 临时覆盖活跃身份 (`identity.active`) | 空（使用 config.json 或身份索引） |
| `AWIKI_RUNTIME_MODE` | 临时覆盖运行模式 (`runtime.mode`) | `http` |
| `AWIKI_FORMAT` | 临时覆盖输出格式 (`output.format`) | `json` |
| `AWIKI_NO_COLOR` | 临时覆盖是否禁用颜色 (`output.no_color`) | `false` |

> 注意：不再提供 `AWIKI_CONFIG_DIR` / `AWIKI_DATA_DIR` / `AWIKI_STATE_DIR` / `AWIKI_CACHE_DIR`，也不再提供 `AWIKI_USER_SERVICE_URL` 等 URL 级环境变量，更不再兼容任何 `AVIKI_*` / `E2E_*` 变量。服务端域名等长期配置统一通过 `config.json` 管理。

### 3.4 身份文件布局

DID 身份存储在 `$AWIKI_HOME/identities/` 下，每个身份一个子目录：

```text
identities/
├── index.json                    # 身份索引（默认身份、凭证列表）
└── <identity-dir>/
    ├── identity.json             # 身份元数据
    ├── auth.json                 # JWT token 缓存
    ├── did_document.json         # DID 文档
    ├── key-1-private.pem         # secp256k1 身份私钥
    ├── key-1-public.pem          # secp256k1 身份公钥
    ├── e2ee-signing-private.pem  # E2EE 签名私钥
    ├── e2ee-agreement-private.pem # E2EE 密钥协商私钥
    └── e2ee-state.json           # E2EE 会话状态
```

- 目录权限：`0700`
- 私钥 / 凭证文件权限：`0600`

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

# 初始化工作目录（推荐在首次安装后执行一次）
./awiki-cli init
# ./awiki-cli init --home "$HOME/my-awiki"  # 可选：自定义工作目录

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
./awiki-cli msg send --to <handle> --content "hello"

# 群聊
./awiki-cli msg send --group <group-id> --content "hello"

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
| message-service | 消息收发、WebSocket 推送 | `https://awiki.ai` |

本地开发时需先启动这两个后端服务，参考各自的安装说明：

- user-service 安装说明：`../../user-service/docs/installation.md`
- message-service 安装说明：`../../message-service/docs/installation.md`

---

## 7. 常见问题

### Q: 编译报错 `CGO_ENABLED` 相关

本项目必须以 `CGO_ENABLED=0` 编译。SQLite 使用的是 pure Go 实现 (`modernc.org/sqlite`)，不依赖 C 库：

```bash
CGO_ENABLED=0 go build ./cmd/awiki-cli/
```

### Q: 编译报错找不到 ANP SDK

确认当前模块依赖已成功下载，并且 `go.mod` 中使用的是远端版本 `github.com/agent-network-protocol/anp/golang v0.7.2`：

```bash
go get github.com/agent-network-protocol/anp/golang@v0.7.2
```

### Q: `go mod tidy` 报错

可能是远端依赖下载失败，或 Go 版本不匹配。确认使用 Go 1.22.x：

```bash
go version
```

### Q: doctor 命令报数据库异常

数据库在首次使用相关命令时自动创建。如需重置：

```bash
rm -rf "$AWIKI_HOME/db/awiki-cli.db"
```

下次运行会自动重建 schema。

### Q: 连接本地后端服务失败

检查 `config.json` 是否正确指向本地服务域名，或通过环境变量临时覆盖：

```bash
./awiki-cli config show | jq '.data.user_service_url, .data.message_service_url'
```

### Q: v1 身份迁移

如果之前使用 Python 版 CLI（awiki-agent-id-message），可导入旧身份：

```bash
./awiki-cli id import-v1
```

旧身份目录默认扫描 `~/.openclaw/credentials/awiki-agent-id-message/`。
