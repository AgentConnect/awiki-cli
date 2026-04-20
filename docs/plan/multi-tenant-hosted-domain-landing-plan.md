# awiki-cli Hosted Multi-Domain 落地技术计划

**状态**：Landing Plan
**来源**：`docs/architecture/multi-tenant-hosted-domain-implementation.md`
**目标目录**：`awiki-cli/docs/plan/`
**适用范围**：`awiki-cli` Go 实现中的 workspace 配置、初始化命令、DID 文档生成、doctor 诊断、runtime/listener 与多 workspace 使用方式
**非目标**：不支持单 workspace 多 active domain；不改变 ANP / DID WBA 协议；不把 `peer_routes` 暴露为 CLI 主路径；不引入新的 workspace config schema 字段

---

## 1. 文档目的

本文把 `docs/architecture/multi-tenant-hosted-domain-implementation.md` 的方案拆成可执行工程计划，用于指导 awiki-cli 接入 hosted multi-domain tenant 架构。

目标形态：服务端一套 `user-service` + 一套 `message-service` 可托管多个 domain，而 CLI 的每个 workspace 只激活一个 home domain：

```text
~/.awiki-cli-a -> services.did_domain = a.example.com
~/.awiki-cli-b -> services.did_domain = b.example.com
~/.awiki-cli-c -> services.did_domain = c.example.com
```

CLI 需要保证：

1. 用户能通过命令初始化或切换 workspace 的 active home domain，不必手工改 YAML。
2. 本地 DID 文档中的 `ANPMessageService.serviceEndpoint` 与 `serviceDid` 和该 home domain 的服务发现语义一致。
3. `doctor` 能识别多域名配置错误，并对高级网关部署给出 warning 而不是误报失败。
4. 多域名用户通过多个 workspace 并行使用同一套后端。

---

## 2. 权威参考顺序

实现、测试和文档修订时按以下顺序取权威：

1. `docs/architecture/multi-tenant-hosted-domain-implementation.md`
2. `docs/architecture/anp-service-discovery.md`
3. `docs/architecture/awiki-command-v2.md`
4. `docs/architecture/awiki-v2-architecture.md`
5. `docs/architecture/output-format.md`
6. `../message-service/docs/api/`
7. `../user-service/docs/api/`
8. `../awiki-agent-id-message/`
9. `../cli/`

若发现 CLI 行为需要服务端 API 变化，先登记并同步对应服务仓库 API 文档，不得只改 CLI。

---

## 3. 当前实现基线与缺口

### 3.1 当前代码基线

| 领域 | 当前形态 | 主要位置 |
| --- | --- | --- |
| 配置模型 | 已有 `services.service_base_url`、`did_domain`、`anp_service_endpoint`、`anp_service_did` | `internal/config/config.go` |
| 默认推导 | 默认仍以 `awiki.ai` 为主；`anp_service_endpoint` / `anp_service_did` 可从 `did_domain` 推导 | `internal/config/config.go` |
| init | `init` 负责 workspace / config / DB / listener 初始化，尚未暴露 `--domain` 等 flags | `internal/cli/init.go` |
| config 命令 | 已有 `config show`，尚无 `config services show/set` 子命令 | `internal/cli/root.go` |
| DID 文档 | 已有 ANP service 构造与 endpoint/service DID 校验 | `internal/identity/anp_service.go`, `internal/identity/did.go` |
| doctor | 已有配置、identity、SQLite、legacy 路径等诊断，需强化 services / ANP service 检查 | `internal/doctor/doctor.go` |
| message/runtime | 已通过 resolved config 使用 HTTP/WSS endpoint 和 ANP service 信息 | `internal/message/*`, `internal/runtime/*` |

### 3.2 主要缺口

1. 用户无法通过 `awiki-cli init --domain a.example.com` 生成完整 multi-domain services 配置。
2. 缺少 `config services set/show` 作为不创建身份、不启动 listener 的服务配置入口。
3. `doctor` 对 multi-domain 默认部署 / 高级网关部署的错误与 warning 还不够明确。
4. 内建 docs / schema / cmdmeta 还没有描述 `config services` 与 `init --domain`。
5. 多 workspace E2E 还没有专门覆盖三域名 direct / group / attachment / listener 链路。

---

## 4. 固定设计原则

1. **一个 workspace 一个 active domain**：同一 workspace 不同时激活多个 tenant。
2. **不新增 config schema 字段**：继续使用 `services.*` 四个 canonical 字段。
3. **DID domain 与 API gateway 分离**：`did_domain` 是身份 namespace，`service_base_url` 是 API 入口。
4. **默认同域，允许高级覆盖**：默认 URL 都从 `--domain` 推导；高级部署可覆盖 API gateway / ANP endpoint。
5. **service DID 必须是 bare-domain DID**：默认 `did:wba:<domain>`，高级覆盖也必须是 bare-domain did:wba。
6. **public CLI 不暴露 user_id**：输出仍遵守 handle-first 规则，`did` 只用于协议级定位。
7. **多域名并行用多个 workspace**：不引入 profile 自动切换或多租户账户管理。

---

## 5. 目标配置模型

### 5.1 canonical services 字段

`config.yaml` 保持现有 schema：

```yaml
services:
  service_base_url: https://a.example.com
  did_domain: a.example.com
  anp_service_endpoint: https://a.example.com/anp-im/rpc
  anp_service_did: did:wba:a.example.com
```

字段语义：

| 字段 | 语义 | 默认推导 |
| --- | --- | --- |
| `service_base_url` | CLI 调用 user/message/content API 的公共入口基地址 | `https://<domain>` |
| `did_domain` | 当前 workspace 新建 DID 的 home domain | `--domain` |
| `anp_service_endpoint` | 写入 DID 文档的 `ANPMessageService.serviceEndpoint` | `https://<domain>/anp-im/rpc` |
| `anp_service_did` | 写入 DID 文档的 `ANPMessageService.serviceDid` | `did:wba:<domain>` |

### 5.2 domain normalization

`--domain` 与 `services.did_domain` 规范化规则：

1. trim 首尾空白；
2. lowercase；
3. 去尾点；
4. 不接受包含 `://`、`/`、空白或端口的 URL / host:port；
5. 空值报错。

### 5.3 高级覆盖

支持高级部署：

```bash
awiki-cli config services set \
  --domain a.example.com \
  --service-base-url https://api.a.example.com \
  --anp-service-endpoint https://a.example.com/anp-im/rpc \
  --anp-service-did did:wba:a.example.com
```

约束：

- `service_base_url` 必须是 HTTP(S) URL。
- `anp_service_endpoint` 必须是 HTTP(S) URL，不得是 WS/WSS。
- 默认生产语义下，`anp_service_endpoint` 不得指向 localhost / loopback。
- `anp_service_did` 必须是 bare-domain DID：`did:wba:<domain>`。
- endpoint host 与 `did_domain` 不一致时 warning，不直接失败。

---

## 6. 分阶段实施计划

## CLI-MTD-0：计划与现状基线

**状态**：已完成（2026-04-20）。已新增本落地计划文档，明确当前实现基线、阶段拆分、验收标准与实现边界，并同步 `CLAUDE.md` 成员清单。

### 目标

建立本计划文档，明确当前代码基线、阶段拆分、验收标准与后续实现边界。

### 主要工作

- 新增本文件到 `docs/plan/`。
- 更新 `CLAUDE.md` 成员清单。
- 记录不新增 schema、不支持单 workspace 多 active domain 等约束。

### 验收

- `docs/plan/multi-tenant-hosted-domain-landing-plan.md` 存在。
- `CLAUDE.md` 成员清单包含该文档。
- `git diff --check` 通过。

---

## CLI-MTD-1：services 配置解析与校验收口

**状态**：已完成（2026-04-20）。已在 `internal/config` 收口 domain normalization、默认 services 推导和 services diagnostic 校验；`Resolve` 会规范化合法 `did_domain`，并从该 domain 派生 `service_base_url`、`anp_service_endpoint` 与 `anp_service_did` 的默认值。

### 目标

把 domain normalization、default derivation、高级覆盖校验收口为可复用配置逻辑，供 `init`、`config services set`、`doctor`、DID 生成共用。

### 主要工作

- 在 `internal/config` 增加或收敛 services resolver helper：
  - `NormalizeDomain(raw string) (string, error)`
  - `DefaultServicesForDomain(domain string) ServicesConfig`
  - `ValidateServices(services, mode) []Diagnostic`
- 对旧 workspace 缺失字段继续兼容推导：
  - 缺 `anp_service_endpoint`：从 `did_domain` 推导 `https://<did_domain>/anp-im/rpc`。
  - 缺 `anp_service_did`：从 `did_domain` 推导 `did:wba:<did_domain>`。
- 保持 `service_base_url` 与 `did_domain` 语义分离，不从 API URL 反推 DID domain。

### 验收

- `--domain A.Example.COM.` 规范化为 `a.example.com`。
- `--domain https://a.example.com` 报错。
- 高级 endpoint / service DID 不被默认值覆盖。
- legacy config 解析不回归。

---

## CLI-MTD-2：`init --domain` onboarding

**状态**：已完成（2026-04-20）。`init` 命令已支持 `--domain`、`--service-base-url`、`--anp-service-endpoint`、`--anp-service-did`；dry-run 会输出 planned services 且不落盘，非 dry-run 会写入或更新 `services.*`。

### 目标

用户可以用一条命令初始化指定 home domain 的 workspace。

### 目标命令

```bash
awiki-cli init --domain a.example.com
```

新增 flags：

| Flag | 说明 |
| --- | --- |
| `--domain` | active DID / Handle domain，推荐必填 |
| `--service-base-url` | 高级覆盖，默认 `https://<domain>` |
| `--anp-service-endpoint` | 高级覆盖，默认 `https://<domain>/anp-im/rpc` |
| `--anp-service-did` | 高级覆盖，默认 `did:wba:<domain>` |

### 行为

- 没有 `--domain` 时保持当前 init 兼容行为。
- 有 `--domain` 时写入或更新 `services.*` 四个字段。
- `--dry-run` 输出计划，不创建目录、不写 config、不初始化 DB、不安装 listener。
- 输出包含：
  - `workspace.root_dir`
  - `config_file`
  - `services.normalized_domain`
  - `services.service_base_url`
  - `services.anp_service_endpoint`
  - `services.anp_service_did`

### 验收

- `init --domain a.example.com --dry-run --format json` 输出 planned services 且不落盘。
- `init --domain a.example.com` 写入完整 services。
- 既有无 `--domain` init 测试继续通过。

---

## CLI-MTD-3：`config services show/set`

**状态**：已完成（2026-04-20）。已新增 `config services show` 与 `config services set` 命令；`set` 复用 `init` 的 services 推导/校验逻辑并支持 `--dry-run`，`show` 输出 active services、sources 与 diagnostics。

### 目标

提供独立服务配置入口，不创建身份、不启动 listener。

### 命令树

```text
config
  show
  services
    show
    set
```

### `config services set`

```bash
awiki-cli config services set --domain b.example.com
```

行为：

- 加载现有 config，不存在则按最小 config 生成。
- 写入 services 字段。
- 支持与 `init --domain` 相同的高级覆盖 flags。
- 支持 `--dry-run`。
- 不创建 identity，不改 default identity，不启动 listener。

### `config services show`

输出：

```json
{
  "services": {
    "service_base_url": "https://b.example.com",
    "did_domain": "b.example.com",
    "anp_service_endpoint": "https://b.example.com/anp-im/rpc",
    "anp_service_did": "did:wba:b.example.com"
  },
  "sources": {},
  "warnings": []
}
```

### 验收

- `config services set --domain b.example.com` 后 `config services show` 可见新 domain。
- `--dry-run` 不写文件。
- 高级覆盖 warning 可通过 show / doctor 观察。

---

## CLI-MTD-4：DID 生成与服务发现对齐

**状态**：已完成（2026-04-20）。已锁定 DID 文档生成字段来源：`id create/register/replace-did` 使用 resolved `services.did_domain` 作为 DID/proof domain，并使用 `services.anp_service_endpoint` / `services.anp_service_did` 生成 `ANPMessageService`；测试覆盖 service base URL 与 DID domain 分离。

### 目标

确保 identity 生命周期始终使用 services 配置，而不是从 API URL 或其他字段推导 DID 文档。

### 主要工作

- 检查并锁定：
  - `id create`
  - `id register`
  - `id replace-did`
- DID 文档字段来源：

| DID 文档字段 | 来源 |
| --- | --- |
| DID id host | `services.did_domain` |
| proof domain / challenge target | `services.did_domain` |
| `ANPMessageService.serviceEndpoint` | `services.anp_service_endpoint` |
| `ANPMessageService.serviceDid` | `services.anp_service_did` |

### 验收

- `id create` 在 `did_domain = b.example.com` 时生成 `did:wba:b.example.com:...`。
- DID document service endpoint 与 service DID 等于配置值。
- `service_base_url` 改为 `https://api.b.example.com` 不影响 DID id host。

---

## CLI-MTD-5：doctor 多域名诊断

**状态**：已完成（2026-04-20）。`doctor` 的 `anp_service` 检查已接入 `config.ValidateServices()` diagnostics，能区分 blocking error 与 advanced deployment warning，并输出 service base URL、DID domain、ANP endpoint、service DID 与 diagnostics 详情。

### 目标

`doctor` 能解释当前 workspace 的 active domain 与 ANP service 配置状态。

### 检查项

| 检查 | 级别 | 说明 |
| --- | --- | --- |
| `services.did_domain` 为空 | error | 无法生成 DID |
| `service_base_url` 非 HTTP(S) | error | API 入口无效 |
| `anp_service_endpoint` 非 HTTP(S) | error | DID 文档不可发布 |
| endpoint localhost / loopback | error 或 dev warning | 生产不可发布本地地址 |
| `anp_service_did` 非 bare-domain `did:wba` | error | 服务身份语义不成立 |
| endpoint host != did_domain | warning | 高级网关部署 |
| `anp_service_did != did:wba:<did_domain>` | warning | 高级服务身份部署 |

### 验收

- doctor 输出包含 `anp_service` 或等价 section。
- 本地 `*.awiki.test` 开发配置不误判为 fatal。
- localhost endpoint 的级别符合实现约定，并有清晰 hint。

---

## CLI-MTD-6：runtime / listener / message 兼容边界

**状态**：已完成（2026-04-20）。已补充 message / attachment 测试锁定边界：HTTP RPC 与 websocket 仍以 `service_base_url` 为 API 入口，attachment control target 使用配置的 `anp_service_did`，运行时不引入多 active domain 状态。

### 目标

保持单 active domain workspace 模型，确保消息与 listener 不引入多租户状态复杂度。

### 主要工作

- HTTP message / group / attachment 继续通过 `service_base_url` 调用域内 API。
- DID 文档和服务发现继续通过 `anp_service_endpoint/anp_service_did` 表达公开消息服务。
- listener 使用当前 workspace 的 identity 与 service URL，不同时连接多个 tenant。
- docs 明确多域名并行用多个 workspace。

### 验收

- runtime listener 现有测试不回归。
- `msg send --to`、`group create`、`msg send --file` 读取 config 的行为不变。
- 多 workspace 示例文档完整。

---

## CLI-MTD-7：迁移与兼容

**状态**：已完成（2026-04-20）。已锁定 `config services set` 不改写 identity store；`doctor` 会在 identity DID domain 与 active `services.did_domain` 不一致时给出 warning，提示换 home domain 应使用新 workspace 或显式维护命令。

### 目标

保护既有 workspace，同时给用户明确换 home domain 的路径。

### 主要工作

- 旧 workspace 缺 `anp_service_endpoint` / `anp_service_did` 时继续推导并 warning。
- 不自动迁移现有 identity DID domain。
- 换 domain 推荐新 workspace：

```bash
AWIKI_CLI_WORKSPACE_HOME_DIR=~/.awiki-cli-b awiki-cli init --domain b.example.com
```

- 若用户必须迁移 DID，要求显式维护命令或重新注册，不由 `config services set` 隐式修改 identity。

### 验收

- 旧 config 测试继续通过。
- 修改 services 不会改写 identity store。
- `doctor` 能提示 identity DID domain 与 services DID domain 不一致的风险。

---

## CLI-MTD-8：文档、schema 与系统测试同步

**状态**：已完成（2026-04-20）。已同步 README / installation / command architecture / ANP discovery / 总实施计划 / built-in docs topic / system-test 规划，并确认 schema/help 已包含 `init --domain` 与 `config services show/set`。

### 目标

让 CLI 产品面、内建 docs/schema、安装说明、系统测试规划与新配置入口一致。

### 文档同步

至少更新：

- `README.md`
- `docs/installation.md`
- `docs/architecture/awiki-command-v2.md`
- `docs/architecture/anp-service-discovery.md`
- `docs/architecture/multi-tenant-hosted-domain-implementation.md`（如实现偏差）
- `docs/plan/awiki-v2-implementation-plan.md`
- `internal/docs/topics.go` 对应内建 docs 主题
- `CLAUDE.md`

### schema / cmdmeta

- `internal/cmdmeta/catalog.go` 增加 `config services show/set`。
- `schema` 输出包含新命令与 flags。
- help 文案保持 handle-first，不暴露 `user_id`。

### 系统测试规划

在 `awiki-system-test` 或后续 v2 E2E 中增加三 workspace 覆盖：

```text
workspace a -> a.awiki.test
workspace b -> b.awiki.test
workspace c -> c.awiki.test
```

覆盖：

1. 三 workspace 分别 `init --domain`。
2. 三 workspace 分别注册 handle / DID。
3. DID 文档中的 `ANPMessageService` 与各自 domain 一致。
4. Alice@a 给 Bob@b 发 direct。
5. Alice@a 创建 group，Bob@b / Carol@c 收 group 消息。
6. Bob@b 在 a-host group 里发附件，CLI 下载成功。
7. listener 在不同 workspace 下连接对应 domain。

---

## 7. 测试计划

### 7.1 单元测试

- `internal/config`
  - domain normalization。
  - default services derivation。
  - advanced override validation。
  - legacy missing endpoint / service DID fallback。
- `internal/identity`
  - DID document host = `services.did_domain`。
  - `ANPMessageService.serviceEndpoint/serviceDid` = config values。
- `internal/doctor`
  - errors / warnings matrix。
- `internal/cli`
  - init flags / dry-run / write behavior。
  - config services show/set command behavior。

### 7.2 命令级测试

```bash
AWIKI_CLI_WORKSPACE_HOME_DIR=$(mktemp -d) \
  go test ./internal/cli -run 'TestInit.*Domain|TestConfigServices'
```

### 7.3 全仓验证

```bash
gofmt -w $(find cmd internal -name '*.go')
go test ./...
```

如本机 Go 不可用，使用项目约定的 Docker Go 1.22 环境执行。

### 7.4 E2E 验收

后续统一联调阶段执行：

```bash
cd ../awiki-system-test
uv run pytest tests_v2/message_service -v
```

实际命令以系统测试落地文件为准。

---

## 8. 验收标准

1. `awiki-cli init --domain a.example.com` 能写入完整 canonical `services.*`。
2. `awiki-cli config services set/show` 可独立修改和展示 active domain 配置。
3. `id create/register/replace-did` 生成 DID 文档时使用正确 DID domain、ANP endpoint 与 service DID。
4. `doctor` 能识别无效 domain、无效 URL、loopback endpoint、非 bare service DID，并正确区分 error / warning。
5. 旧 workspace 继续可读；缺失新字段时有推导和提示。
6. 多 workspace 使用方式有 README / installation 文档示例。
7. `schema` / help / built-in docs 包含新命令与 flags。
8. `go test ./...` 通过。
9. 后续系统联调覆盖 a/b/c 三 workspace direct / group / attachment / listener。

---

## 9. 风险与防护

| 风险 | 防护 |
| --- | --- |
| 用户把 URL 误填为 domain | `NormalizeDomain` 拒绝 URL / path / host:port |
| DID domain 被 service_base_url 反推污染 | DID 生成层只读 `services.did_domain` |
| 高级网关部署被 doctor 误判失败 | endpoint host mismatch 降为 warning |
| loopback endpoint 被发布到公网 DID 文档 | doctor error 或生产语义 error；文档明确禁止 |
| 修改 services 隐式破坏现有 identity | `config services set` 不迁移 identity DID domain |
| 单 workspace 多 tenant 复杂度失控 | 明确不支持；推荐多个 workspace |
| schema/help/docs 漏同步 | CLI-MTD-8 明确 cmdmeta、docs topic 与 README 更新 |

---

## 10. 建议提交拆分

1. `docs: add hosted domain CLI landing plan`
2. `config: derive services from active domain`
3. `cli: support init domain service flags`
4. `cli: add config services commands`
5. `identity: align DID documents with service config`
6. `doctor: diagnose hosted domain service config`
7. `docs: document multi-workspace hosted domains`
8. `test: plan hosted domain CLI E2E`
