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
  - ANP Go SDK（远端模块依赖）：`github.com/agent-network-protocol/anp/golang@v0.8.1`

## 成员清单

**README.md**: 仓库入口说明文件。  
**go.mod / go.sum**: Go 模块定义与依赖锁定；当前 Go 版本基线固定为 `1.22`，依赖 `cobra`、`gojq`、`yaml.v3`、`secp256k1/v4`、`modernc.org/sqlite`，并直接使用远端模块依赖 `github.com/agent-network-protocol/anp/golang@v0.8.1`，要求 pure Go。  
**cmd/awiki-cli/main.go**: `awiki-cli` 主程序入口。  
**internal/buildinfo/buildinfo.go**: 版本、构建时间、CGO 状态等构建信息。  
**internal/cmdmeta/catalog.go**: 静态命令元数据目录，作为 schema/命令骨架的事实来源。  
**internal/config/config.go**: XDG 路径解析、AWIKI/AVIKI/E2E 环境变量兼容读取、config.yaml 解析。  
**internal/output/output.go**: 统一 success/error JSON envelope、`--jq`、table/ndjson 渲染。  
**internal/doctor/doctor.go**: 诊断实现，检查构建、配置、env、identity store、SQLite、legacy 路径与 legacy DB。  
**internal/docs/topics.go**: CLI 内建 docs 主题索引。  
**internal/anpsdk/registry.go**: ANP Go SDK 的远端模块依赖入口，统一暴露 DID WBA、HTTP Signatures、direct_e2ee 等后续 Phase 要用到的基础能力。  
**internal/authsdk/session.go**: 基于 ANP SDK `DIDWbaAuthHeader` 的身份鉴权封装，负责 HTTP/WSS hop auth、401 重试、JWT token 捕获与持久化。  
**internal/cli/app.go**: CLI 应用装配、配置解析与统一错误输出入口。  
**internal/cli/root.go**: Cobra 根命令、顶级命令树、status/docs/schema/doctor/version/config show 的实现。  
**internal/cli/id.go**: `id` 域命令处理器，包含 create/list/current/use/register/bind/resolve/recover/profile/import-v1。  
**internal/cli/debug.go**: `debug db query` 与 `debug db import-v1` 的 CLI 处理器。  
**internal/cli/msg.go**: `msg send/inbox/history/mark-read` 的 CLI 处理器，现已支持 direct + group plain messaging。  
**internal/cli/group.go**: `group create/get/join/add/remove/leave/update/members/messages` 的 CLI 处理器。  
**internal/identity/types.go**: identity store、legacy scan、command result 等核心类型。  
**internal/identity/layout.go**: identity 根目录、index.json、路径与安全写入辅助。  
**internal/identity/store.go**: 当前 v2 identity store 的读写、默认 identity 管理。  
**internal/identity/legacy.go**: v1 indexed/flat credential layout 扫描与导入。  
**internal/identity/did.go**: 本地 DID 文档与 proof 生成，使用 pure Go secp256k1。  
**internal/identity/client.go**: user-service RPC/REST 客户端。  
**internal/identity/service.go**: Phase 2/3 高层 identity + user 业务流，封装本地 store、handle lifecycle 与远端 API。  
**internal/identity/did_test.go**: DID 文档和 proof 生成测试。  
**internal/identity/store_test.go**: identity store 与 legacy import 测试。  
**internal/store/types.go**: SQLite store 的核心类型、记录结构与导入报告类型。  
**internal/store/open.go**: pure Go SQLite 打开、WAL / foreign_keys / busy_timeout 配置。  
**internal/store/helpers.go**: thread id、row map、schema version、表/视图存在性等辅助函数。  
**internal/store/schema.go**: v11 schema、indexes、views 与 `EnsureSchema()`。  
**internal/store/dao.go**: messages / contacts / groups / outbox / relationship / rebind / execute_sql 的 DAO。  
**internal/store/import.go**: legacy SQLite 扫描与从 v1 DB 导入 v2 DB。  
**internal/store/schema_test.go**: schema 初始化和 version 测试。  
**internal/store/dao_test.go**: DAO、thread view、owner rebinding、E2EE 清理测试。  
**internal/store/import_test.go**: legacy SQLite 导入测试。  
**internal/message/types.go**: direct/group message 与 group lifecycle 的命令输入/输出模型和 transport 错误定义。  
**internal/message/auth.go**: direct message 的 hop-level auth 与本地 key / did document 读取。  
**internal/message/proof.go**: 基于 ANP Go SDK 0.8.0 的 RFC 9421 origin proof 薄封装。  
**internal/message/attachment.go**: 附件文件读取、manifest 组装、控制面/数据面 HTTP 交互与下载解析辅助。  
**internal/message/attachment_wire.go**: 附件 control-plane、download ticket 与 direct/group attachment manifest 的 RPC 参数构造器。  
**internal/message/attachment_service.go**: direct/group attachment send 与 `msg attachment download` 的业务编排层。  
**internal/message/group_wire.go**: group 标准面和 local-only RPC 参数构造器。  
**internal/message/http_client.go**: direct/group message 与 group lifecycle 的 HTTP JSON-RPC adapter。  
**internal/message/ws_proxy_client.go**: websocket 模式下通过本地 bridge 调用 listener/daemon 的 direct/group adapter。  
**internal/message/service.go**: direct inbox/send/history/mark-read 的业务编排层，融合 transport、identity、store。  
**internal/message/group_service.go**: group lifecycle、group message、本地群缓存同步与群 inbox 聚合逻辑。  
**internal/message/helpers.go**: message 域常用值转换和解码辅助。  
**internal/message/proof_test.go**: origin_proof round-trip 测试。  
**internal/message/group_wire_test.go**: group RPC 参数构造与签名测试。  
**internal/runtime/config.go**: runtime mode（http/websocket）与本地 websocket bridge 配置解析。  
**internal/runtime/listener/types.go**: listener 状态与 session 状态结构。  
**internal/runtime/listener/files.go**: listener 的 pid/status/log/socket 路径与状态文件读写。  
**internal/runtime/listener/wsclient.go**: 远端 message-service WebSocket client。  
**internal/runtime/listener/server.go**: 本地 daemon server、session supervisor、notification 消费与 SQLite 落库。  
**internal/runtime/listener/manager.go**: listener 的 start/stop/restart/status/run 管理逻辑。  
**docs/architecture/awiki-v2-architecture.md**: awiki CLI V2 的整体架构设计文档。  
**docs/architecture/awiki-command-v2.md**: awiki CLI 命令模型与命令层设计文档。  
**docs/architecture/anp-service-discovery.md**: awiki-cli 生成 DID 文档时的 `ANPMessageService` 填写规则、配置约束与实施记录。
**docs/architecture/output-format.md**: CLI 输出格式约束与展示设计文档。  
**docs/plan/awiki-v2-implementation-plan.md**: v2 的总体落地实施规划。  
**docs/plan/phase-0/implementation-constraints.md**: Phase 0 冻结后的实现约束表。  
**docs/plan/phase-0/capability-mapping.md**: v2 命令、v1 脚本、服务 API 的能力映射。  
**docs/plan/phase-0/audit-findings.md**: Phase 0 审计冲突与裁决。  
**docs/plan/phase-0/adr-index.md**: Phase 0 ADR 索引。  

## 当前实现边界

### 已实现

- Phase 1：
  - `awiki-cli` 根命令与顶级命令树
  - 全局 flags：`--format`、`--jq`、`--dry-run`、`--identity`、`--verbose`
  - 统一输出 envelope
  - 静态 `schema`
  - 内建 `docs`
  - 基础 `doctor`
  - `config show`
- Phase 2：
  - XDG identity store 与 `index.json`
  - default identity 解析与 `id list/current/use/status`
  - 本地 DID identity 创建 `id create`（内部/bootstrap，用于迁移或调试；默认从公开 help 隐藏）
  - v1 legacy credential scan / import：`id import-v1`
- Phase 3：
  - handle registration / bind / resolve / recover / profile 的首版实现
  - local-only identity vs registered user 状态判断
  - `msg` / `runtime listener` 的 user gating 首版实现
  - current/default identity 自动回填到配置解析结果
  - `id profile get/set` 已正式接入 `user-service` DID Profile RPC
- Phase 4：
  - pure Go SQLite 打开与 `EnsureSchema()`
  - v11 tables / indexes / views
  - 本地 DAO：messages、contacts、relationship_events、groups、group_members、e2ee_outbox、e2ee_sessions
  - owner_did rebind 与 E2EE 清理 helper
  - legacy SQLite scan / import
  - `debug db query`
  - `debug db import-v1`
  - `doctor` / `config show` 的数据库诊断增强
- Phase 5（当前首版已落地 direct plain）：
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
  - attachment control 不再发送独立业务 proof
- Phase 8（当前首版已落地 content page）：
  - `page create/list/get/update/rename/delete`
  - `page create/update --visibility public|draft|unlisted`
  - 通过 `POST /content/rpc` 接入 `user-service` content pages API
- Phase 7（当前首版已落地 websocket 服务端，先于 secure phase 提前接入）：
  - `runtime status`
  - `runtime setup`
  - `runtime mode get/set`
  - `runtime listener status/install/start/stop/restart/uninstall`
  - 隐藏命令 `runtime listener run`
  - 后台 listener 进程、pid/status/socket 管理
  - 本地 daemon / unix socket bridge 服务端
  - 远端单 websocket 连接与 `direct.incoming` / `group.incoming` / `group.state_changed` 下行落库
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

- `msg` 域中 direct plain 已实现，listener 服务端首版也已实现，但 websocket 远端真实联调与 secure E2EE 仍未完成
- `group` 域的 plain lifecycle / local view / group messaging 已接入，`people` 仍大多为 stub；`page` 已完成 content pages 首版
- secure E2EE 业务流、group plain、发布链路属于后续阶段

## 开发与验证约定

- 本机 Go 版本基线固定为 `1.22.x`。
- 本机若无 `go`，优先使用 Docker 的 `golang:1.22` 镜像执行：
  - `go mod tidy`
  - `gofmt -w $(find cmd internal -name '*.go')`
  - `CGO_ENABLED=0 go build ./...`
  - `CGO_ENABLED=0 go test ./...`
- Phase 2 / Phase 3 / Phase 4 的本地 smoke test 可通过临时 `AWIKI_*` XDG 环境变量完成，避免污染真实目录。
- 代码注释和日志保持英文；命令行对用户的交互输出遵循统一 JSON envelope。

⚡触发器: 一旦本文件夹增删文件、调整架构、修改服务依赖、补充新的 Go 模块目录，或切换 Phase 实现边界，请立即重写此文档。
