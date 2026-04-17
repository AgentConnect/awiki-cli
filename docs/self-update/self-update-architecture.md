# awiki-cli 自升级与单一 Skill 真相源架构

**文档状态**：Draft v2.0  
**适用范围**：`awiki-cli` 二进制自升级、websocket 升级通知、公开 root skill 同步、skill bundle 生成与读取  
**目标读者**：CLI 开发者、runtime/listener 开发者、发布系统维护者、skill 文档维护者、服务端更新 API 设计者

---

## 1. 文档目的

本文档定义 awiki-cli 下一阶段的自升级与 skill 分发正式方案，目标是把以下几条线收敛成一个一致系统：

- CLI 版本检查与最低支持版本策略
- 二进制下载、校验、安装与切换
- websocket 长连接下的主动升级通知
- 对外唯一公开 root skill 的自动同步
- 公开 root skill 的 URL bootstrap 与初始安装说明
- 仓库 `skills/` 文档与运行时 skill bundle 的单一真相源

本文档优先解决当前设计中的三个不一致：

1. 运行期版本检查不再依赖 npm registry，而统一改为 **awiki 服务端更新 API**。
2. 正式升级路径不再依赖 `npm install -g`，而改为 **CLI 自己管理 workspace 内的版本目录**。
3. skill 文档不再维护两套树状结构，而统一以 **仓库 `skills/` 为唯一人工维护真相源**。

当本文档与历史自升级草稿冲突时，以本文档为准；当本文档与当前仓库已经存在的 `skills/` 制品描述冲突时，以：

- `skills/SKILL.md`
- `skills/README.md`
- `skills/references/*.md`
- `skills/manifests/skills.yaml`

作为 skill 内容编辑真相源，以本文档作为运行时升级与分发架构真相源。

---

## 2. 设计目标

### 2.1 核心目标

1. **运行期版本策略统一由服务端更新 API 决策**  
   CLI 在启动检查、显式升级检查、自动升级、websocket 推送升级这几条路径中，统一读取同一套服务端版本策略。

2. **正式升级统一由 awiki-cli 自己完成**  
   npm 仅承担 bootstrap 安装职责；后续版本切换、校验、回滚边界、状态落盘都由 `awiki-cli` 自己负责。

3. **对外只暴露一个公开 skill，并同时提供 URL bootstrap 入口**  
   对宿主公开的入口固定为：

   ```text
   ~/.agents/skills/awiki-cli/SKILL.md
   ```

   同时提供固定 HTTPS URL 供 Agent 在本地尚未安装 `awiki-cli` 时直接下载 root `SKILL.md`，并按其中说明先通过 npm 完成 bootstrap 安装，再调用 `awiki-cli skill ...` 读取更深内容。deeper docs 默认不常驻公开目录，而由 `awiki-cli skill ...` 动态提供。

4. **skill 文档与 CLI 版本严格对齐**  
   公开 root skill 和 deeper docs 都必须与当前 CLI 版本绑定，不再存在独立的 skill 发布链路。

5. **websocket 模式下支持服务端主动升级通知**  
   listener 建联时上报自身版本与 host 元数据；服务端可在连接存活期间推送升级事件，客户端可按本地策略自动处理。

6. **host 维度先只进入请求，不进入服务端差异化响应**  
   当前版本允许客户端上报 `host_agent / host_version / host_capabilities / skill_format_version`，但服务端首版不返回 host-specific 内容。host 差异先在编译产物中写死。

### 2.2 非目标

本设计当前**不负责**：

- 把 CLI 命令面按 host 做分叉
- 把 deeper docs 全量铺到 `~/.agents/skills/`
- 把服务端 host-specific skill 模板下发协议在首版中做完
- 把 npm 完全移出发布链路（npm 仍可保留 bootstrap 入口）
- 定义 secure messaging 或 people 的业务协议细节

---

## 3. 设计原则

### 原则 1：服务端 API 是运行期版本真相源

CLI 运行时的版本判断、最低支持版本策略、平台匹配 artifact、skill bundle 版本信息，统一来自 awiki 服务端更新 API。

### 原则 2：CLI 自管升级，不委托 npm

一旦 CLI 已经安装完成，后续升级不再依赖 `npm install -g`。workspace 内的版本目录、当前指针、校验与状态文件都由 CLI 自己管理。

### 原则 3：`skills/` 是唯一人工维护真相源

仓库中的 `skills/` 目录是唯一需要人工编辑和审阅的 skill 内容来源；运行时 bundle 只能由它构建而来，不再人工维护第二套 `internal/skill/**` 文档树。

### 原则 4：只公开一个 root skill，并让它可独立完成 bootstrap

对宿主只暴露一个 root `SKILL.md`。所有 deeper docs 都通过 CLI 动态索引、读取、缓存和导出。该 root skill 必须同时满足两种消费方式：

- 已安装 `awiki-cli` 时，作为本地公开 skill 使用
- 未安装 `awiki-cli` 时，作为公开 URL 下载的 bootstrap 文档使用，并指导 Agent 先通过 npm 安装 `awiki-cli`

### 原则 5：默认自动升级，但允许降级策略

默认策略是 **全自动升级**；同时必须允许用户通过配置或命令把行为降级为：

- 后台预下载 + 用户确认
- 仅提醒，不自动执行

### 原则 6：升级事务与 root skill 同步绑定，但失败边界分层

二进制切换成功后再同步 root skill。root skill 同步失败不回滚二进制，但必须留下明确状态和告警。

### 原则 7：websocket 既是消息下行通道，也是升级通知通道

在 websocket 模式下，listener 是唯一长期持有远端连接的组件，因此版本上报与升级推送都应收敛在 listener 连接生命周期内处理。

---

## 4. 当前事实与目标裁决

### 4.1 当前仓库事实

当前仓库中已经存在：

- `internal/update/update.go`：基于 npm registry 的版本检查逻辑
- `internal/cli/upgrade.go`：当前升级命令仍直接调用 `npm install -g @awiki/cli@latest`
- `internal/runtime/listener/wsclient.go`：当前 websocket client，已具备握手 header 能力与通知消费能力
- `skills/SKILL.md`、`skills/references/*.md`、`skills/manifests/skills.yaml`：当前实际 skill 文档制品

### 4.2 本文档的裁决

1. `internal/update` 后续改为服务端更新 API 客户端，不再以 npm registry 为正式运行期来源。  
2. `upgrade` 命令后续改为 CLI 自下载/校验/切换，不再以 `npm install -g` 为正式升级执行路径。  
3. `skills/` 是 skill 文档的唯一人工维护真相源。  
4. 对外 root skill 名称目标统一为 **`awiki-cli`**。若迁移期仍保留旧 `awiki` 名称或 alias，应作为兼容策略处理，而不是继续作为长期正式名称。  
5. host-specific 差异当前先内置到编译产物，不要求服务端首版响应返回 host 定制内容。

---

## 5. 总体架构

```text
远端:
  update check API                    # 运行期版本检查真相源
  websocket notifications             # system.upgrade_available / required
  artifact storage                    # 各平台二进制包
  public root skill URL               # root SKILL.md bootstrap 入口

仓库真相源:
  skills/SKILL.md
  skills/references/*.md
  skills/manifests/skills.yaml

CLI 内置:
  internal/update                     # 更新检查、版本决策、缓存
  internal/upgrader                   # 下载、校验、解压、切换、状态写入
  internal/skillbundle                # 从 skills/ 构建得到的运行时 bundle
  internal/runtime/listener           # websocket 建联、消息通知、升级通知
  compiled host profile               # 编译期内置的 host 元数据与差异行为

本地状态:
  ~/.awiki-cli/versions/<version>/awiki-cli
  ~/.awiki-cli/bin/awiki-cli
  ~/.awiki-cli/state/release-state.json
  ~/.awiki-cli/state/skill-state.json
  ~/.awiki-cli/cache/skill/<cli-version>/**
  ~/.awiki-cli/tmp/**

对外公开:
  ~/.agents/skills/awiki-cli/SKILL.md
  ~/.agents/skills/awiki-cli/.awiki-managed.json
  https://awiki.ai/skills/awiki-cli/SKILL.md
```

---

## 6. 目录结构

### 6.1 仓库内部目录

```text
awiki-cli/
├─ cmd/
├─ internal/
│  ├─ update/                 # 更新检查、版本策略、缓存
│  ├─ upgrader/               # 下载、校验、解压、版本切换、状态文件更新
│  ├─ skillbundle/            # 从 skills/ 构建得到的运行时 bundle（embed/生成代码）
│  ├─ runtime/
│  │  └─ listener/            # websocket 建联、升级通知消费、宿主通知
│  └─ buildinfo/              # CLI 版本、构建信息、host profile 注入入口
├─ skills/
│  ├─ SKILL.md                # root skill 内容真相源
│  ├─ README.md               # skill 结构说明
│  ├─ manifests/skills.yaml   # 结构化索引/校验辅助
│  └─ references/*.md         # deeper docs 真相源
├─ docs/
└─ scripts/
```

说明：

- `skills/` 是**人工维护输入**。
- `internal/skillbundle` 是**构建产物承载层**，不是手工维护的第二套文档树。
- 若未来通过生成代码、embed FS 或构建脚本产出 bundle，均属于 `skillbundle` 实现细节，不改变真相源归属。

### 6.2 用户机器上的运行目录

```text
~/.awiki-cli/
├─ versions/
│  ├─ 1.8.0/awiki-cli
│  └─ 1.8.1/awiki-cli
├─ bin/
│  └─ awiki-cli -> ../versions/1.8.1/awiki-cli
├─ state/
│  ├─ release-state.json
│  └─ skill-state.json
├─ cache/
│  └─ skill/
│     └─ 1.8.1/
│        ├─ SKILL.md
│        └─ references/
│           ├─ 00-installation.md
│           └─ 03-messaging.md
├─ tmp/
└─ upgrade/
   ├─ locks/
   └─ staging/
```

说明：

- `versions/` 保存历史安装版本。
- `bin/awiki-cli` 指向当前激活版本。
- `tmp/` 与 `upgrade/staging/` 用于下载与解压的中间过程。
- `cache/skill/` 保存按 `cli_version + doc_path` 版本化的动态文档缓存。

### 6.3 对外公开的唯一 skill 目录

```text
~/.agents/skills/awiki-cli/
├─ SKILL.md
└─ .awiki-managed.json
```

规则：

- 这里只放 root skill
- 不创建 `awiki-loader/`
- 不创建多个公开子 skill 目录
- deeper docs 不默认落地到公开目录

---

### 6.4 公开 root skill URL

除本地公开目录外，root `SKILL.md` 还需要在固定 HTTPS URL 提供 bootstrap 下载入口：

```text
https://awiki.ai/skills/awiki-cli/SKILL.md
```

约束：

- 该 URL 指向的 root skill 内容必须与当前正式发布的 root skill 渲染结果同源
- 该 URL 面向“本地尚未安装 `awiki-cli`”的 Agent/bootstrap 场景
- 该 URL 内容必须包含初始安装说明：当检测到 `awiki-cli` 不在 PATH 时，先通过 npm 安装 `@awiki/cli`，再按 root skill 中的 `skill index/get/schema` 协议继续读取 deeper docs
- 本地公开 skill 与 URL 下载版 root skill 不允许长期分叉为两套不同内容

---

## 7. 服务端更新检查 API

### 7.1 目标

更新检查 API 是 CLI 运行期版本决策唯一真相源，用于：

- 判断是否有新版本
- 判断当前版本是否已低于最小支持版本
- 返回当前平台可下载 artifact
- 返回当前 CLI 对应的 skill bundle 版本信息
- 为后续 host-specific 策略扩展预留请求字段

### 7.2 请求字段

建议使用：

```http
POST /api/cli/updates/check
Content-Type: application/json
```

请求体示例：

```json
{
  "schema_version": 1,
  "current_version": "1.8.0",
  "current_did": "did:wba:awiki.ai:user:alice",
  "channel": "stable",
  "goos": "darwin",
  "goarch": "arm64",
  "host_agent": "codex",
  "host_version": "5.4",
  "host_capabilities": ["skills", "local_exec", "websocket_runtime"],
  "skill_format_version": "v1"
}
```

字段说明：

- `current_version`：当前 CLI 版本
- `current_did`：当前 active identity 已解析 DID 时携带；没有 active identity 或尚未完成身份初始化时可省略
- `channel`：发布通道，例如 `stable` / `prerelease`
- `goos` / `goarch`：当前平台
- `host_agent`：`codex | claude-code | openclaw | unknown`
- `host_version`：宿主版本，无法判断时可为空
- `host_capabilities`：宿主能力标签数组
- `skill_format_version`：CLI 当前支持的 skill 渲染/索引格式版本

### 7.3 鉴权与 DID 语义

更新检查 API 的 HTTP 请求默认采用 **best-effort identity auth**：

- 如果当前 active identity 已可解析 DID，客户端应在请求体中携带 `current_did`
- 如果当前 identity 已有可用凭证或 bearer token，客户端默认应携带身份认证信息发起请求
- 如果当前没有 active identity、没有可用凭证、或鉴权头生成失败，客户端应退化为匿名请求，而不是因为缺少凭证直接让更新检查失败
- 服务端必须同时兼容：
  - 带身份认证 + 带 `current_did` 的请求
  - 不带认证、也不带 `current_did` 的匿名请求

推荐实现语义：

- 有凭证时：优先发送认证请求
- 无凭证时：直接发送匿名请求
- 鉴权构造失败时：记录 verbose warning，并回退到匿名请求

### 7.4 响应字段

服务端首版**不返回 host-specific 内容**，响应只包含通用版本决策与当前平台 artifact。

示例：

```json
{
  "schema_version": 1,
  "latest_version": "1.8.1",
  "min_supported_version": "1.8.0",
  "channel": "stable",
  "published_at": "2026-04-17T08:30:00Z",
  "artifact": {
    "platform": "darwin-arm64",
    "url": "https://downloads.example.com/awiki-cli/1.8.1/awiki-cli-darwin-arm64.tar.gz",
    "sha256": "f7a0d8d6b1c4...",
    "size": 18432012,
    "signature": "minisign:RWQ..."
  },
  "skill_bundle": {
    "bundle_version": "2026.04.17",
    "bundle_sha256": "4d8f9c3b...",
    "root_skill_sha256": "673ac2..."
  }
}
```

字段说明：

- `latest_version`：服务端建议的最新版本
- `min_supported_version`：最小支持版本；低于该版本时 CLI 可能被阻断
- `artifact`：当前平台匹配后的最终下载项；若当前平台不可用，可返回空并配合错误码或 `artifact: null`
- `skill_bundle`：与该版本绑定的 skill bundle 版本信息

### 7.5 当前版本的 host 语义

当前版本中：

- **请求会带 host 维度**
- **响应暂不带 host 特化内容**
- host 差异先写死在本地 binary 的编译配置中

这意味着：

- 服务端可以记录和观察 host 维度
- 服务端可以基于 host 做统计或灰度策略预留
- 当前 CLI 不依赖服务端返回 host 专属 skill 内容才能工作

---

## 8. websocket 主动升级通知

### 8.1 目标

在 websocket runtime 模式下，listener 已经是唯一长期持有远端连接的组件，因此升级通知也应统一走这条连接下发，避免每次命令执行都重复轮询。

### 8.2 建联上报字段

websocket 握手应携带以下 header；如果后续协议需要，也可在连接建立后的 hello 请求中重复上报：

```text
X-Awiki-CLI-Version
X-Awiki-CLI-Channel
X-Awiki-Host-Agent
X-Awiki-Host-Version
X-Awiki-Host-Capabilities
X-Awiki-Skill-Format-Version
```

要求：

- 这些字段用于升级策略与观测，不替代现有鉴权 header
- 字段值必须来自当前激活 binary 的本地事实
- 当某字段未知时，传空字符串或 `unknown`，不阻断建联

### 8.3 服务端通知事件

服务端首版约定以下事件：

- `system.upgrade_available`
- `system.upgrade_required`

示例：

```json
{
  "jsonrpc": "2.0",
  "method": "system.upgrade_available",
  "params": {
    "latest_version": "1.8.1",
    "min_supported_version": "1.8.0",
    "channel": "stable",
    "artifact": {
      "platform": "darwin-arm64",
      "url": "https://downloads.example.com/awiki-cli/1.8.1/awiki-cli-darwin-arm64.tar.gz",
      "sha256": "f7a0d8d6b1c4...",
      "size": 18432012,
      "signature": "minisign:RWQ..."
    },
    "skill_bundle": {
      "bundle_version": "2026.04.17",
      "bundle_sha256": "4d8f9c3b...",
      "root_skill_sha256": "673ac2..."
    }
  }
}
```

### 8.4 默认处理策略

默认配置下：

```yaml
update:
  auto_upgrade_enabled: true
  auto_upgrade_mode: apply
  allow_ws_push_trigger: true
```

因此 listener 收到升级通知后默认执行：

1. 记录收到的升级事件
2. 下载 artifact 到 staging
3. 校验 hash / signature
4. 解压并切换当前 binary
5. 同步 root skill
6. 更新 release/skill 状态文件

### 8.5 降级策略

后续允许用户通过配置或命令把默认行为降级为：

- `predownload`：后台预下载并校验，但不切换，等待用户确认
- `notify`：只记录 pending update 并提示用户手动执行 `awiki-cli upgrade apply`

当前版本中默认仍以 `apply` 为正式策略。

---

## 9. 本地状态文件

### 9.1 `release-state.json`

```json
{
  "schema_version": 1,
  "current_version": "1.8.1",
  "current_artifact_sha256": "f7a0d8d6b1c4...",
  "channel": "stable",
  "last_checked_at": "2026-04-17T09:00:00Z",
  "last_check_source": "check_api",
  "last_ws_upgrade_event_at": "2026-04-17T09:05:00Z",
  "pending_update": null,
  "last_applied_version": "1.8.1",
  "last_failed_update": null
}
```

说明：

- `last_check_source`：`check_api | cache | ws_push`
- `pending_update`：可为空；在 `notify` 或 `predownload` 模式下用于保存待处理更新
- `last_failed_update`：记录最近一次失败的版本、阶段、错误摘要

### 9.2 `skill-state.json`

```json
{
  "schema_version": 1,
  "current_cli_version": "1.8.1",
  "current_bundle_version": "2026.04.17",
  "current_bundle_sha256": "4d8f9c3b...",
  "root_skill_sync": {
    "dir": "~/.agents/skills",
    "root_skill_sha256": "673ac2...",
    "last_synced_at": "2026-04-17T09:01:20Z",
    "last_sync_status": "ok"
  },
  "cache_policy": {
    "keep_versions": ["1.8.1", "1.8.0"],
    "ttl_days": 30
  }
}
```

说明：

- `root_skill_sync.last_sync_status`：至少支持 `ok | warning | failed`
- `current_bundle_sha256` 用于判定缓存和 root skill 是否与当前运行版本一致

### 9.3 本地受管 root skill manifest

路径：

```text
~/.agents/skills/awiki-cli/.awiki-managed.json
```

示例：

```json
{
  "schema_version": 1,
  "type": "root-skill",
  "skill_name": "awiki-cli",
  "cli_version": "1.8.1",
  "bundle_version": "2026.04.17",
  "root_skill_sha256": "673ac2...",
  "installed_at": "2026-04-17T08:35:12Z",
  "managed_paths": ["SKILL.md"],
  "adopted": false
}
```

规则：

- 有该文件且当前 `SKILL.md` 哈希匹配 `root_skill_sha256`：可自动覆盖
- 有该文件但当前 `SKILL.md` 已被用户修改：视为冲突
- 没有该文件：视为非受管 skill，不自动覆盖

---

## 10. 命令定义

### 10.1 升级命令

#### `awiki-cli upgrade`

默认执行“检查并展示当前决策”。

输出至少包含：

- `current_version`
- `latest_version`
- `min_supported_version`
- `artifact_available`
- `auto_upgrade_enabled`
- `auto_upgrade_mode`
- `pending_update`
- `last_ws_upgrade_event_at`

#### `awiki-cli upgrade check`

显式请求更新检查 API，刷新本地决策缓存。

#### `awiki-cli upgrade apply`

执行：

- 下载
- 校验
- 解压
- 切换当前 binary
- 同步 root skill
- 更新状态文件

#### `awiki-cli upgrade status`

查看当前版本状态、最近检查来源、最近 websocket 升级事件、最近失败信息、是否存在 pending update。

#### `awiki-cli upgrade config show`

输出当前解析后的升级配置。

#### `awiki-cli upgrade config set`

允许更新：

- `channel`
- `auto_upgrade_enabled`
- `auto_upgrade_mode`
- `allow_ws_push_trigger`
- `disable_strict_version`
- `metadata_cache_ttl_seconds`

### 10.2 skill 命令

#### `awiki-cli skill index`

输出唯一公开 root skill 的元数据和 deeper docs 索引。

#### `awiki-cli skill get <path>`

按 path 读取 bundle 中的 deeper docs 节点；默认输出 markdown，也可输出 JSON 或写入文件。

#### `awiki-cli skill sync`

同步公开 root skill：

```text
<dir>/awiki-cli/SKILL.md
<dir>/awiki-cli/.awiki-managed.json
```

#### `awiki-cli skill export`

导出离线 skill 包；默认从当前 CLI 的运行时 bundle 导出，而不是从另一套内部文档树导出。

#### `awiki-cli schema <resource>.<method>`

skill 文档只描述“何时用、怎么用、风险是什么、如何验证”；具体字段真相始终以 `schema` 命令输出为准。

---

## 11. Skill 单一真相源

### 11.1 真相源定义

本方案明确采用以下分层：

#### 人工维护真相源

```text
skills/
  SKILL.md
  README.md
  manifests/skills.yaml
  references/*.md
```

#### 运行时消费产物

```text
internal/skillbundle/
```

`internal/skillbundle` 可以由以下任一方式生成：

- `go:embed`
- 代码生成
- 构建期打包

但它都不是人工直接维护的编辑入口。

### 11.2 为什么必须合并为一套真相源

如果同时保留：

- 一套 `skills/` 手工文档
- 一套 `internal/skill/**` 手工文档

将会带来：

- 文档漂移
- 审阅成本翻倍
- root skill 与 deeper docs 版本不一致
- `cmdmeta`、skill 索引与实际文档不一致

因此本方案要求：

- 只人工维护 `skills/`
- 运行时 bundle 只能构建自 `skills/`
- 构建或测试阶段必须校验 `skills/` 与 bundle manifest 的一致性

### 11.3 root skill 名称

目标公开 skill 名称统一为：

```text
awiki-cli
```

说明：

- 当前仓库里若仍存在旧 `awiki` frontmatter 或宿主兼容配置，应视为迁移中状态
- 本方案不把 `awiki` 继续作为长期正式公开名
- 如需兼容旧名，应通过 alias/迁移规则处理，而不是双真相源长期并存

---

## 12. 渲染、读取与缓存规则

### 12.1 root skill 渲染顺序

1. 读取当前运行 CLI 对应的 skill bundle manifest
2. 读取 root skill 真相源模板（来自 `skills/SKILL.md` 的构建产物）
3. 注入：
   - CLI 版本
   - bundle 版本
   - `awiki-cli skill index`
   - `awiki-cli skill get <path>`
   - `awiki-cli schema <resource>.<method>`
   - 编译期 host profile 的补充 metadata
   - bootstrap 安装说明（当 `awiki-cli` 不存在时，先通过 npm 安装 `@awiki/cli`）
   - 固定公开 URL metadata（`https://awiki.ai/skills/awiki-cli/SKILL.md`）
4. 生成 root `SKILL.md`
5. 计算哈希
6. 同步到公开 skill 目录，并发布到公开 URL

### 12.2 root skill bootstrap 内容

root `SKILL.md` 必须同时满足“已安装运行态”和“未安装 bootstrap 态”两种使用方式，因此内容中必须明确包含：

1. `awiki-cli` 是否已安装的检查说明
2. 若未安装，先执行 npm bootstrap 安装，例如：

   ```bash
   npm install -g @awiki/cli
   ```

3. 安装完成后，先验证 `awiki-cli version` 或等价命令可运行
4. 然后再按 root skill 协议继续使用：
   - `awiki-cli skill index --json`
   - `awiki-cli skill get <path>`
   - `awiki-cli schema <resource>.<method>`

约束：

- 公开 URL 下载版 root skill 与本地同步版 root skill 必须来自同一渲染结果
- root skill 必须能在本地尚未安装 `awiki-cli` 的情况下单独成立，不能假设 Agent 已经具备 CLI
- root skill 中的安装说明只承担 bootstrap 角色；一旦 CLI 安装完成，后续升级仍由 `awiki-cli` 自己管理

### 12.3 deeper docs 读取顺序

`awiki-cli skill get <path>` 的运行顺序：

1. 读取当前运行 CLI 对应的 bundle manifest
2. 校验 `<path>` 在索引中存在
3. 读取 bundle 中对应文档内容
4. 按需注入版本和 metadata
5. 计算输出哈希
6. 写入版本化缓存
7. 返回 stdout 或目标文件

### 12.4 缓存规则

缓存 key：

```text
<cli-version>/<doc-path>
```

示例：

```text
~/.awiki-cli/cache/skill/1.8.1/SKILL.md
~/.awiki-cli/cache/skill/1.8.1/references/03-messaging.md
```

缓存策略：

- `auto`：优先读缓存；若 bundle hash 不匹配则失效
- `refresh`：强制重渲染并更新缓存
- `bypass`：不读写缓存

---

## 13. 自动升级事务与失败边界

### 13.1 标准事务顺序

无论升级由显式命令触发还是 websocket 通知触发，标准顺序固定为：

1. 获取更新决策（API 或 ws 推送）
2. 下载 artifact 到 staging
3. 校验 `sha256` 和可选签名
4. 解压到 `~/.awiki-cli/versions/<version>/`
5. 原子切换 `~/.awiki-cli/bin/awiki-cli`
6. 同步 root skill
7. 更新 `release-state.json` 与 `skill-state.json`

### 13.2 失败边界

#### 切换前失败

如果在下载、校验、解压阶段失败：

- 当前版本保持不变
- `last_failed_update` 记录失败版本与阶段
- 不改动 root skill

#### 切换后 root skill 失败

如果 binary 已切换成功，但 root skill 同步失败：

- 不回滚 binary
- `skill-state.json` 标记 `warning` 或 `failed`
- CLI 输出 warning
- 后续允许通过 `awiki-cli skill sync` 单独修复

#### 状态落盘失败

如果二进制切换和 skill 同步都完成，但状态文件写入失败：

- 新版本继续生效
- 必须在下一次 `upgrade status` 或 `doctor` 中暴露“不完整升级”信号

---

## 14. root skill 冲突处理

执行 `awiki-cli skill sync` 时的规则：

- 目标不存在：创建并写入
- 目标存在且是受管文件、且未被修改：自动覆盖
- 目标存在且是受管文件、但已被用户修改：冲突，默认不覆盖
- 目标存在但不是受管文件：默认不覆盖
- `--adopt`：接管已有文件，先备份后写入
- `--force`：强制覆盖，但仍先备份

建议备份路径：

```text
<dir>/awiki-cli/.backup/SKILL.md.<timestamp>.bak
```

---

## 15. 配置建议

建议在 `config.yaml` 中增加或统一以下 `update` 配置：

```yaml
update:
  channel: stable
  disable_strict_version: false
  metadata_cache_ttl_seconds: 3600
  auto_upgrade_enabled: true
  auto_upgrade_mode: apply
  allow_ws_push_trigger: true
```

字段语义：

- `channel`：升级通道
- `disable_strict_version`：关闭最小支持版本硬拦截，仅用于调试或紧急兜底
- `metadata_cache_ttl_seconds`：检查 API 结果缓存 TTL
- `auto_upgrade_enabled`：是否允许自动升级
- `auto_upgrade_mode`：`apply | predownload | notify`
- `allow_ws_push_trigger`：是否允许 websocket 推送触发升级流程

默认值建议：

- `auto_upgrade_enabled: true`
- `auto_upgrade_mode: apply`
- `allow_ws_push_trigger: true`

---

## 16. 推荐默认策略

建议直接冻结以下默认值：

```text
公开 skill 名称: awiki-cli
公开 skill 路径: ~/.agents/skills/awiki-cli/SKILL.md
公开 root skill URL: https://awiki.ai/skills/awiki-cli/SKILL.md

skill 编辑真相源:
  skills/SKILL.md
  skills/references/*.md
  skills/manifests/skills.yaml

运行期版本真相源:
  awiki 服务端更新 API

HTTP 更新检查:
  有 active DID 时带 current_did
  有可用凭证时默认带身份认证
  无凭证时退化为匿名请求

root skill bootstrap:
  Agent 可先通过公开 URL 下载 root SKILL.md
  若 awiki-cli 未安装，先执行 npm install -g @awiki/cli
  再通过 awiki-cli skill index/get 读取 deeper docs

websocket 建联上报:
  CLI version
  channel
  host agent
  host version
  host capabilities
  skill format version

服务端响应:
  当前不返回 host-specific 内容

默认自动升级:
  enabled = true
  mode = apply
  allow_ws_push_trigger = true

自动同步范围:
  仅公开 root SKILL.md

deeper docs:
  默认不预装到 ~/.agents/skills/
  通过 awiki-cli skill get 动态读取
  通过 awiki-cli skill export 离线导出
```

---

## 17. 最小实现顺序

### 第一阶段：更新源与安装模型收口

1. 把 `internal/update` 改为服务端更新 API 客户端
2. 实现 `release-state.json` 新格式
3. 实现 `awiki-cli upgrade check / status / apply`
4. 实现 workspace 版本目录与当前 binary 指针切换

这一阶段完成后，CLI 已不再依赖 npm 作为正式运行期升级路径。

### 第二阶段：skill 单一真相源与 root skill 同步

5. 定义从 `skills/` 构建运行时 bundle 的流程
6. 实现 root skill 渲染
7. 实现 `awiki-cli skill sync`
8. 实现 `skill-state.json`

这一阶段完成后，root skill 可与 binary 版本自动同步。

### 第三阶段：动态文档与缓存

9. 实现 `awiki-cli skill index`
10. 实现 `awiki-cli skill get <path>`
11. 实现缓存与路径校验
12. 实现 `awiki-cli skill export`

这一阶段完成后，deeper docs 的动态索引、读取、导出能力完整落地。

### 第四阶段：websocket 主动升级

13. websocket 握手上报 CLI/host 元数据
14. 监听 `system.upgrade_available` / `system.upgrade_required`
15. 默认自动 apply
16. 增加配置降级到 `predownload` / `notify`

---

## 18. 当前版本未做的事

本设计当前**没有要求**：

- 服务端按 host 返回不同的 root skill 文本
- 服务端按 host 返回不同的 deeper docs 索引
- CLI 命令面因 host 改变
- deeper docs 自动同步到 `~/.agents/skills/awiki-cli/`
- 完整定义 artifact 签名体系的公钥分发方案

这些都可以作为后续版本扩展，但不属于当前正式落地范围。
