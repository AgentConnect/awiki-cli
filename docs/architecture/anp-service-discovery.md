# awiki-cli DID 文档中的 ANP Service 方案

## 1. 目标

本文记录 `awiki-cli` 在生成 Agent DID 文档时，`ANPMessageService` 应如何填写，以及本地配置应该如何组织。

设计依据：

- `anp/AgentNetworkProtocol/chinese/message/02-身份与发现.md`
- 本仓库当前 CLI / runtime / identity 结构

当前结论：

- 每个 Agent DID 文档只公开一个 `ANPMessageService`
- `serviceEndpoint` 指向 **公开 HTTP RPC 入口**
- `serviceDid` 使用 **bare-domain did:wba DID**
- 当前 **不声明** `anp.direct.e2ee.v1` / `direct-e2ee`

## 2. DID 文档填写规则

`awiki-cli` 生成 DID 文档时，固定写入如下语义的 service 条目：

```json
{
  "id": "<agent_did>#message",
  "type": "ANPMessageService",
  "serviceEndpoint": "https://example.com/message/rpc",
  "serviceDid": "did:wba:example.com",
  "profiles": [
    "anp.core.binding.v1",
    "anp.direct.base.v1",
    "anp.attachment.v1"
  ],
  "securityProfiles": [
    "transport-protected"
  ]
}
```

说明：

- `serviceEndpoint` 是 DID 文档里的公开发现地址，不是 CLI 本地 bridge、listener socket 或 websocket 地址
- `serviceDid` 是服务身份提示字段，当前固定要求为 bare-domain DID
- direct E2EE 还未作为 awiki-cli 的公开互通能力启用，因此不写 `anp.direct.e2ee.v1` / `direct-e2ee`

## 3. 本地配置项

`config.yaml` 的 `services` 下新增两个显式字段：

```yaml
services:
  did_domain: "awiki.ai"
  anp_service_endpoint: "https://awiki.ai/message/rpc"
  anp_service_did: "did:wba:awiki.ai"
```

职责拆分：

- `did_domain`：决定本地生成的 DID 域
- `anp_service_endpoint`：写入 DID 文档的公开 RPC 地址
- `anp_service_did`：写入 DID 文档的 service DID
- `message_service_url`：CLI 调用消息服务时使用
- `message_service_ws_url`：runtime listener 使用

默认值：

- `anp_service_endpoint = https://<did_domain>/message/rpc`
- `anp_service_did = did:wba:<did_domain>`

环境变量：

- `AWIKI_ANP_SERVICE_ENDPOINT`
- `AWIKI_ANP_SERVICE_DID`

## 4. 校验规则

为了避免把本地实现细节写进 DID 文档，CLI 在生成 DID 文档和执行 `doctor` 时都做以下校验：

- `anp_service_endpoint` 只能是 `http` 或 `https`
- 不能使用 `localhost`
- 不能使用 loopback IP（如 `127.0.0.1`、`::1`）
- 不能使用 websocket URL
- `anp_service_did` 必须是 `did:wba:*`
- `anp_service_did` 不能带 fragment
- `anp_service_did` 必须是 bare-domain DID，不能带额外路径段

当前 **不要求** `serviceEndpoint` 和 `serviceDid` 必须指向同一个 home message-service；两者各自只做格式与公开性校验。

## 5. 实施记录

本次落地包含：

- `internal/config/config.go`
  - 新增 `anp_service_endpoint` / `anp_service_did`
  - 新增对应环境变量和默认推导
- `internal/identity/did.go`
  - 生成 DID 文档时自动写入 `ANPMessageService`
- `internal/identity/anp_service.go`
  - 封装 ANP Service 默认值、校验与 service 构造
- `internal/doctor/doctor.go`
  - 新增 `anp_service` 检查项
- `docs/installation.md`
  - 补充配置样例、环境变量和约束说明

## 6. 验收点

- `awiki-cli id create` 生成的 DID 文档包含且仅包含一个 `ANPMessageService`
- `profiles` 只包含 direct base + attachment
- `securityProfiles` 只包含 `transport-protected`
- 无效 `anp_service_endpoint` / `anp_service_did` 会在 DID 生成和 `doctor` 中暴露
