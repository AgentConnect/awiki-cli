# awiki-cli 多域名托管实现方案

**状态**：Implementation Plan
**来源**：根目录《多租户方案.md》第 4 / 9.1 / 10 / 12 节
**适用范围**：`awiki-cli` 客户端配置、初始化、DID 生成、doctor 诊断和多 workspace 使用方式
**目标读者**：CLI 开发者、系统联调负责人、面向用户编写安装/使用文档的维护者

---

## 1. 目标与边界

本方案描述 `awiki-cli` 如何接入 hosted multi-domain tenant 架构：服务端一套 `user-service` + 一套 `message-service` 可以托管多个域名，客户端在一个 workspace 内只选择一个 active domain。

目标：

1. 用户无需手工编辑 YAML 即可把当前 workspace 初始化到指定 home domain。
2. 本地 DID 文档中的 `ANPMessageService.serviceEndpoint` 和 `serviceDid` 与该 home domain 一致。
3. `doctor` 可以识别多域名配置错误，并对高级部署给出 warning 而非误判失败。
4. 多域名用户通过多个 workspace 并行使用同一套后端。

非目标：

- 不让同一个 workspace 同时激活多个 domain。
- 不在本次实现 profile 自动切换或多租户账户管理。
- 不改变 ANP / DID WBA 协议，不把 `peer_routes` 暴露为客户端主路径。
- 不要求 `service_base_url`、`did_domain`、`anp_service_endpoint` 永远同 host；默认同域，高级部署允许显式覆盖。

---

## 2. 客户端租户模型

### 2.1 一个 workspace 一个 active domain

`awiki-cli` 的 identity store、JWT、listener、SQLite owner DID、attachment download 状态都围绕当前 identity 工作。多域名托管下，客户端仍保持：

```text
workspace A -> services.did_domain = a.example.com
workspace B -> services.did_domain = b.example.com
workspace C -> services.did_domain = c.example.com
```

推荐使用独立 workspace：

```bash
AWIKI_CLI_WORKSPACE_HOME_DIR=~/.awiki-cli-a awiki-cli init --domain a.example.com
AWIKI_CLI_WORKSPACE_HOME_DIR=~/.awiki-cli-b awiki-cli init --domain b.example.com
```

### 2.2 每个 domain 对应一个 bare service DID

客户端不自行生成服务 DID，只把服务端暴露的 bare-domain DID 写入本地配置并发布到用户 DID 文档：

```text
a.example.com -> did:wba:a.example.com
b.example.com -> did:wba:b.example.com
c.example.com -> did:wba:c.example.com
```

不得把多个托管域名统一写成同一个 `did:wba:awiki.ai`，否则 direct / group / attachment 的服务发现和 proof 语义会混乱。

---

## 3. 配置模型

### 3.1 canonical services 字段

`config.yaml` 的租户相关配置保持现有字段，不引入新的 workspace schema：

```yaml
services:
  service_base_url: https://a.example.com
  did_domain: a.example.com
  anp_service_endpoint: https://a.example.com/anp-im/rpc
  anp_service_did: did:wba:a.example.com
```

字段语义：

| 字段 | 语义 | 默认推导 |
|---|---|---|
| `service_base_url` | CLI 调用用户/消息服务的公共入口基地址 | `https://<domain>` |
| `did_domain` | 本 workspace 新建 DID 的 home domain | `--domain` |
| `anp_service_endpoint` | 写入 DID 文档的 `ANPMessageService.serviceEndpoint` | `https://<domain>/anp-im/rpc` |
| `anp_service_did` | 写入 DID 文档的 `ANPMessageService.serviceDid` | `did:wba:<domain>` |

### 3.2 默认推导规则

实现 `--domain a.example.com` 时，默认写入：

```yaml
services:
  service_base_url: https://a.example.com
  did_domain: a.example.com
  anp_service_endpoint: https://a.example.com/anp-im/rpc
  anp_service_did: did:wba:a.example.com
```

domain 规范化规则：

1. 去除首尾空白。
2. 转小写。
3. 去掉末尾 `.`。
4. 不接受 URL 作为 `--domain`；URL 应通过高级参数显式传入。

### 3.3 高级覆盖规则

允许高级部署把 public user/message gateway 与 DID domain 分离：

```bash
awiki-cli config services set \
  --domain a.example.com \
  --service-base-url https://api.a.example.com \
  --anp-service-endpoint https://a.example.com/anp-im/rpc \
  --anp-service-did did:wba:a.example.com
```

高级覆盖必须保持以下不变量：

- `did_domain` 是 DID namespace，不是 API URL。
- `anp_service_endpoint` 必须是 `http` 或 `https` URL，不得是 `ws` / `wss`。
- 默认生产配置中 `anp_service_endpoint` 不得指向 localhost / loopback。
- `anp_service_did` 必须是 bare-domain DID：`did:wba:<domain>`。

---

## 4. 命令设计

### 4.1 `init --domain`

目标命令：

```bash
awiki-cli init --domain a.example.com
```

行为：

1. 初始化 workspace 目录和本地 store。
2. 如果 config 不存在，生成最小 `config.yaml`。
3. 写入或更新 `services.*` 四个字段。
4. 输出结构化结果，包含 normalized domain、service base URL、ANP endpoint、service DID 与 workspace path。

建议 flags：

| Flag | 必填 | 说明 |
|---|---|---|
| `--domain` | 推荐必填 | active DID / Handle domain |
| `--service-base-url` | 否 | 高级覆盖，默认 `https://<domain>` |
| `--anp-service-endpoint` | 否 | 高级覆盖，默认 `https://<domain>/anp-im/rpc` |
| `--anp-service-did` | 否 | 高级覆盖，默认 `did:wba:<domain>` |

如果 `init` 已经存在，新增 flag 时保持旧行为兼容：没有 `--domain` 时继续使用模板默认或既有 config。

### 4.2 `config services set`

目标命令：

```bash
awiki-cli config services set --domain a.example.com
```

行为与 `init --domain` 相同，但只负责修改服务配置，不创建身份，不启动 listener。

命令树建议：

```text
config
  show
  services
    show
    set
```

`config services show` 输出应只展示 `services.*` 与来源信息，方便用户确认 active domain。

### 4.3 dry-run 与输出契约

所有配置写入命令应支持全局 `--dry-run`，输出拟写入内容但不落盘：

```bash
awiki-cli init --domain a.example.com --dry-run --format json
```

输出中不得暴露 `user_id`。对外身份仍以 handle / DID 协议字段为主。

---

## 5. DID 生成与服务发现

`id create`、`id register`、`id replace-did` 生成 DID 文档时必须读取解析后的 `services.*`：

| DID 文档字段 | 来源 |
|---|---|
| DID id host | `services.did_domain` |
| proof domain / challenge target | `services.did_domain` |
| `ANPMessageService.serviceEndpoint` | `services.anp_service_endpoint` |
| `ANPMessageService.serviceDid` | `services.anp_service_did` |

示例：

```json
{
  "id": "did:wba:a.example.com:user:alice:e1_xxx",
  "service": [
    {
      "id": "#message",
      "type": "ANPMessageService",
      "serviceEndpoint": "https://a.example.com/anp-im/rpc",
      "serviceDid": "did:wba:a.example.com",
      "profiles": [
        "anp.core.binding.v1",
        "anp.direct.base.v1",
        "anp.group.base.v1",
        "anp.attachment.v1"
      ],
      "securityProfiles": ["transport-protected"]
    }
  ]
}
```

DID 生成层不得从 `service_base_url` 反推 DID domain；`service_base_url` 是客户端 API 入口，`did_domain` 才是身份 namespace。

---

## 6. doctor 诊断规则

`awiki-cli doctor` 增加或强化 `anp_service` 检查：

| 检查 | 级别 | 说明 |
|---|---|---|
| `services.did_domain` 为空 | error | 无法生成规范 DID |
| `service_base_url` 非 HTTP(S) | error | CLI HTTP API 入口无效 |
| `anp_service_endpoint` 非 HTTP(S) | error | DID 文档不能发布非 HTTP(S) endpoint |
| `anp_service_endpoint` 是 localhost / loopback | error 或本地环境 warning | 生产 DID 文档不可发布本地地址 |
| `anp_service_did` 不是 `did:wba:<domain>` bare DID | error | 服务身份语义不成立 |
| 默认模式下 endpoint host 与 `did_domain` 不一致 | warning | 允许高级网关部署 |
| 默认模式下 `anp_service_did != did:wba:<did_domain>` | warning | 允许高级部署，但需人工确认 |

本地开发环境如使用 `*.awiki.test` / `localhost`，可按现有 env / build profile 降级为 warning，但文档和默认模板不得鼓励公网 DID 发布 loopback endpoint。

---

## 7. 兼容与迁移

1. 现有 `config.yaml` schema 不变，只更新字段默认值和命令写入方式。
2. `config.template.yaml` 默认值从固定 `awiki.ai` 调整为可解释的示例，并说明应通过 `init --domain` 生成真实配置。
3. 旧 workspace 没有 `anp_service_did` 时，配置解析层可从 `did_domain` 推导 `did:wba:<did_domain>`，同时 `doctor` 提示用户写回配置。
4. 旧 workspace 没有 `anp_service_endpoint` 时，配置解析层可从 `did_domain` 推导 `https://<did_domain>/anp-im/rpc`。
5. 不自动迁移 identity DID domain；用户如果要换 home domain，应创建新 workspace 或显式执行维护命令。

---

## 8. 测试计划

### 8.1 单元测试

- `config.Resolve`：
  - `--domain a.example.com` 推导四个 `services.*` 字段。
  - 高级自定义 endpoint / service DID 不被默认值覆盖。
  - domain normalization 正确处理大小写和尾点。
- DID 生成：
  - DID id host 使用 `services.did_domain`。
  - proof domain 使用 `services.did_domain`。
  - `ANPMessageService.serviceEndpoint` 使用配置 endpoint。
  - `ANPMessageService.serviceDid` 使用配置 service DID。
- doctor：
  - 合法多域配置通过。
  - endpoint host 与 DID domain 不一致时 warning。
  - loopback endpoint 在生产语义下失败或 warning。
  - 非 bare service DID 失败。

### 8.2 集成 / 系统测试对接

在 `awiki-system-test` 中使用三个 workspace：

```text
~/.awiki-cli-a -> a.awiki.test
~/.awiki-cli-b -> b.awiki.test
~/.awiki-cli-c -> c.awiki.test
```

覆盖：

1. 三个 workspace 分别注册 DID + Handle。
2. DID 文档中的 `ANPMessageService` 与各自 domain 一致。
3. Alice / Bob / Carol 跨 domain direct message 互通。
4. 跨 domain group create / join / send 互通。
5. 三个 listener 都能收到 direct / group 下行。
6. direct / group attachment 可上传和下载。

---

## 9. 文档同步清单

实现本方案时同步检查：

- `config.template.yaml`：示例改为 domain-driven，说明默认推导。
- `README.md` / `docs/installation.md`：新增 `init --domain` onboarding。
- `docs/architecture/anp-service-discovery.md`：补充 hosted multi-domain 下 service DID 规则。
- `docs/architecture/awiki-command-v2.md`：登记 `config services set/show` 或 `init --domain` flags。
- `CLAUDE.md`：新增本架构文档入口，保持分形文档成员清单同步。
