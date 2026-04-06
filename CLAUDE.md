# awiki-cli/

> L2 文档 | 父级: [../CLAUDE.md](../CLAUDE.md) | 分形协议: 三层结构

1. **地位**: `awiki-cli` 是 awiki 的命令行客户端项目，负责把命令行输入编排成对后端服务的 API 调用，并为后续 AI/人类用户提供统一 CLI 产品面。
2. **边界**: 输入是 CLI 命令、参数、配置和认证信息；输出是对同级后端服务的请求以及面向终端用户的命令执行结果。本仓库只承担客户端侧的命令编排与交互，不承载服务端业务真相。
3. **约束**:
   - 后端服务依赖同级的 `../user-service/` 和 `../message-service/`。
   - 消息相关 API 文档位于 `../message-service/docs/api/`。
   - 用户相关 API 文档位于 `../user-service/docs/api/`。
   - 本项目是一个重写项目，重写参考实现位于同级 `../awiki-agent-id-message/`。
   - CLI 交互方式与工程组织可以参考同级飞书 CLI 仓库 `../cli/`。
   - v2 当前实现语言是 **Go**，并且要求保持 **pure Go / no CGO**。
   - 若需要做系统兼容性壳层，可放在 TypeScript/Node 的薄壳中，不在 Go 核心里引入 CGO。
   - 如果命令实现涉及服务端 API 变化，需要同步更新对应服务仓库下的 API 文档。

## 项目背景

- 项目名称：`awiki-cli`
- 项目形态：命令行客户端
- 当前阶段：Phase 1 CLI 产品壳已落地基础代码骨架
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

## 成员清单

**README.md**: 仓库入口说明文件。  
**go.mod**: Go 模块定义，当前 Phase 1 依赖 `cobra`、`gojq`、`yaml.v3`，要求 pure Go。  
**cmd/awiki-cli/main.go**: `awiki-cli` 主程序入口。  
**internal/buildinfo/buildinfo.go**: 版本、构建时间、CGO 状态等构建信息。  
**internal/cmdmeta/catalog.go**: 静态命令元数据目录，作为 schema/命令骨架的事实来源。  
**internal/config/config.go**: XDG 路径解析、AWIKI/AVIKI/E2E 环境变量兼容读取、config.yaml 解析。  
**internal/output/output.go**: 统一 success/error JSON envelope、`--jq`、table/ndjson 渲染。  
**internal/doctor/doctor.go**: Phase 1 基础诊断实现，检查构建、配置、env、identity store、SQLite、legacy 路径。  
**internal/docs/topics.go**: CLI 内建 docs 主题索引。  
**internal/cli/app.go**: CLI 应用装配与统一错误输出入口。  
**internal/cli/root.go**: Cobra 根命令、顶级命令树、全局 flags、status/docs/schema/doctor/version/config show 的 Phase 1 实现。  
**docs/architecture/awiki-v2-architecture.md**: awiki CLI V2 的整体架构设计文档。  
**docs/architecture/awiki-command-v2.md**: awiki CLI 命令模型与命令层设计文档。  
**docs/architecture/output-format.md**: CLI 输出格式约束与展示设计文档。  
**docs/plan/awiki-v2-implementation-plan.md**: v2 的总体落地实施规划。  
**docs/plan/phase-0/implementation-constraints.md**: Phase 0 冻结后的实现约束表。  
**docs/plan/phase-0/capability-mapping.md**: v2 命令、v1 脚本、服务 API 的能力映射。  
**docs/plan/phase-0/audit-findings.md**: Phase 0 审计冲突与裁决。  
**docs/plan/phase-0/adr-index.md**: Phase 0 ADR 索引。  

## 当前实现边界（Phase 1）

- 已有：
  - `awiki-cli` 根命令与顶级命令树
  - 全局 flags：`--format`、`--jq`、`--dry-run`、`--identity`、`--verbose`
  - 统一输出 envelope
  - 静态 `schema`
  - 内建 `docs`
  - 基础 `doctor`
  - `config show`
- 尚未实现业务逻辑：
  - `id`、`msg`、`group`、`runtime`、`people`、`page`、`debug` 的真实执行路径当前仍以 stub 为主
  - identity store、SQLite、secure、listener、迁移、发布链路属于后续阶段

⚡触发器: 一旦本文件夹增删文件、调整架构、修改服务依赖、补充新的 Go 模块目录，或切换 Phase 实现边界，请立即重写此文档。
