# awiki-cli/

> L2 文档 | 父级: [../CLAUDE.md](../CLAUDE.md) | 分形协议: 三层结构

1. **地位**: `awiki-cli` 是 awiki 的命令行客户端项目，负责把命令行输入编排成对后端服务的 API 调用，并为 AI/人类用户提供统一 CLI 产品面。
2. **边界**: 输入是 CLI 命令、参数、配置和认证信息；输出是对同级后端服务的请求以及面向终端用户的命令执行结果。本仓库只承担客户端侧命令编排与交互，不承载服务端业务真相。
3. **约束**:
   - 后端服务依赖同级的 `../user-service/` 和 `../message-service/`。
   - 消息相关 API 文档位于 `../message-service/docs/api/`。
   - 用户相关 API 文档位于 `../user-service/docs/api/`。
   - 本项目是一个重写项目，重写参考实现位于同级 `../awiki-agent-id-message/`。
   - CLI 交互方式与工程组织可以参考同级飞书 CLI 仓库 `../cli/`。
   - v2 当前实现语言是 **Go 1.22**，并且要求保持 **pure Go / no CGO**。
   - 若需要做系统兼容性壳层，可放在 TypeScript/Node 的薄壳中，不在 Go 核心里引入 CGO。
   - 对外身份标识统一使用 **handle**；`did` 只在协议级定位需要时出现；`user_id` 仅允许作为内部实现字段存在，不得出现在公共 CLI 参数、help、schema、docs 示例或结构化输出中。
   - 如果命令实现涉及服务端 API 变化，需要同步更新对应服务仓库下的 API 文档。
   - 新增或修改本地测试、fixture、协议示例时，默认使用 `e1_...` 形态的 DID profile 后缀（例如 `e1_alice`、`e1_group`），不要使用裸 `e1`。

## 项目背景

- 项目名称：`awiki-cli`
- 项目形态：命令行客户端
- 当前阶段：
  - Phase 1：CLI 产品壳已实现
  - Phase 2：配置 / local identity / credential layout 已实现首版
  - Phase 3：User / handle lifecycle 与 user gating 已实现首版
  - Phase 4：SQLite 本地状态与迁移已实现首版
  - Phase 5：direct/group plain messaging 已实现首版，并已补齐 Base attachment 的单文件发送与下载首版（控制面 / 数据面固定走 HTTP）
  - Phase 7（部分提前落地）：websocket listener / local daemon 服务端已实现首版
  - Phase 7.1：ANP SDK 身份鉴权已接入 HTTP / websocket listener 主路径
- 通信模式：通过 CLI 命令调用 API 连接 awiki 服务端
- 主要服务端依赖：
  - `../user-service/`
  - `../message-service/`
- 关键文档入口：
  - 总体架构：`docs/architecture/awiki-v2-architecture.md`
  - 命令模型：`docs/architecture/awiki-command-v2.md`
  - 输出契约：`docs/architecture/output-format.md`
  - 总实施计划：`docs/plan/awiki-v2-implementation-plan.md`
  - Phase 0 冻结结果：`docs/plan/phase-0/`
- 重写参考：
  - Python 版本 CLI：`../awiki-agent-id-message/`
  - 飞书 CLI：`../cli/`
  - ANP Go SDK：依赖基线为远端发布版 `github.com/agent-network-protocol/anp/golang@v0.8.7`，P5 secure direct / OPK API 已随该版本发布，主线不得提交同级工作区 `replace`

## 成员清单

**README.md**: 仓库入口说明文件。
**config.template.yaml**: 标准用户主配置模板，展示当前 canonical `config.yaml` 字段与默认值；`service_base_url` 是平台服务入口，`did_domain` 可独立指定租户 DID provider domain。
**go.mod / go.sum**: Go 模块定义与依赖锁定；当前 Go 版本基线固定为 `1.22`，直接依赖 `cobra`、`gojq`、`modernc.org/sqlite` 与 ANP Go SDK `github.com/agent-network-protocol/anp/golang@v0.8.7`，要求 pure Go。P5 secure direct 通过远端发布版 SDK 消费 `OneTimePrekey`、`NewFileOneTimePrekeyStore` 与 top-level OPK publish/get API；主线不得提交 `replace => ../anp/anp/golang` 这类工作区本地依赖。上游 / 间接依赖树中可能仍出现 secp256k1 相关库，但 `awiki-cli` 当前本地 DID 主路径已统一为 `e1` / Ed25519。
**cmd/awiki-cli/main.go**: `awiki-cli` 主程序入口。
**internal/buildinfo/buildinfo.go**: 版本、构建时间、CGO 状态等构建信息。
**internal/cmdmeta/catalog.go**: 静态命令元数据目录，作为 schema/命令骨架的事实来源；测试守护 group E2EE 不发布 External Commit、cloud snapshot、multi-device、k1 recovery 等当前隐藏 P6 非目标命令面。
**internal/config/config.go**: 单根目录工作区路径解析（默认 `~/.awiki-cli/`）、仅支持 `AWIKI_CLI_WORKSPACE_HOME_DIR` 作为工作区环境变量，并统一解析 `config.yaml`；旧 `config.json` 由 workspace upgrade 在首次访问时自动迁移到 `config.yaml`，其余历史业务环境变量不再驱动 awiki-cli 行为；默认 `ANPMessageService` 从 `service_base_url` 推导而不是从 `did_domain` 推导。
**internal/output/output.go**: 统一 success/error JSON envelope、`--jq`、table/ndjson 渲染。
**internal/doctor/doctor.go**: 诊断实现，检查构建、配置、env、identity store、SQLite、legacy 路径、legacy DB 与 `anp-mls` binary/版本/状态目录；SQLite 检查会额外暴露 `contact_handle_bindings` 历史映射表状态与行数，MLS 检查会同时扫描 root 与 agent/device-scoped `state.db`/`state.lock`。
**internal/docs/topics.go**: CLI 内建 docs 主题索引，`skills` 主题引用当前 single-entry `skills/SKILL.md` 与懒加载 `skills/references/*.md` 拓扑。
**internal/anpsdk/registry.go**: ANP Go SDK 的远端模块依赖入口，统一暴露 DID WBA、HTTP Signatures、direct_e2ee 等后续 Phase 要用到的基础能力。
**internal/authsdk/session.go**: 基于 ANP SDK `DIDWbaAuthHeader` 的身份鉴权封装，负责 HTTP/WSS hop auth、401 重试、JWT token 捕获与持久化。
**internal/cli/app.go**: CLI 应用装配、配置解析与统一错误输出入口。
**internal/cli/root.go**: Cobra 根命令、顶级命令树、status/docs/schema/doctor/version/init/config show 的实现。
**internal/cli/init.go**: `init` 命令处理器，负责初始化工作区目录、upgrade 目录和最小 `config.yaml`。
**internal/cli/id.go**: `id` 域命令处理器，包含 create/list/current/use/register/bind/resolve/recover/profile/import-v1，以及公开但危险的维护命令 `replace-did`。
**internal/cli/debug.go**: `debug db query`、`debug db handle-history` 与 `debug db import-v1` 的 CLI 处理器。
**internal/cli/msg.go**: `msg send/inbox/history/mark-read` 的 CLI 处理器，现已支持 direct + group plain messaging。
**internal/cli/group.go**: `group create/get/join/add/remove/leave/update/members/messages` 的 CLI 处理器；E2EE 群 remove/leave 会路由到 hidden `group.e2ee.remove/leave` 组合编排而不是公开 P4-only 方法。
**internal/cli/group_e2ee.go**: P6 group E2EE 诊断/维护命令处理器；支持本地 exec provider/status/KeyPackage 发布路径、`publish-key-package --purpose normal|recovery|update`（`--recovery` 兼容）、hidden/test-only `group.e2ee.head` local/service epoch 对比、`group.e2ee.notice` pending/repair 拉取与 welcome/commit/update-welcome 重放、PR-C1 hidden `update-key` owner/admin update KeyPackage 轮换、PR-C2 hidden `rejoin` wrapper（canonical `group add --e2ee`）、PR-B2 accepted pending commit 安全 finalize / unrecoverable gap fail-closed 诊断、PR-B3 `recover-member` owner/admin same-device crypto recovery 编排（prepare -> hidden `group.e2ee.recover_member` -> finalize/abort，禁止 P4 `group.add`），以及 PR-B1 `process-leave-request` owner/admin epoch-advancing remove 编排；`contract-test` 仅在显式 flag 下启用。
**internal/identity/types.go**: identity store、legacy scan、command result 等核心类型。
**internal/identity/layout.go**: identity 根目录、index.json、路径与安全写入辅助。
**internal/identity/store.go**: 当前 v2 identity store 的读写、默认 identity 管理。
**internal/identity/legacy.go**: v1 indexed/flat credential layout 扫描与导入。
**internal/identity/did.go**: 本地 DID 文档与 proof 生成，当前默认生成 `e1` profile DID（`key-1` 为 Ed25519）。
**internal/identity/key_compat.go**: legacy ANP 私有 PEM 标签 / SEC1 私钥到标准 PKCS#8 PEM 的兼容迁移，确保旧身份在 ANP Go SDK 0.8.6+ 下仍可完成 DID WBA 签名。
**internal/identity/client.go**: user-service RPC/REST 客户端。
**internal/identity/service.go**: Phase 2/3 高层 identity + user 业务流，封装本地 store、handle lifecycle、`replace_did` DID 换绑能力，以及远端 API。
**internal/identity/did_test.go**: DID 文档和 proof 生成测试。
**internal/identity/store_test.go**: identity store 与 legacy import 测试。
**internal/store/types.go**: SQLite store 的核心类型、记录结构与导入报告类型。
**internal/store/open.go**: pure Go SQLite 打开、WAL / foreign_keys / busy_timeout 配置。
**internal/store/helpers.go**: thread id、row map、schema version、表/视图存在性等辅助函数。
**internal/store/schema.go**: v12 schema、indexes、views 与 `EnsureSchema()`；新增 `contact_handle_bindings` 历史映射表，用于 Handle↔DID 历史绑定。
**internal/store/dao.go**: messages / contacts / contact_handle_bindings / groups / outbox / relationship / rebind / execute_sql 的 DAO。
**internal/store/rebind.go**: 基于工作区 SQLite 打开器的 owner DID 重绑与旧 E2EE 状态清理编排。
**internal/store/import.go**: legacy SQLite 扫描与从 v1 DB 导入 v2 DB。
**internal/store/schema_test.go**: schema 初始化和 version 测试。
**internal/store/dao_test.go**: DAO、thread view、owner rebinding、E2EE 清理测试。
**internal/store/import_test.go**: legacy SQLite 导入测试。
**internal/message/types.go**: direct/group message 与 group lifecycle 的命令输入/输出模型和 transport 错误定义。
**internal/message/auth.go**: direct message 的 hop-level auth 与本地 key / did document 读取。
**internal/message/proof.go**: 基于 ANP Go SDK 0.8.7 的 RFC 9421 origin proof 薄封装。
**internal/message/attachment.go**: 附件文件读取、manifest 组装、控制面/数据面 HTTP 交互与下载解析辅助。
**internal/message/attachment_wire.go**: 附件 control-plane、download ticket 与 direct/group attachment manifest 的 RPC 参数构造器。
**internal/message/attachment_service.go**: direct/group attachment send 与 `msg attachment download` 的业务编排层。
**internal/message/secure.go**: P5 direct E2EE secure send 的首版编排层，使用 ANP Go SDK direct_e2ee、本地文件会话/预密钥存储和 HTTP JSON-RPC；key-service 请求绑定当前 DID 文档里 `ANPMessageService.serviceDid`，并在有可用 sidecar OPK 时优先用 OPK 建链（本地保存 `p5-one-time-prekeys/`）；HTTP inbox/history 现已接入入站密文解密与会话推进，并会顺带补发本地 prekey bundle；轮询路径解密 direct-init 成功后会自动发送 encrypted ACK 并尝试 flush 该 peer 的 `e2ee_outbox`；当 initiator 仍处于 `pending-confirmation` 时，新的 secure 发送会进入 `e2ee_outbox` 排队。
**internal/message/secure_control.go**: secure 控制面与恢复辅助，负责 secure ack/init payload、pending 阶段的 `e2ee_outbox` 排队、secure outbox flush，以及 `msg secure status/init/repair/failed/retry/drop` 需要的本地会话/发件箱读取与重试逻辑。
**internal/message/group_e2ee_provider.go**: `anp-mls` exec provider 抽象；按 `AWIKI_ANP_MLS_BINARY`、测试/运行时注入路径、`PATH` 顺序发现二进制；JSON request 走 stdin、response 走 stdout、日志/错误走 stderr，默认 MLS 根目录为 `<workspace>/mls`，实际 OpenMLS 私有状态按 agent/device 分到子目录，并可扫描同一 agent 下的本地 device state 供收件解密恢复；封装 update-member prepare/finalize/abort 命令且不把 MLS 明文/私材放入 argv，保持 Go 主工程 pure Go / no CGO。
**internal/message/group_e2ee_service.go**: group E2EE 业务编排层；负责 KeyPackage 发布前用当前 DID `key-1` 生成 strict Appendix-B `did_wba_binding.proof`、normal/recovery/update KeyPackage purpose tagging、owner create/add/remove、PR-B1 leave_request 创建与 owner/admin process-leave-request epoch-advancing remove、PR-C1 hidden `update-key` 编排（lease `purpose=update` KeyPackage、anp-mls `update-member-prepare`、hidden `group.e2ee.update`、service accept 后 update finalize、deterministic rejection 后 update abort，且不改变 P4 membership）、PR-B3 recovery KeyPackage tagging 与 owner/admin `recover-member` same-device crypto recovery 编排（anp-mls `recover-member-prepare`、hidden `group.e2ee.recover_member`、service accept 后 finalize、deterministic rejection 后 abort，且不调用/伪装 P4 `group.add`）、stale send epoch mismatch 时从 anp-mls pending status finalize/repair 后重试、pending commit finalize/abort、send encrypt、messages decrypt、P6 notice pending/repair、ratchet tree welcome/commit/update-welcome notice process，以及 MLS AAD 元数据传入 `anp-mls`；status/repair 会读取 hidden `group.e2ee.head`，主动区分 active state loss 的 `recover-member` 和 removed/left rejoin 的 fresh normal KeyPackage + `group add --e2ee` 路径，重复 commit notice 以 local epoch 判定幂等 delivered，缺失 notice gap 输出 `needs_snapshot_or_readd` recovery artifact 并 fail closed；在同一工作区存在目标成员身份时也会本地处理 add/recovery/update 返回的 welcome notice，使 one-shot `anp-mls` agent/device 状态可恢复。
**internal/message/group_wire.go**: group 标准面和 local-only RPC 参数构造器；P6 publish/get/notice 使用 `transport-protected` service/agent target，create 使用 service target，add/remove/leave/send 使用 group target；group E2EE send/remove/leave 会在签名/发送前裁剪 provider-local MLS/private 字段，只把 P6 service 允许的 opaque cipher/commit 字段送到 message-service。
**internal/message/http_client.go**: direct/group message、group lifecycle 与 hidden/test-only P6 notice pull/mark-delivered 的 HTTP JSON-RPC adapter。
**internal/message/ws_proxy_client.go**: websocket 模式下通过本地 bridge 调用 listener/daemon 的 direct/group adapter。
**internal/message/service.go**: direct inbox/send/history/mark-read 的业务编排层，融合 transport、identity、store；支持收件后自动 DID→Handle 补全，以及按 handle 聚合历史 DID 消息。
**internal/message/contact_sync.go**: direct inbox/history 的联系人补全与 Handle 历史 DID 聚合辅助。
**internal/message/group_service.go**: group lifecycle、group message、本地群缓存同步与群 inbox 聚合逻辑；E2EE 群消息在落本地 message view 前先尝试解密，避免返回缓存时覆盖已解密展示内容。
**internal/message/helpers.go**: message 域常用值转换和解码辅助。
**internal/message/proof_test.go**: origin_proof round-trip 测试。
**internal/message/group_wire_test.go**: group RPC 参数构造与签名测试。
**internal/runtime/config.go**: runtime mode（默认 websocket）、listener 默认策略、host notify 默认开启（默认 sink 为 `log`）与本地 bridge 配置解析。
**internal/runtime/listener/types.go**: listener 状态与 session 状态结构。
**internal/runtime/listener/files.go**: listener 的 pid/status/log/socket 路径与状态文件读写。
**internal/runtime/listener/wsclient.go**: 远端 message-service WebSocket client。
**internal/runtime/listener/server.go**: 本地 daemon server、session supervisor、notification 消费与 SQLite 落库；首条陌生来信会按 DID 反查 Handle 并更新通讯录。
**internal/runtime/listener/contact_sync.go**: websocket 收件路径的 DID→Handle 自动补全与联系人重绑定辅助。
**internal/runtime/listener/host_notify.go**: websocket 下行通知到宿主事件的标准化、字段裁剪与 host notify sink 注册入口；direct/group 事件可带 `sender_handle` / `recipient_handle`。
**internal/runtime/listener/openclaw_host_notify.go**: OpenClaw 适配器，负责从本地 route registry 读取已注册 routes，并通过 `/hooks/agent` 执行 webhook fan-out；事件文本仍保留 sender/recipient handle 等可读字段。
**internal/runtime/listener/service.go**: listener 系统服务编排，基于 `kardianos/service` 提供 install/start/stop/uninstall 与 service-run；`start` 在服务缺失时会自动 install，并等待 bridge ready 后再返回。
**internal/runtime/listener/manager.go**: listener 的 start/stop/restart/status/run 管理逻辑与系统服务状态聚合。
**internal/runtime/bridge_unix.go / bridge_windows.go**: 本地 bridge 跨平台 IPC 实现；Unix 平台使用 Unix Domain Socket，Windows 使用 Named Pipe。
**docs/architecture/awiki-v2-architecture.md**: awiki CLI V2 的整体架构设计文档。
**docs/architecture/awiki-command-v2.md**: awiki CLI 命令模型与命令层设计文档。
**docs/architecture/anp-service-discovery.md**: awiki-cli 生成 DID 文档时的 `ANPMessageService` 填写规则、配置约束与实施记录。
**docs/architecture/websocket-host-notification-v1.md**: websocket listener 向宿主 Agent 暴露统一通知事件的 v1 设计文档。
**docs/architecture/openclaw-host-adapter-v1.md**: websocket host notification 到 OpenClaw `/hooks/agent` 的 v1 适配设计文档。
**docs/architecture/output-format.md**: CLI 输出格式约束与展示设计文档。
**docs/plan/awiki-v2-implementation-plan.md**: v2 的总体落地实施规划。
**docs/plan/phase-0/implementation-constraints.md**: Phase 0 冻结后的实现约束表。
**docs/plan/phase-0/capability-mapping.md**: v2 命令、v1 脚本、服务 API 的能力映射。
**docs/plan/phase-0/audit-findings.md**: Phase 0 审计冲突与裁决。
**docs/plan/phase-0/adr-index.md**: Phase 0 ADR 索引。

**scripts/release/build-anp-mls.sh**: 本地 release hardening 辅助脚本，从同级 `../anp/anp/rust` 构建 `anp-mls` 并 stage 到 `dist/anp-mls/<os>-<arch>/`，用于 awiki-cli group E2EE helper 的打包或 `AWIKI_ANP_MLS_BINARY` 注入验证。

## 当前实现边界

### 已实现

- Phase 1：
  - `awiki-cli` 根命令与顶级命令树
  - `init` 工作区初始化命令
  - 全局 flags：`--format`、`--jq`、`--dry-run`、`--identity`、`--verbose`
  - 统一输出 envelope
  - 静态 `schema`
  - 内建 `docs`
  - 基础 `doctor`
  - `config show`
- Phase 2：
  - 单根目录 identity store 与 `index.json`
  - default identity 解析与 `id list/current/use/status`
  - 本地 DID identity 创建 `id create`（内部/bootstrap，用于迁移或调试；默认从公开 help 隐藏）
  - v1 legacy credential scan / import：`id import-v1`
- Phase 3：
  - handle registration / bind / resolve / recover / profile 的首版实现
  - 危险维护命令 `id replace-did`，接入 `POST /did-auth/rpc` 的 `replace_did`，用于为指定 `--identity` 生成新的 e1 DID 并替代旧 DID
  - local-only identity vs registered user 状态判断
  - `msg` / `runtime listener` 的 user gating 首版实现
  - current/default identity 自动回填到配置解析结果
  - `id profile get/set` 已正式接入 `user-service` DID Profile RPC
- Phase 4：
  - pure Go SQLite 打开与 `EnsureSchema()`
  - v11 tables / indexes / views
  - 本地 DAO：messages、contacts、contact_handle_bindings、relationship_events、groups、group_members、e2ee_outbox、e2ee_sessions
  - owner_did rebind 与 E2EE 清理 helper
  - legacy SQLite scan / import
  - 默认 Python v1 → Go workspace upgrade 中，对已导入的 handle k1 DID 自动尝试 `replace_did` 迁移到 e1 DID；失败不阻断整次升级，但会记录 warning
  - `debug db query`
  - `debug db import-v1`
  - `doctor` / `config show` 的数据库诊断增强
- Phase 5（当前首版已落地 direct plain，P5 secure direct send 首版接入）：
  - `msg send --to`
  - `msg send --group`
  - `msg send --file`
  - `msg attachment download`
  - `msg inbox`
  - `msg history --with`
  - `msg mark-read`
  - `group create/get/join/add/remove/leave/update/members/messages`
  - HTTP JSON-RPC adapter
  - attachment control-plane / data-plane HTTP upload, ticket, download
  - websocket runtime bridge / local daemon direct+group client
  - direct/group origin_proof 生成与本地消息落库
  - `msg send --secure on` 首版通过 ANP Go SDK P5 direct_e2ee 生成 `direct_init` / `direct_cipher` 并走 HTTP JSON-RPC；prekey key-service 请求使用本地 DID 文档的 `ANPMessageService.serviceDid` 做 `meta.target` 绑定，并在远端返回 top-level `one_time_prekey` 时优先走 OPK 建链；HTTP inbox/history 已支持入站密文自动解密与会话推进，并在轮询 direct-init 时自动 encrypted ACK + flush `e2ee_outbox`；listener 现已支持 secure incoming 自动解密、自动 first-reply/ack、以及在收到 secure ack 后 flush 已排队的 `e2ee_outbox`
  - `msg secure status` / `init` / `repair` / `failed` / `retry` / `drop` 已有首版命令面实现；其中 `init` 当前通过发送 product-local secure control init 预热会话，`repair` 会重置本地会话并重排该 peer 的 failed outbox 后重新 init
  - attachment control 不再发送独立业务 proof
- Phase 8（当前首版已落地 content page）：
  - `page create/list/get/update/rename/delete`
  - `page create/update --visibility public|draft|unlisted`
  - 通过 `POST /content/rpc` 接入 `user-service` content pages API
- Phase 7（当前首版已落地 websocket 服务端，先于 secure phase 提前接入）：
  - `runtime status`
  - `runtime apply`
  - `runtime setup`
  - `runtime mode get/set`
  - `runtime listener status/install/start/stop/restart/uninstall`
  - `runtime listener config show/set`
  - `runtime listener enable/disable`
  - `runtime host-notify config show/set`
  - `runtime host-notify enable/disable`
  - `runtime host-notify openclaw set/set-token/clear-token`
  - 隐藏命令 `runtime listener run`
  - 隐藏命令 `runtime listener service-run`
  - 后台 listener 进程、pid/status/socket 管理
  - 本地 daemon / unix socket bridge 服务端
  - 远端单 websocket 连接与 `direct.incoming` / `group.incoming` / `group.state_changed` 下行落库
  - websocket 下行到宿主 Agent 的统一通知事件 v1（字段裁剪 + noop/log/file/openclaw sink）
  - `host_notify.enabled` 默认开启；默认 sink 为 `log`
  - listener 系统服务化：macOS LaunchAgent / Linux systemd / Windows Service
  - Windows bridge 主路径改为 Named Pipe；Unix 平台保持 Unix Domain Socket
  - websocket session 断线自动重连、周期 ping 保活、桥接请求按连接状态快速失败后由上层回退 HTTP
- Phase 7.1（当前首版已落地 ANP SDK 鉴权）：
  - 基于 `DIDWbaAuthHeader` 的 HTTP hop auth
  - 401 后自动重试与 challenge 处理
  - 从响应头捕获 bearer token 并回写 identity store
  - listener 在 websocket 模式下可自动尝试 bootstrap JWT
- Phase 7.2：
  - `id` 域公共输出已统一移除 `user_id`，对外保持 handle-first 身份语义
  - direct origin proof scheme 已切换为 `anp-rfc9421-origin-proof-v1`
  - websocket `direct.incoming` 已按最新 P3 结构仅消费 `params.meta/body/auth`，不再依赖 `server` 包装字段

### 尚未实现

- `msg` 域中 direct plain 已实现，P5 secure direct 出站首版已接入；HTTP inbox/history 的 P5 入站自动解密、轮询 direct-init 自动 ACK 与 outbox flush 已接入；websocket listener 已能解密 secure incoming、自动 first-reply/ack，并在 secure ack 后尝试 flush `e2ee_outbox`；`msg secure status/init/repair/failed/retry/drop` 有首版，但完整 runtime 联调与更强系统测试仍未完成
- `group` 域的 plain lifecycle / local view / group messaging 已接入，`people` 仍大多为 stub；`page` 已完成 content pages 首版
- secure E2EE 业务流目前只完成 P5 出站首版；group E2EE 已接入 CLI 侧 `anp-mls` exec 编排、KeyPackage 发布（含 recovery/update purpose）、group-e2ee create/add/remove/send、安全 self-leave request、owner/admin process-leave-request、owner/admin recover-member same-DID/device crypto recovery、hidden owner/admin update-key leaf replacement UX、removed/left rejoin 的 `group add --e2ee` UX、pending commit finalize/abort、commit/update-welcome repair 与轮询消息本地解密分支，并已有隐藏 focused target 覆盖真实 OpenMLS Alice/Bob 最小闭环与 PR-B3 recovery；对外 discovery 仍保持隐藏，External Commit、多设备、cloud snapshot 与完整 MLS 群管理能力仍未实现；update-key/rejoin 仍属 hidden/test-only PR-C1/PR-C2 路径。

## 开发与验证约定

- 本机 Go 版本基线固定为 `1.22.x`。
- 本机若无 `go`，优先使用 Docker 的 `golang:1.22` 镜像执行：
  - `go mod tidy`
  - `gofmt -w $(find cmd internal -name '*.go')`
  - `CGO_ENABLED=0 go build ./...`
  - `CGO_ENABLED=0 go test ./...`
- Phase 2 / Phase 3 / Phase 4 的本地 smoke test 可通过临时 `AWIKI_*` 工作区环境变量完成，避免污染真实目录。
- 代码注释和日志保持英文；命令行对用户的交互输出遵循统一 JSON envelope。

⚡触发器: 一旦本文件夹增删文件、调整架构、修改服务依赖、补充新的 Go 模块目录，或切换 Phase 实现边界，请立即重写此文档。
