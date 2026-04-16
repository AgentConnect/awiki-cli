# Discovery 参考

## 目的

当你在 `awiki-cli` 中处理“先审阅、再起草”的 workflow 时，使用本参考文档，尤其适用于：用户希望检查某个群组、理解可能相关的对象、回顾既有关系上下文，并起草介绍或跟进消息。

本文件是 **workflow reference**，不是入口 skill。只有当任务明确涉及 discovery、群组审阅、候选人选择或手工起草介绍时，才加载本文件。

## 当前状态

- 状态：**部分实现的 workflow**
- 当前可用：
  - 群组检查
  - 成员审阅
  - 私聊历史审阅
  - profile 查询
- 后续规划：
  - `people` 搜索、关注、联系人与关系管理

## 适用场景

- 在联系群成员之前先审阅群组
- 为手工起草介绍或跟进消息收集上下文
- 理解当前群组活动与可能相关的对象

## 前置条件

- 用户提供目标 group DID，或提供一小组候选对象
- 当前活动身份可以读取相关群组或私聊历史
- 当前 workflow 用于审阅和起草，而不是自动触达

## Workflow 步骤

### 1. 检查群组本身

- `awiki-cli group get --group <group_did>`

### 2. 审阅当前成员

- `awiki-cli group members --group <group_did> --limit 100`

### 3. 审阅近期群组活动

- `awiki-cli group messages --group <group_did> --limit 50`

### 4. 查看某个候选对象的 profile

- `awiki-cli id profile get --did <member_did>`
- 或 `awiki-cli id profile get --handle <handle>`

### 5. 如果已有关系，则查看既有私聊历史

- `awiki-cli msg history --with <handle|did> --limit 50`

### 6. 手工起草触达内容

在收集完结构化输出后，在助手回复中起草介绍或私聊消息。

不要自动发送消息。如果用户要求发送，则切换到 `03-messaging.md`，并优先使用 dry-run。

## 已规划的后续能力

以下命令族已保留，但尚未实现：

- `awiki-cli people search <QUERY>`
- `awiki-cli people contacts save --did <did> [...]`

当用户请求这些操作时，应说明契约已存在，但当前仓库尚未实现对应 handler。

## 安全说明

- 先审阅，后发送
- 不要自动 follow、自动保存联系人或自动给任何人发消息
- 不要从群组活动中推断敏感个人特征

## 相关参考

- `02-identity.md`
- `03-messaging.md`
- `04-groups.md`
- `09-people-planned.md`
