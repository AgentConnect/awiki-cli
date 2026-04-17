# awiki-cli 自升级落地实施规划

**文档状态**：Draft v1.0  
**文档用途**：把 `docs/self-update/self-update-architecture.md` 转成可执行、可拆解、可验收的工程实施计划。  
**适用范围**：`awiki-cli` 更新检查、CLI 自管升级、websocket 主动升级通知、root skill 同步、URL bootstrap root skill、skill bundle 单一真相源、发布与回滚收口。  
**最后更新**：2026-04-17

> 当本文与 `docs/self-update/self-update-architecture.md` 冲突时，以 architecture 文档为准。本文只回答“如何落地”，不重新讨论架构方向。

---

## 1. 目标与输入基线

本文档基于 `docs/self-update/self-update-architecture.md`，回答下面这个问题：

**如何把 awiki-cli 当前基于 npm registry + npm global install 的升级机制，迁移为服务端更新 API 驱动、CLI 自管 binary、websocket 可主动推送升级、并与单一 skill 真相源联动的正式系统。**

### 1.1 本次实施目标

本次实施需要同时达成以下目标：

1. 把运行期版本检查从 npm registry 迁移到 awiki 服务端更新 API。
2. 把正式升级执行路径从 `npm install -g` 迁移到 CLI 自己下载、校验、解压、切换二进制。
3. 在 websocket 模式下，让 listener 建联时上报 CLI 版本和 host 元数据，并接收服务端主动升级通知。
4. 把公开 root skill 的同步纳入升级事务，并且只公开一个 `awiki-cli` skill。
5. 让 root `SKILL.md` 同时支持固定 URL bootstrap 下载，并在本地尚未安装 `awiki-cli` 时先通过 npm 完成初始安装。
6. 把 `skills/` 明确为唯一人工维护真相源，构建出 CLI 运行期消费的 skill bundle。
7. 让 HTTP 更新检查在有 DID/凭证时默认带上 DID 与身份认证，在没有凭证时自动退化为匿名请求。
8. 让发布、升级、skill、配置、状态文件、回滚路径形成一套一致的系统。

### 1.2 输入文档与代码基线

| 类别 | 路径 | 用途 |
|---|---|---|
| 架构真相 | `docs/self-update/self-update-architecture.md` | 自升级与 skill 分发目标态 |
| 历史方案 | `docs/self-update/chatgpt-初始方案.md` | 历史设计输入，仅供参考 |
| 发布手册 | `docs/publish.md` | 当前发布/回滚主链路与 npm 叙事 |
| 当前更新实现 | `internal/update/update.go` | 当前 npm registry 检查逻辑 |
| 当前升级命令 | `internal/cli/upgrade.go` | 当前 `npm install -g` 升级逻辑 |
| websocket client | `internal/runtime/listener/wsclient.go` | 当前建联与通知消费路径 |
| 配置解析 | `internal/config/config.go` | update 配置字段与默认值 |
| build metadata | `internal/buildinfo/buildinfo.go` | 当前版本/构建信息入口 |
| 当前 skill 文档 | `skills/SKILL.md` | root skill 真相源 |
| 当前 skill 结构说明 | `skills/README.md` | 单入口 + references 的当前结构 |
| 命令契约真相 | `internal/cmdmeta/catalog.go` | CLI 命令/flag/implemented 状态真相 |

### 1.3 发生冲突时的优先级

实施时按以下优先级裁决：

1. `docs/self-update/self-update-architecture.md`
2. `docs/self-update/self-update-implementation-plan.md`
3. `internal/cmdmeta/catalog.go`
4. `skills/SKILL.md` 与 `skills/references/*.md`
5. `skills/manifests/skills.yaml`
6. `docs/publish.md`

说明：

- `internal/cmdmeta/catalog.go` 负责命令存在性、flag 与实现状态真相。
- `skills/` 负责 skill 内容真相。
- `docs/publish.md` 会在后续阶段被更新，当前不作为最终升级架构真相源。

---

## 2. 当前状态审计

本节作为实施起点，明确当前代码和文档已经是什么状态，避免规划假设错误。

### 2.1 已存在的能力

当前仓库已经具备以下可复用基础：

1. **版本信息入口**  
   `internal/buildinfo/buildinfo.go` 已经有 `Version / Commit / BuildDate / CGOEnabled`，可继续作为版本与构建注入入口。

2. **版本门禁入口**  
   `internal/cli/root.go` 中的 `maybeCheckForUpdates()` 已经是运行命令前的统一检查点，且已有一组 update-exempt commands。

3. **update 配置字段**  
   `internal/config/config.go` 已经有：
   - `update.disable_strict_version`
   - `update.metadata_cache_ttl_seconds`

4. **websocket listener 基础设施**  
   `internal/runtime/listener/wsclient.go` 已有：
   - WebSocket 握手 header 注入入口
   - 持续读通知通道
   - pending request / notification 分流
   - reader error 与 ping 基础逻辑

5. **当前 skill 文档制品**  
   当前 skill 体系已经实际落在：
   - `skills/SKILL.md`
   - `skills/references/*.md`
   - `skills/manifests/skills.yaml`

6. **workspace 单根目录模型**  
   当前 `internal/config` 已经采用 `~/.awiki-cli/` 单根目录模型，适合继续承载 versions / state / cache / upgrade staging。

### 2.2 当前存在的问题

当前状态与目标架构存在以下明确差距：

1. `internal/update/update.go` 仍通过 npm registry 获取 `latest_version` 与 `min_supported_version`。
2. `internal/cli/upgrade.go` 仍执行 `npm install -g @awiki/cli@latest`。
3. 当前没有 CLI 自己管理的 `versions/ + bin/current` 版本目录模型。
4. 当前 websocket 建联没有带 CLI 版本与 host 元数据。
5. 当前 websocket 通知类型中没有升级通知约定。
6. 当前没有 `release-state.json` 与 `skill-state.json` 的正式实现。
7. 当前没有从 `skills/` 构建运行时 skill bundle 的链路。
8. 当前 `skills/SKILL.md` 的 frontmatter 仍是 `name: awiki`，与目标公开 skill 名 `awiki-cli` 不一致。
9. `docs/publish.md` 当前仍把 npm 版本策略视为运行期升级真相源。
10. 当前没有固定的公开 root skill URL 作为 Agent bootstrap 下载入口。
11. 当前更新检查请求没有在有 DID/凭证时默认附带 `current_did` 与身份认证。

### 2.3 本次实施要解决的问题边界

本次实施**解决**：

- 运行期版本检查与升级执行迁移
- websocket 主动升级通知
- root skill 同步与 bundle 化
- 发布与回滚叙事收口

本次实施**不解决**：

- host-specific 服务端返回内容
- CLI 命令面分 host 分叉
- secure messaging 业务协议
- people 域能力实现

---

## 3. 冻结决策

本节是实施阶段不再摇摆的裁决，实施者不应在编码过程中重新发明这些策略。

### 3.1 升级执行模型冻结

- 正式升级模型固定为：**方案 1：CLI 自管 binary 版本目录**。
- npm 仅保留 bootstrap 安装角色，不再是运行期正式升级路径。
- `awiki-cli upgrade apply` 是正式升级执行入口。

### 3.2 运行期版本真相源冻结

- 运行期版本决策只来自 awiki 服务端更新 API。
- npm registry 不再作为运行期版本真相源。
- 缓存只作为服务端 API 的故障兜底，不允许长期与服务端状态分裂。
- HTTP 更新检查默认采用 best-effort auth：
  - 有 active DID 时带 `current_did`
  - 有可用凭证时默认带身份认证
  - 无凭证或鉴权失败时退化为匿名请求

### 3.3 skill 真相源冻结

- `skills/` 是唯一人工维护真相源。
- 不允许新增手工维护的 `internal/skill/**` 第二文档树。
- 运行时 skill bundle 只能由 `skills/` 构建而来。

### 3.4 root skill 命名与 bootstrap 冻结

- 对外唯一公开 skill 名称固定为：`awiki-cli`
- 对外目录固定为：

  ```text
  ~/.agents/skills/awiki-cli/
  ```

- 固定公开 bootstrap URL 为：

  ```text
  https://awiki.ai/skills/awiki-cli/SKILL.md
  ```

- root `SKILL.md` 必须可在 `awiki-cli` 尚未安装时单独成立，并明确指导 Agent 先通过 npm 执行 bootstrap 安装：

  ```bash
  npm install -g @awiki/cli
  ```

- 当前 `skills/SKILL.md` 中的 `name: awiki` 视为迁移遗留，实施过程中需要收敛到正式名。

### 3.5 websocket 升级策略冻结

- websocket listener 建联时必须上报 CLI 版本和 host 信息。
- 服务端首版可推送：
  - `system.upgrade_available`
  - `system.upgrade_required`
- 客户端默认行为固定为：**全自动升级**。
- 后续允许通过配置降级为 `predownload` 或 `notify`。

### 3.6 host 语义冻结

- 客户端请求会带：
  - `host_agent`
  - `host_version`
  - `host_capabilities`
  - `skill_format_version`
- 服务端首版响应**不返回 host-specific 内容**。
- host-specific 差异先内置到 binary 的编译配置中。

---

## 4. 目标工程结构

建议实施后的相关模块结构如下：

```text
internal/
  buildinfo/
  cli/
    root.go
    upgrade.go
  config/
  update/
  upgrader/
  skillbundle/
  runtime/
    listener/
skills/
docs/self-update/
```

### 4.1 模块职责

| 模块 | 职责 |
|---|---|
| `internal/update` | 服务端更新检查 API client、决策计算、metadata cache、版本比较 |
| `internal/upgrader` | artifact 下载、校验、解压、版本切换、状态文件更新 |
| `internal/cli/upgrade.go` | `upgrade check/apply/status/config` 命令入口与输出编排 |
| `internal/runtime/listener` | websocket 握手 header 注入、升级通知接收、自动升级触发 |
| `internal/skillbundle` | 从 `skills/` 构建得到的运行时 bundle、index/get/export/sync 支撑 |
| `internal/buildinfo` | 当前运行版本、构建信息、编译期 host profile |
| `internal/config` | `update.*` 配置解析、默认值、workspace 目录派生 |

### 4.2 本地目录目标态

实施后，workspace 需要支持：

```text
~/.awiki-cli/
├─ versions/
├─ bin/
├─ state/
│  ├─ release-state.json
│  └─ skill-state.json
├─ cache/
│  └─ skill/
├─ tmp/
└─ upgrade/
   ├─ locks/
   └─ staging/
```

说明：

- `versions/`：保存已安装版本
- `bin/`：指向当前激活版本
- `state/`：保存升级与 skill 同步状态
- `cache/skill/`：保存按 CLI 版本缓存的动态文档
- `upgrade/staging/`：升级过程中的下载与解压中间态

---

## 5. 实施总里程碑

本次实施拆成 5 个阶段，每阶段必须具备独立验收标准。

| Phase | 名称 | 目标 |
|---|---|---|
| Phase 0 | 模型冻结与接口校准 | 冻结字段、命名、状态文件、迁移策略 |
| Phase 1 | 更新检查 API 接入 | 用服务端 API 替换 npm registry 检查 |
| Phase 2 | CLI 自管升级器 | 下载、校验、解压、切换、状态落盘 |
| Phase 3 | Skill bundle 与 root skill 同步 | `skills/` 单一真相源、bundle 化、root skill 自动同步 |
| Phase 4 | websocket 主动升级通知 | 建联上报元数据、接收升级事件、默认自动升级 |
| Phase 5 | 发布与运维收口 | 发布脚本、install.js、publish.md、回滚与兼容收口 |

执行顺序固定为 Phase 0 → 1 → 2 → 3 → 4 → 5。

---

## 6. Phase 0：模型冻结与接口校准

### 6.1 目标

在编码前冻结所有高影响接口与命名，避免实现中重复改 schema、路径或状态文件。

### 6.2 任务清单

1. 冻结更新检查 API 请求字段：
   - `schema_version`
   - `current_version`
   - `current_did`
   - `channel`
   - `goos`
   - `goarch`
   - `host_agent`
   - `host_version`
   - `host_capabilities`
   - `skill_format_version`

2. 冻结更新检查 API 响应字段：
   - `latest_version`
   - `min_supported_version`
   - `channel`
   - `published_at`
   - `artifact`
   - `skill_bundle`

3. 冻结本地状态文件结构：
   - `release-state.json`
   - `skill-state.json`
   - `.awiki-managed.json`

4. 冻结更新检查请求的鉴权语义：
   - 有 DID 时带 `current_did`
   - 有可用凭证时默认带认证
   - 无凭证或鉴权失败时允许匿名回退

5. 冻结 root skill 迁移与 bootstrap 策略：
   - 公开名 `awiki-cli`
   - 公开 URL `https://awiki.ai/skills/awiki-cli/SKILL.md`
   - root skill 必须包含 npm bootstrap 安装说明
   - 旧 `awiki` 视为迁移兼容项

6. 冻结 artifact 命名约定与下载 URL 结构，供客户端和发布系统共享。

### 6.3 代码与文档改造点

- `docs/self-update/self-update-architecture.md`
- `docs/self-update/self-update-implementation-plan.md`
- `docs/publish.md`（先记录待更新项，不必在本 phase 一次性改完）

### 6.4 输出物

- 冻结后的接口示例
- 冻结后的状态文件示例
- root skill 迁移与 bootstrap 说明
- update request auth 语义说明
- artifact naming 约定

### 6.5 完成标准

- 实现者不再需要猜测字段名、状态文件结构、skill 名称或版本目录目标
- 所有后续阶段都可以直接引用本文档中的 frozen schema

---

## 7. Phase 1：服务端更新检查 API 接入

### 7.1 目标

用 awiki 服务端更新 API 替换当前 npm registry 版本检查逻辑，并保留现有版本阻断与告警语义。

### 7.2 代码改造点

1. `internal/update/update.go`
   - 删除 `npmLatestURL` 依赖
   - 新增服务端检查 API client
   - 在有 active identity DID 时附带 `current_did`
   - 默认尝试携带身份认证信息；无凭证或鉴权失败时退化为匿名请求
   - 解析新的 `artifact` 和 `skill_bundle` 字段
   - 继续支持 metadata cache

2. `internal/cli/root.go`
   - `maybeCheckForUpdates()` 改为基于新 `Decision`
   - 保持现有 exempt commands 逻辑
   - 更新 hint/warning 文案，不再引用 npm 作为正式升级路径

3. `internal/config/config.go`
   - 保留已有：
     - `update.disable_strict_version`
     - `update.metadata_cache_ttl_seconds`
   - 新增后续 phase 需要的配置字段占位：
     - `update.channel`
     - `update.auto_upgrade_enabled`
     - `update.auto_upgrade_mode`
     - `update.allow_ws_push_trigger`

4. `config.template.yaml`
   - 加入 update 配置组与默认值

### 7.3 类型与数据结构调整

`internal/update` 中的 `Metadata` 和 `Decision` 需要扩展为至少支持：

- `artifact_available`
- `artifact_url`
- `artifact_sha256`
- `skill_bundle_version`
- `skill_bundle_sha256`
- `root_skill_sha256`
- `source`
- `blocked`
- `has_newer_version`
- `pending_update`

### 7.4 缓存策略

- 继续保留本地 metadata cache
- 缓存文件位置仍放在 `cache/update/` 下
- 网络失败时允许回退缓存
- 缓存命中时需在 `source` 中明确标注：
  - `network`
  - `cache`
  - `cache_stale`

### 7.5 服务端依赖

服务端需提供：

- 可用的更新检查 API
- 按 `goos/goarch` 匹配 artifact 的能力
- 对 `host_agent / host_version / host_capabilities / skill_format_version` 的透传和容错
- 同时兼容两类请求：
  - 带 `current_did` 且带身份认证的请求
  - 不带认证、也不带 `current_did` 的匿名请求

### 7.6 验收标准

- 普通命令运行前可基于服务端 `min_supported_version` 做阻断
- `version`、`doctor`、`schema`、`upgrade` 等豁免命令不受阻断影响
- 有 active DID 和可用凭证时，请求会带 `current_did` 与身份认证
- 无凭证时会自动退化为匿名请求，而不是直接失败
- 网络失败时可使用缓存继续工作
- 告警和错误输出中不再把 `npm install -g` 写成唯一正式升级路径

---

## 8. Phase 2：CLI 自管升级器

### 8.1 目标

把升级执行从 npm global install 切换为 CLI 自己完成下载、校验、解压、切换和状态落盘。

### 8.2 新增模块

新增：

```text
internal/upgrader/
```

建议职责拆分：

- `artifact_fetcher.go`
- `artifact_verify.go`
- `artifact_extract.go`
- `install_paths.go`
- `switch_current.go`
- `release_state.go`
- `lock.go`

说明：文件名可调整，但职责边界不要合并成单一 giant file。

### 8.3 代码改造点

1. `internal/cli/upgrade.go`
   - 重写 `upgrade` 逻辑
   - 默认 `awiki-cli upgrade` 只做检查输出
   - 增加：
     - `upgrade check`
     - `upgrade apply`
     - `upgrade status`
     - `upgrade config show`
     - `upgrade config set`

2. `internal/config/config.go`
   - 派生 versions/bin/state/upgrade staging 目录

3. `internal/buildinfo`
   - 如需要，补充当前运行 binary path / host profile 的导出接口

### 8.4 安装事务顺序

固定为：

1. 获取更新决策
2. 下载 artifact 到 `upgrade/staging/`
3. 校验 sha256
4. 校验可选 signature（首版可先保留接口，后续再启用强校验）
5. 解压到 `versions/<version>/`
6. 原子切换 `bin/awiki-cli`
7. 写入 `release-state.json`

### 8.5 并发与锁

必须实现单实例升级锁，防止：

- 用户手动执行 `upgrade apply`
- websocket 收到升级通知自动 apply
- 两者并发执行

建议锁文件放在：

```text
~/.awiki-cli/upgrade/locks/self-update.lock
```

### 8.6 状态文件要求

`release-state.json` 至少记录：

- `current_version`
- `current_artifact_sha256`
- `channel`
- `last_checked_at`
- `last_check_source`
- `last_ws_upgrade_event_at`
- `pending_update`
- `last_applied_version`
- `last_failed_update`

### 8.7 失败边界

- 下载/校验/解压失败：不影响当前版本
- binary 切换失败：保留旧版本，记录失败原因
- 状态落盘失败：新 binary 可继续工作，但必须留下 repair signal

### 8.8 验收标准

- `awiki-cli upgrade apply` 不再调用 npm
- 新版本能安装到 `versions/<version>/`
- `bin/awiki-cli` 可切换到新版本
- 失败时旧版本仍可继续使用
- `upgrade status` 能看到最近一次失败/成功信息

---

## 9. Phase 3：Skill bundle 与 root skill 同步

### 9.1 目标

把 `skills/` 明确变成唯一人工维护真相源，并形成运行时 skill bundle 与 root skill 自动同步能力。

### 9.2 新增模块

新增：

```text
internal/skillbundle/
```

建议职责：

- 读取编译期 bundle manifest
- 提供 `skill index`
- 提供 `skill get <path>`
- 提供 `skill export`
- 提供 root skill 渲染
- 提供 root skill sync
- 管理 `skill-state.json`

### 9.3 真相源与构建策略

人工维护输入固定为：

- `skills/SKILL.md`
- `skills/references/*.md`
- `skills/manifests/skills.yaml`

运行时 bundle 必须由上面三类输入构建，不允许再人工维护第二套内部文档树。

构建方式可选：

- `go:embed`
- 代码生成
- 构建期打包

本 phase 不强制具体实现手段，但必须满足：

- 审阅入口仍是 `skills/`
- 运行时只消费一份 bundle 产物

### 9.4 root skill 命名迁移

当前 `skills/SKILL.md` frontmatter 是：

```yaml
name: awiki
```

本 phase 需要明确迁移到：

```yaml
name: awiki-cli
```

迁移策略固定为：

1. 运行时公开目录只认 `~/.agents/skills/awiki-cli/`
2. 如果检测到旧 `~/.agents/skills/awiki/`：
   - 若是受管目录且未被用户修改：备份后迁移/替换
   - 若是非受管目录：不自动改动，只输出 warning
3. 不长期保留两个公开 skill 目录并存

### 9.5 root skill bootstrap 内容

本 phase 需要把 root skill 渲染成同时适用于两类入口的内容：

1. 本地公开目录下的 skill 入口
2. 固定 URL 下载的 bootstrap skill 入口

root skill 内容必须明确包含：

- `awiki-cli` 是否已安装的检查说明
- 未安装时先通过 npm bootstrap 安装：`npm install -g @awiki/cli`
- 安装后先执行 `awiki-cli version` 验证
- 再执行：
  - `awiki-cli skill index --json`
  - `awiki-cli skill get <path>`
  - `awiki-cli schema <resource>.<method>`

并且需要保证：

- 本地同步版与公开 URL 版 root skill 使用同一渲染产物
- 不允许长期分叉成两套不同 root skill 文本

### 9.6 新增命令

新增或补齐：

- `awiki-cli skill index`
- `awiki-cli skill get <path>`
- `awiki-cli skill sync`
- `awiki-cli skill export`

### 9.7 root skill 同步规则

`skill sync` 需要支持：

- 目标不存在：创建
- 目标是受管文件且未修改：自动覆盖
- 目标是受管文件但已修改：冲突，不自动覆盖
- 目标不是受管文件：默认不覆盖
- `--adopt`：先备份，再接管
- `--force`：先备份，再覆盖

### 9.8 状态文件要求

`skill-state.json` 至少记录：

- `current_cli_version`
- `current_bundle_version`
- `current_bundle_sha256`
- `root_skill_sync.dir`
- `root_skill_sync.root_skill_sha256`
- `root_skill_sync.last_synced_at`
- `root_skill_sync.last_sync_status`
- `cache_policy`

### 9.9 构建与测试约束

必须新增 bundle 一致性校验：

- `skills/SKILL.md` 与 root bundle 描述一致
- `skills/references/*.md` 与 index manifest 一致
- 文档引用的命令面与 `internal/cmdmeta/catalog.go` 一致

### 9.10 验收标准

- 当前 CLI 能生成并同步 `~/.agents/skills/awiki-cli/SKILL.md`
- 公开 URL `https://awiki.ai/skills/awiki-cli/SKILL.md` 可提供同一版 root skill bootstrap 文本
- root skill 内容中包含 npm bootstrap 安装说明和后续 `skill index/get` 协议
- `skill get` 能正确读取 references 文档
- 用户修改过的 root skill 不会被静默覆盖
- `skills/` 成为唯一审阅与维护入口

---

## 10. Phase 4：websocket 主动升级通知

### 10.1 目标

让 websocket listener 在建联时上报版本/host 元数据，并在收到服务端升级通知后按配置自动处理升级。

### 10.2 代码改造点

1. `internal/runtime/listener/wsclient.go`
   - 在握手 header 注入：
     - `X-Awiki-CLI-Version`
     - `X-Awiki-CLI-Channel`
     - `X-Awiki-Host-Agent`
     - `X-Awiki-Host-Version`
     - `X-Awiki-Host-Capabilities`
     - `X-Awiki-Skill-Format-Version`

2. `internal/buildinfo`
   - 提供 host profile 信息读取入口

3. `internal/runtime/listener/server.go` 或等价协调层
   - 识别升级通知
   - 触发 updater/upgrader
   - 写入 `last_ws_upgrade_event_at`

4. `internal/update` / `internal/upgrader`
   - 支持以 `ws_push` 作为决策来源触发 apply / predownload / notify

### 10.3 服务端事件约定

首版支持：

- `system.upgrade_available`
- `system.upgrade_required`

事件 params 沿用更新检查 API 的核心字段：

- `latest_version`
- `min_supported_version`
- `channel`
- `artifact`
- `skill_bundle`

### 10.4 默认行为

默认配置：

```yaml
update:
  auto_upgrade_enabled: true
  auto_upgrade_mode: apply
  allow_ws_push_trigger: true
```

因此默认行为固定为：

- 收到升级通知后自动下载
- 自动校验
- 自动切换版本
- 自动同步 root skill

### 10.5 降级行为

后续允许配置为：

- `predownload`
- `notify`

语义固定为：

- `predownload`：下载+校验成功后写入 `pending_update`
- `notify`：不下载，只写入 `pending_update`

### 10.6 并发与幂等

必须处理以下情况：

- 同一版本升级通知重复下发
- 手动 `upgrade apply` 与 ws 自动升级并发
- listener 断线重连后再次收到相同事件

规则：

- 同版本升级事件必须幂等
- 以升级锁为唯一并发仲裁点
- 已应用版本再次收到相同版本通知，只刷新观测时间，不再重复执行 install

### 10.7 验收标准

- websocket 握手 header 带上版本和 host 信息
- 收到升级通知后能按默认策略自动升级
- 重复通知不会重复安装
- 配置关闭自动升级后可降级为 predownload / notify
- 正常消息通知主链路不受影响

---

## 11. Phase 5：发布、安装与运维收口

### 11.1 目标

让发布系统、客户端升级逻辑、bootstrap installer 与文档叙事全部一致。

### 11.2 代码与脚本改造点

1. `scripts/install.js`
   - 当前根据 `package.json.version` 拼下载 URL
   - 需要改为：先调用服务端更新 API，再下载匹配 artifact

2. root skill 静态发布
   - 需要把 root `SKILL.md` 发布到固定 URL：`https://awiki.ai/skills/awiki-cli/SKILL.md`
   - 该 URL 对应的内容必须来自正式发布版本的 root skill 渲染结果

3. `scripts/run.js`
   - 保持 thin wrapper 角色即可，但错误提示应改成与 CLI 自升级路径一致

4. `docs/publish.md`

   - 更新“运行期版本策略”叙事
   - 区分：
     - npm bootstrap 发布
     - artifact 发布
     - 服务端更新 API 元数据发布

5. 发布流程本身
   - 需要明确 artifact 上传后，服务端更新 API 何时可见新版本
   - 需要明确撤回 bad version 时，如何同步调整 `min_supported_version`

### 11.3 运维要求

必须有明确流程处理：

- bad version 撤回
- 某平台 artifact 缺失
- 服务端误推升级事件
- `min_supported_version` 提升导致老版本阻断

### 11.4 完成标准

- bootstrap install 与 CLI runtime upgrade 使用同一套 artifact 分发事实
- 公开 root skill URL 能发布与回滚到正确版本
- `docs/publish.md` 不再把 npm 视为运行期升级真相源
- 回滚流程能同时覆盖 artifact、更新 API、最低支持版本策略和公开 root skill URL

---

## 12. 跨阶段工程约束

### 12.1 命令兼容约束

不得破坏以下现有豁免命令的可用性：

- `awiki-cli version`
- `awiki-cli doctor`
- `awiki-cli schema`
- `awiki-cli config show`
- `awiki-cli upgrade`
- `awiki-cli runtime listener run`
- `awiki-cli runtime listener service-run`

### 12.2 输出契约约束

所有新命令输出仍必须走现有 JSON envelope 体系，不得引入裸 JSON 或非 envelope 结构作为 canonical 输出。

### 12.3 安全约束

- 自动升级下载必须有至少 `sha256` 校验
- signature 校验即使首版不强制，也要在结构上预留
- 不允许 websocket 通知里直接携带 shell 指令或脚本执行内容
- 不允许 host-specific 服务端响应作为首版必需条件

### 12.4 迁移约束

- 自动升级逻辑不能破坏现有 workspace
- root skill 迁移不能静默覆盖用户手改的文件
- 旧 `awiki` skill 目录只做 best-effort 迁移或告警，不做无提示强删

---

## 13. 测试计划

### 13.1 单元测试

至少覆盖：

1. 更新检查 API 响应解析
2. semver 与 `min_supported_version` 判断
3. metadata cache 读写与 source 标记
4. 更新检查请求在有 DID/凭证时的 `current_did` 与 auth 注入
5. 更新检查请求在无凭证时的匿名回退
6. artifact 下载器与 sha256 校验
7. release-state / skill-state 读写
8. skill bundle index/path 校验
9. root skill bootstrap 内容渲染
10. root skill managed/unmanaged/conflict 判定
11. websocket 升级通知解析
12. 升级锁与幂等逻辑

### 13.2 集成测试

至少覆盖：

1. 正常检查更新
2. 有 DID/凭证时的认证更新检查
3. 无凭证时的匿名更新检查
4. 当前版本被最小支持版本阻断
5. 手动 `upgrade apply`
6. `bin/awiki-cli` 指向切换
7. root skill 自动同步
8. 公开 URL root skill 可下载且内容正确
9. websocket 收到升级通知自动 apply
10. 关闭自动升级后转为 `pending_update`
11. 旧 `awiki` 目录迁移或冲突告警

### 13.3 失败场景测试

至少覆盖：

1. 更新 API 网络失败 + 缓存回退
2. 更新检查鉴权构造失败后匿名回退
3. 当前平台无 artifact
4. sha256 校验失败
5. 解压失败
6. binary 切换失败
7. root skill 已被用户修改
8. 公开 root skill URL 发布缺失或版本不匹配
9. 状态文件写入失败
10. websocket 重复收到相同升级事件
11. 手动 apply 与 ws 自动升级并发

### 13.4 验收 checklist

发布前必须逐项验证：

- [ ] 运行期版本检查已不依赖 npm registry
- [ ] `upgrade apply` 已不调用 `npm install -g`
- [ ] 当前 binary 安装在 `versions/<version>/` 下
- [ ] `bin/awiki-cli` 可以切换到新版本
- [ ] `release-state.json` 与 `skill-state.json` 可正确记录状态
- [ ] 有 DID/凭证时 HTTP 更新检查会带 `current_did` 与身份认证
- [ ] 无凭证时 HTTP 更新检查可匿名回退
- [ ] websocket 握手能带版本与 host 信息
- [ ] websocket 升级通知可自动触发升级
- [ ] root skill 只同步到 `~/.agents/skills/awiki-cli/`
- [ ] 公开 URL `https://awiki.ai/skills/awiki-cli/SKILL.md` 可下载正确的 bootstrap root skill
- [ ] root skill 中包含 npm bootstrap 安装说明
- [ ] `skills/` 是唯一人工维护真相源
- [ ] `docs/publish.md` 已更新到新叙事

---

## 14. 兼容、迁移与回滚策略

### 14.1 旧 npm 安装兼容

迁移期内，仍允许用户通过 npm 安装 bootstrap 包，但运行期一旦执行新版本 `awiki-cli upgrade apply`，后续版本切换应优先走 CLI 自管版本目录。

### 14.2 旧 skill 名称兼容

- 正式公开名固定为 `awiki-cli`
- 若存在旧 `awiki` 目录：
  - 受管且未修改：迁移
  - 非受管或已修改：只告警

### 14.3 服务端未就绪兼容

- 如果更新 API 尚未部署，客户端允许继续使用缓存兜底
- 如果认证更新检查不可用，客户端必须允许匿名更新检查继续工作
- 但发布前不能把“长期无服务端更新 API”作为正式状态接受

### 14.4 自动升级失败兼容

- 自动升级失败不应影响当前正在运行的进程继续服务
- 新版本切换未完成前不影响旧版本可用性
- skill sync 失败不回滚 binary，但必须显式记录 warning

### 14.5 回滚策略

回滚分四层：

1. **artifact 层**：撤回 GitHub Release / 下载源中的坏版本产物
2. **更新 API 层**：停止把坏版本标为 latest
3. **最低支持版本层**：必要时提高 `min_supported_version`，阻断坏版本继续运行
4. **root skill URL 层**：把公开 URL 的 root skill 回滚到与当前正式版本一致的文本

客户端在下次检查更新或收到 ws 推送后，应能收敛到新的服务端策略。

---

## 15. 建议的 Issue / 子任务拆分

建议按以下粒度拆实现任务：

1. 定义更新检查 API client 与 response model
2. 定义更新检查请求的 `current_did` 与 best-effort auth 语义
3. 重写 `internal/update` 决策逻辑
4. 重写 `maybeCheckForUpdates()` 提示与阻断逻辑
5. 新增 `internal/upgrader` 下载/校验/解压模块
6. 实现 `release-state.json`
7. 重写 `internal/cli/upgrade.go`
8. 设计 `skills/` → bundle 构建链路
9. 实现 `skill index/get/sync/export`
10. 实现 `skill-state.json`
11. root skill `awiki` → `awiki-cli` 迁移策略
12. root skill bootstrap 内容与公开 URL 发布
13. websocket 握手 header 注入
14. websocket 升级通知处理与自动 apply
15. `config.yaml` update 配置收口
16. `scripts/install.js` 改造
17. `docs/publish.md` 更新
18. 完整单元测试 + 集成测试 + failure tests

每个 issue 必须包含：

- 明确影响模块
- 输入/输出契约
- 是否涉及状态文件变更
- 是否需要服务端配合
- 验收条件

---

## 16. 实施完成定义（Definition of Done）

只有同时满足以下条件，才能认为本次自升级系统落地完成：

1. CLI 启动前的版本判断已全部切到服务端更新 API。
2. 正式升级执行已全部切到 CLI 自管 binary 版本目录。
3. websocket listener 建联时会上报 CLI 版本与 host 信息。
4. websocket 支持服务端主动升级通知，默认自动 apply。
5. root skill 会同步到 `~/.agents/skills/awiki-cli/`，并可通过 `https://awiki.ai/skills/awiki-cli/SKILL.md` 下载。
6. root skill 在 `awiki-cli` 未安装时也能指导 Agent 先通过 npm bootstrap 安装。
7. 更新检查在有 DID/凭证时会默认带 `current_did` 与身份认证，无凭证时会匿名回退。
8. `skills/` 已成为唯一人工维护真相源，运行时 bundle 能稳定构建和读取。
9. 发布、安装、升级、回滚文档已统一到新叙事。
10. 所有关键失败场景都有测试覆盖和明确定义的回退行为。

---

## 17. 本文档之后的推荐动作

完成本文件后，建议按以下顺序推进后续工作：

1. 先补服务端更新 API 的接口定义与 mock server
2. 再实现 `internal/update` 替换 npm registry
3. 然后实现 `internal/upgrader` 与 `upgrade apply`
4. 接着落 skill bundle 与 root skill sync
5. 最后接 websocket 主动升级通知与发布链路收口

这个顺序的原因是：

- 先打通版本决策真相源
- 再打通安装器
- 再把 skill 同步接到升级事务里
- 最后用 websocket 做自动化增强

这样可以保证每个阶段都有可运行、可验收的中间状态。
