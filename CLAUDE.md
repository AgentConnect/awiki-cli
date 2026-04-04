# awiki-cli/

> L2 文档 | 父级: [../CLAUDE.md](../CLAUDE.md) | 分形协议: 三层结构

1. **地位**: `awiki-cli` 是 awiki 的命令行客户端项目，负责把命令行输入编排成对后端服务的 API 调用。
2. **边界**: 输入是 CLI 命令、参数、配置和认证信息；输出是对同级后端服务的请求以及面向终端用户的命令执行结果。本仓库只承担客户端侧的命令编排与交互，不承载服务端业务真相。
3. **约束**:
   - 后端服务依赖同级的 `../user-service/` 和 `../message-service/`。
   - 消息相关 API 文档位于 `../message-service/docs/api/`。
   - 用户相关 API 文档位于 `../user-service/docs/api/`。
   - 本项目是一个重写项目，重写参考实现位于同级 `../awiki-agent-id-message/`。
   - CLI 交互方式与工程组织可以参考同级飞书 CLI 仓库 `../cli/`。
   - 作为 Python 项目，依赖管理与运行统一使用 `uv`。
   - 如果命令实现涉及服务端 API 变化，需要同步更新对应服务仓库下的 API 文档。

## 项目背景

- 项目名称：`awiki-cli`
- 项目形态：命令行客户端
- 通信模式：通过 CLI 命令调用 API 连接 awiki 服务端
- 主要服务端依赖：
  - `../user-service/`
  - `../message-service/`
- 关键文档入口：
  - 消息 API：`../message-service/docs/api/`
  - 用户 API：`../user-service/docs/api/`
- 重写参考：
  - Python 版本 CLI：`../awiki-agent-id-message/`
  - 飞书 CLI：`../cli/`

## 成员清单

**README.md**: 仓库入口说明文件，当前内容较少，可在后续逐步补充项目使用说明。  
**docs/architecture/awiki-v2-architecture.md**: awiki CLI V2 的整体架构设计文档。  
**docs/architecture/awiki-command-v2.md**: awiki CLI 命令模型与命令层设计文档。  
**docs/architecture/cli-init.md**: CLI 初始化流程设计文档。  
**docs/architecture/overall-init.md**: 整体初始化与启动链路设计文档。  
**docs/architecture/output-format.md**: CLI 输出格式约束与展示设计文档。

⚡触发器: 一旦本文件夹增删文件、调整架构、修改服务依赖或替换参考实现，请立即重写此文档。
