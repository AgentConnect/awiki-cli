# Debug 参考

## 目的

当你在 `awiki-cli` 中进行本地调试和最后兜底检查时，使用本参考文档，尤其适用于：SQLite 状态检查、迁移导入核验、schema 混淆，以及常规领域路径已不足以解释问题的场景。

本文件是 **reference**，不是入口 skill。只有在安全检查路径都已用尽后，才加载本文件。

## 当前状态

- 状态：**部分实现**
- 当前已实现：
  - `debug db query`
  - `debug db import-v1`
- 已规划但尚未实现：
  - `debug raw rpc`
  - `debug schema-cache`
  - `debug logs`

## 适用场景

- 本地 SQLite 检查
- 核验迁移导入结果
- 了解当前 debug 面实际提供了什么
- 将底层发现重新映射回领域行为

## 安全优先决策树

只有在以下路径仍然不够时，才使用 debug：

1. `awiki-cli status`
2. `awiki-cli docs [topic]`
3. `awiki-cli schema [command]`
4. `awiki-cli doctor`
5. `awiki-cli config show`
6. 一个匹配的领域或 workflow reference

## 当前可用命令

- `awiki-cli debug db query "<SQL>"`
- `awiki-cli debug db import-v1 [--path <legacy_dir>]`

## 已规划但尚未实现

- `awiki-cli debug raw rpc`
- `awiki-cli debug schema-cache`
- `awiki-cli debug logs [--follow]`

## 限制

- 不要执行 destructive SQL
- 在命令尚未实现前，不要假定 raw RPC 已可用
- 不要暴露 JWT、私钥、secure session material 或无关的本地文件
- 不要使用 debug 绕过领域级确认规则

## 副作用与确认

- 对窄范围、非破坏性检查来说是安全的：
  - `debug db query`
- 需要显式确认，且应先 dry-run：
  - `debug db import-v1`

## 恢复模式

1. 使用 `debug db query` 进行检查
2. 将发现重新翻译回 canonical 的 runtime、identity 或 messaging 行为
3. 返回对应领域 reference，避免长时间停留在 debug 路径

## 相关参考

- `02-identity.md`
- `03-messaging.md`
- `04-groups.md`
- `05-runtime.md`
- `01-onboarding.md`
