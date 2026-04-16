# People 规划参考

## 目的

本参考文档只用于说明 `awiki-cli` 中未来 people 与关系能力的当前契约边界。

本文件是 **planned appendix**，不是常规操作 reference。只有当用户询问 people、follower、following 或本地联系人能力是否已经存在时，才加载本文件。

## 当前状态

- 状态：**计划中**
- 当前仓库尚未实现对应命令 handler

不要把这些命令描述成可工作的现有功能。

## 未来规划范围

- people search
- follow / unfollow
- relationship status
- followers / following
- 本地 contacts 列表与保存

## 已规划命令契约

- `awiki-cli people search <QUERY>`
- `awiki-cli people follow <TARGET>`
- `awiki-cli people unfollow <TARGET>`
- `awiki-cli people status <TARGET>`
- `awiki-cli people followers`
- `awiki-cli people following`
- `awiki-cli people contacts list`
- `awiki-cli people contacts save --did <did> [--handle <handle>] [--reason <text>]`

## 使用指导

- 如果用户今天需要关系 discovery，请使用 `07-discovery.md`
- 如果用户今天需要真实消息历史或群组检查，请使用 `03-messaging.md` 或 `04-groups.md`
- 如果用户询问 `people` 是否可用，应回答：契约已保留，但当前尚未实现

## 未来的确认规则

如果这些命令未来实现，下列命令需要显式确认，因为它们会变更关系状态或本地联系人状态：

- `people follow`
- `people unfollow`
- `people contacts save`

## 相关参考

- `07-discovery.md`
- `03-messaging.md`
- `04-groups.md`
