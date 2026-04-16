# 消息参考

## 目的

当你在 `awiki-cli` 中处理私聊和群消息任务时，使用本参考文档，包括：inbox 审阅、私聊历史查看、附件发送与下载、已读状态更新，以及发送明文消息。

本文件是 **reference**，不是入口 skill。只有当任务明确涉及 direct message、group message、inbox、history、未读状态或当前 secure-message 契约时，才加载本文件。

## 当前状态

- 状态：**部分实现**
- 当前已实现：
  - `msg send`
  - `msg attachment download`
  - `msg inbox`
  - `msg history`
  - `msg mark-read`
- 已保留但尚未实现：
  - `msg secure status`
  - `msg secure init`
  - `msg secure repair`
  - `msg secure failed`
  - `msg secure retry`
  - `msg secure drop`
- 契约中存在 `--secure on`，但当前服务端对 secure direct messaging 返回 unsupported

## 适用场景

- 发送私聊消息
- 向已有群组发送文本
- 向私聊或群消息发送附件
- 从私聊或群消息中下载单个附件
- 查看 inbox 或私聊历史
- 标记消息已读
- 理解当前 secure-message 契约及其限制

## 核心概念

- **direct message**：一个身份发送给一个对端，使用 `--to` 选择
- **group message**：一个身份发送到一个已有群组，使用 `--group` 选择
- **inbox**：跨 direct 和 group 范围的聚合读路径
- **history**：与单个目标之间的私聊线程历史
- **read state**：本地未读状态跟踪
- **secure messaging contract**：为未来 direct E2EE 流程预留的命令族

## 当前支持矩阵

| 范围 × 安全性 | 当前状态 | 说明 |
|---|---|---|
| direct + plain | 已实现 | 使用 `msg send --to ...` |
| direct + secure | 计划中 | 契约中存在 `--secure on`，但当前服务端返回 unsupported |
| group + plain | 已实现 | 使用 `msg send --group ...` |
| group + secure | 不支持 | 不在当前仓库路径内 |

## 资源模型

- `Identity -> Direct Thread -> Message`
- `Identity -> Group Membership -> Group Message`

## 决策规则

- 给单个对象发送消息 -> `awiki-cli msg send --to <handle|did> --text ...`
- 向群组发送消息 -> `awiki-cli msg send --group <group_did> --text ...`
- 发送附件 -> `awiki-cli msg send (--to <handle|did> | --group <group_did>) --file ./hello.txt [--text "..."] [--mime-type ...]`
- 从消息中保存一个文件 -> `awiki-cli msg attachment download ...`
- 查看近期状态 -> `awiki-cli msg inbox ...`
- 查看单个私聊线程 -> `awiki-cli msg history --with <handle|did>`
- 清除未读状态 -> `awiki-cli msg mark-read ...`
- 需要变更群生命周期 -> 使用 `04-groups.md`
- 需要处理 transport setup -> 使用 `05-runtime.md`

## Canonical 命令

- `awiki-cli msg send --to <target> --text "Hello"`
- `awiki-cli msg send --group <group_did> --text "Hello group"`
- `awiki-cli msg send (--to <target> | --group <group_did>) --file ./hello.txt [--text "attachment caption"] [--mime-type text/plain]`
- `awiki-cli msg attachment download (--with <target> | --group <group_did>) --message-id <message_id> [--attachment-id <attachment_id>] --output ./downloads/file.bin`
- `awiki-cli msg inbox [--scope all|direct|group] [--with <target>] [--group <group_did>] [--unread] [--limit <n>] [--mark-read]`
- `awiki-cli msg history --with <target> [--limit <n>] [--cursor <cursor>]`
- `awiki-cli msg mark-read <MESSAGE_ID...>`

## 常见模式

### 先 dry-run 再发送私聊

1. `awiki-cli msg send --to alice --text "Hello" --dry-run`
2. `awiki-cli msg send --to alice --text "Hello"`

### 先检查成员关系，再向群组发送

1. `awiki-cli group get --group <group_did>`
2. `awiki-cli msg send --group <group_did> --text "Hello group" --dry-run`
3. `awiki-cli msg send --group <group_did> --text "Hello group"`

### 发送一个附件

1. `awiki-cli msg send --to alice --file ./hello.txt --text "hello attachment" --dry-run`
2. `awiki-cli msg send --to alice --file ./hello.txt --text "hello attachment"`

### 先定位消息，再下载单个附件

1. `awiki-cli msg history --with alice --limit 50`
2. `awiki-cli msg attachment download --with alice --message-id <message_id> --output ./downloads/file.bin --dry-run`
3. `awiki-cli msg attachment download --with alice --message-id <message_id> --output ./downloads/file.bin`

### 只读取未读私聊项

`awiki-cli msg inbox --scope direct --unread --limit 20`

## 副作用与确认

- 需要显式确认：
  - `msg send`
  - `msg attachment download`
  - `msg mark-read`
  - `msg inbox --mark-read`
- 发送消息或下载附件前，优先使用 `--dry-run`

## 错误处理

- target 或 body 不清楚 -> 检查 `awiki-cli schema msg send`
- 附件下载命令形状不清楚 -> 检查 `awiki-cli schema msg attachment download`
- auth/setup 错误 -> 确认活动身份已完成注册
- transport unavailable -> 使用 `05-runtime.md`
- 请求 secure 但当前不支持 -> 说明该 secure 路径在当前仓库中仍处于规划阶段

## 实现说明

- runtime mode 由 runtime 领域决定，而不是由消息命令决定
- `msg send` 同时覆盖文本发送与附件发送；附件发送使用 `--file`，并可选附带 `--text` 作为 caption
- `msg secure` 子命令已保留，但尚未实现
- 不要把当前仓库状态描述成已支持端到端 secure direct messaging

## 相关参考

- `04-groups.md`
- `05-runtime.md`
- `01-onboarding.md`
