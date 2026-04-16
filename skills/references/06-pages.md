# 页面参考

## 目的

当你在 `awiki-cli` 中处理内容页生命周期任务时，使用本参考文档，包括：创建页面、列出页面、读取页面、更新页面、重命名 slug，以及删除页面。

本文件是 **reference**，不是入口 skill。只有当任务明确涉及内容页、slug、markdown 发布或可见性变更时，才加载本文件。

## 当前状态

- 状态：**已实现**

## 适用场景

- 创建内容页
- 列出或读取页面
- 更新 markdown 或可见性
- 重命名或删除页面 slug

## 核心概念

- **slug**：CLI 契约中的页面标识符
- **title**：页面显示标题
- **markdown body**：来自 `--markdown` 或 `--markdown-file` 的页面内容
- **visibility**：`public`、`draft` 或 `unlisted`

## 决策规则

- 需要新建页面 -> `page create`
- 需要列表视图 -> `page list`
- 需要查看单页 -> `page get`
- 需要修改正文或可见性 -> `page update`
- 需要修改 slug -> `page rename`
- 需要删除页面 -> `page delete`

## Canonical 命令

- `awiki-cli page create --slug <slug> --title <title> [--markdown ... | --markdown-file ...] [--visibility public|draft|unlisted]`
- `awiki-cli page list`
- `awiki-cli page get --slug <slug>`
- `awiki-cli page update --slug <slug> [--title ...] [--markdown ... | --markdown-file ...] [--visibility ...]`
- `awiki-cli page rename --slug <slug> --to <new_slug>`
- `awiki-cli page delete --slug <slug>`

## 常见模式

### 先 dry-run，再从文件创建

1. `awiki-cli page create --slug hiring --title "Hiring" --markdown-file ./hiring.md --dry-run`
2. `awiki-cli page create --slug hiring --title "Hiring" --markdown-file ./hiring.md`

### 只更新可见性

`awiki-cli page update --slug hiring --visibility draft`

## 副作用与确认

- 需要显式确认：
  - `page create`
  - `page update`
  - `page rename`
  - `page delete`

## 错误处理

- slug 或正文不清楚 -> 检查 `awiki-cli schema page create` 或 `page update`
- identity/auth 问题 -> 确认当前活动身份与注册状态
- markdown 文件路径问题 -> 重试前先确认文件可读

## 实现说明

- 正文来源只能二选一：内联 markdown 或 markdown 文件
- 示例保持 slug-first，避免使用服务内部标识符

## 相关参考

- `02-identity.md`
- `08-debug.md`
