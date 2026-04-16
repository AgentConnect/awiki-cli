# Runtime 参考

## 目的

当你在 `awiki-cli` 中处理 runtime 选择和长连接投递任务时，使用本参考文档，包括：runtime mode 检查、websocket listener 控制、宿主通知配置，以及当前 heartbeat 限制说明。

本文件是 **reference**，不是入口 skill。只有当任务明确涉及 runtime mode、listener、websocket transport、host notification 或 runtime 恢复时，才加载本文件。

## 当前状态

- 状态：**部分实现**
- 当前已实现：
  - `runtime status`
  - `runtime apply`
  - `runtime setup`
  - `runtime mode get`
  - `runtime mode set`
  - `runtime listener status/install/start/stop/restart/uninstall`
  - `runtime listener config show/set`
  - `runtime listener enable/disable`
  - `runtime host-notify config show/set`
  - `runtime host-notify enable/disable`
  - `runtime host-notify openclaw set/set-token/clear-token`
- 已规划但尚未实现：
  - `runtime heartbeat status/install/run-once`

## 当前行为说明

- 当 listener service 缺失时，`runtime listener start` 会自动安装该 service
- `runtime setup` 和 `runtime mode set` 在写入配置后会应用 runtime policy；在 websocket 模式下，如果 listener 启用且 auto-install/auto-start 打开，它们可能安装并启动 listener service
- `runtime listener install` 仍然作为显式的仅安装路径存在
- 不要把 heartbeat 描述成当前仓库中已经实现

## 适用场景

- 查看 runtime mode
- 在 `http` 与 `websocket` 之间切换
- 控制实时 listener 和宿主通知设置
- 理解当前 heartbeat 契约及其限制

## 核心概念

- **runtime mode**：仅在 runtime 领域暴露的传输选择
- **listener**：websocket 侧的长驻进程
- **daemon bridge**：websocket 模式下使用的本地进程边界
- **host notify**：转发到 `log`、`file` 或 `openclaw` 的标准化 websocket 事件
- **heartbeat**：契约中保留、但尚未实现的定时可靠性路径

## 决策规则

- 需要知道当前传输状态 -> `runtime status` 或 `runtime mode get`
- 需要按当前 `config.yaml` 收敛 runtime 与 listener 状态 -> `runtime apply`
- 需要初始化 runtime 文件和本地 store -> `runtime setup --mode <http|websocket>`
- 需要修改持久化 listener 策略 -> 使用 `runtime listener config show/set`
- 需要开启或关闭 listener 管理并应用 runtime 状态 -> 使用 `runtime listener enable` 或 `runtime listener disable`
- 需要 websocket 实时接收 -> 先设置 websocket mode，再使用 listener 命令
- 需要宿主/webhook 通知 -> 先检查 `runtime host-notify config show`，再设置 sink 或使用 `runtime host-notify enable`
- messaging 返回 transport-unavailable -> 检查 listener 状态，或切回 `http`
- 需要 heartbeat 自动化 -> 说明该命令族在当前仓库中仍处于规划阶段

## Canonical 命令

当前已实现：

- `awiki-cli runtime status`
- `awiki-cli runtime apply`
- `awiki-cli runtime setup --mode http|websocket`
- `awiki-cli runtime mode get`
- `awiki-cli runtime mode set <http|websocket>`
- `awiki-cli runtime listener status`
- `awiki-cli runtime listener install`
- `awiki-cli runtime listener start`
- `awiki-cli runtime listener stop`
- `awiki-cli runtime listener restart`
- `awiki-cli runtime listener uninstall`
- `awiki-cli runtime listener config show`
- `awiki-cli runtime listener config set [--enabled true|false] [--auto-install true|false] [--auto-start true|false]`
- `awiki-cli runtime listener enable`
- `awiki-cli runtime listener disable`
- `awiki-cli runtime host-notify config show`
- `awiki-cli runtime host-notify config set --sink noop|log|file|openclaw`
- `awiki-cli runtime host-notify enable`
- `awiki-cli runtime host-notify disable`
- `awiki-cli runtime host-notify openclaw set --hook-url <url> --agent-id <id> --hook-name <name>`
- `awiki-cli runtime host-notify openclaw set-token --value <token>`
- `awiki-cli runtime host-notify openclaw clear-token`

## 常见模式

### 初始化 websocket 模式

1. `awiki-cli runtime status`
2. `awiki-cli runtime setup --mode websocket --dry-run`
3. `awiki-cli runtime setup --mode websocket`
4. `awiki-cli runtime listener status`

在默认 websocket listener policy 下，第 3 步可能已经安装并启动 listener service。

### 按当前配置收敛 runtime 状态

1. `awiki-cli runtime status`
2. `awiki-cli runtime apply --dry-run`
3. `awiki-cli runtime apply`

### 持久化关闭 listener auto-start

1. `awiki-cli runtime listener config show`
2. `awiki-cli runtime listener config set --auto-install false --auto-start false --dry-run`
3. `awiki-cli runtime listener config set --auto-install false --auto-start false`

### 从 transport 问题中恢复

1. `awiki-cli runtime listener status`
2. `awiki-cli runtime listener restart`
3. 如果仍然阻塞，执行 `awiki-cli runtime mode set http`

### 显式启用宿主通知

1. `awiki-cli runtime host-notify config show`
2. `awiki-cli runtime host-notify config set --sink openclaw --dry-run`
3. `awiki-cli runtime host-notify config set --sink openclaw`
4. `awiki-cli runtime host-notify openclaw set --hook-url http://127.0.0.1:18789/hooks/agent --agent-id main --hook-name AWiki`

## 副作用与确认

- 需要显式确认：
  - `runtime apply`
  - `runtime setup`
  - `runtime mode set`
  - `runtime listener install/start/stop/restart/uninstall`
  - `runtime listener config set`
  - `runtime listener enable/disable`
  - `runtime host-notify enable/disable`
  - `runtime host-notify config set`
  - `runtime host-notify openclaw set/set-token/clear-token`
- 仅限内部：
  - `runtime listener run`
  - `runtime listener service-run`

## 错误处理

- runtime mode 不清楚 -> 检查 `awiki-cli schema runtime mode set`
- listener 状态不清楚 -> `awiki-cli runtime listener status`
- host notify 配置不清楚 -> `awiki-cli runtime host-notify config show`
- 配置或路径不清楚 -> `awiki-cli config show`
- 更广泛的 runtime 故障 -> `awiki-cli doctor`

## 实现说明

- 业务命令不应直接选择 transport
- `runtime apply` 会按当前配置执行 runtime bootstrap，并可能触发 listener policy 带来的 service 状态变化
- `runtime listener start` 现在在需要时会自动安装 service
- `runtime listener config show/set` 是 `enabled`、`auto_install` 和 `auto_start` 的持久化控制面
- `runtime host_notify.enabled` 默认开启，而默认 sink 仍为 `log`
- OpenClaw 作为 `host-notify` 的宿主接入推荐路径，可参考 `00-installation.md`
- `runtime heartbeat` 在当前仓库状态下仍处于规划阶段

## 相关参考

- `03-messaging.md`
- `01-onboarding.md`
- `08-debug.md`
- `00-installation.md`
