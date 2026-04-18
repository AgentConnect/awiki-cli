# 身份参考

## 目的

当你在 `awiki-cli` 中处理身份生命周期任务时，使用本参考文档，包括：本地身份检查、带 handle 的注册、恢复、联系方式绑定、身份切换，以及 profile 管理。

本文件是 **reference**，不是入口 skill。只有当任务明确涉及 identity、DID、handle、联系方式绑定、恢复或 profile 数据时，才加载本文件。

## 当前状态

- 状态：**已实现**
- 当前公开二进制：`awiki-cli`
- 存在隐藏命令：`id create`
- 存在危险公开命令：`id replace-did`
- 对外说明保持 **handle 优先**

## 适用场景

- 创建或导入本地身份
- 注册或恢复带 handle 的身份
- 绑定手机号或邮箱联系方式
- 切换默认身份
- 读取或更新 DID profile
- 谨慎替换某个本地 identity 对应的协议级 DID

## 核心概念

- **identity**：通过 `--identity` 选择的本地 awiki 身份容器
- **DID**：服务端使用的协议级标识符
- **handle**：人类可读的公开标识符
- **contact binding**：为现有身份增加手机号或邮箱
- **current identity**：在省略 `--identity` 时使用的默认本地身份

## 生命周期

`status -> create/register/import -> bind -> profile set -> current/use`

## 决策规则

- 还没有本地身份 -> 优先使用 `awiki-cli id register ...`；只有在 bootstrap、迁移或 debug 时才使用隐藏命令 `id create`
- 已有本地身份，但还没有 handle-backed 用户状态 -> 使用 `awiki-cli id register ...`
- 已有 handle，但联系方式不完整 -> 使用 `awiki-cli id bind ...`
- handle 丢失，但仍有恢复手机号 -> 使用 `awiki-cli id recover ...`
- 需要查看多个本地身份 -> 使用 `awiki-cli id list`
- 需要切换默认身份 -> 使用 `awiki-cli id use <identity>`
- 需要查看公开 profile 数据 -> 使用 `awiki-cli id profile get ...`
- 只有在明确需要轮换/替换某个 handle identity 的 DID 时，才使用 `awiki-cli --identity <identity> id replace-did`；必须先 dry-run 并确认目标 identity

## Canonical 命令

- `awiki-cli id status`
- `awiki-cli id list`
- `awiki-cli id current`
- `awiki-cli id use <identity>`
- `awiki-cli id register --handle <handle> (--phone <phone> [--otp <code>] | --email <email> [--wait])`
- `awiki-cli id bind (--phone <phone> [--otp <code>] | --email <email> [--wait])`
- `awiki-cli id resolve (--handle <handle> | --did <did>)`
- `awiki-cli id recover --handle <handle> --phone <phone> --otp <code>`
- `awiki-cli --identity <identity> id replace-did [--is-public] [--is-agent] [--role <role>] [--endpoint-url <url>]`
- `awiki-cli id profile get [--self | --handle <handle> | --did <did>]`
- `awiki-cli id profile set [--display-name ...] [--bio ...] [--tags ...] [--markdown ...] [--markdown-file ...]`
- `awiki-cli id import-v1 [--name <identity> | --all]`

## 常见模式

### 推荐的注册流程

1. `awiki-cli id status`
2. `awiki-cli id register --handle alice --phone +8613800138000 --otp 123456`
3. `awiki-cli id current`
4. `awiki-cli id bind --email alice@example.com --wait`
5. `awiki-cli id profile set --display-name "Alice"`

### 从 v1 导入后再切换

1. `awiki-cli id import-v1 --all --dry-run`
2. `awiki-cli id import-v1 --all`
3. `awiki-cli id list`
4. `awiki-cli id use <identity>`

### 危险：替换 DID

`id replace-did` 会为指定 identity 生成新的 e1 DID 和新密钥材料，并用远端 `did-auth.replace_did` 把旧 DID 替换掉。该操作会先把旧 DID document、旧私钥和旧 identity 目录备份到本地 `.legacy-backup/replace-did/`，然后更新本地 identity store、DID document、私钥文件，并重绑本地 SQLite 的 `owner_did`；使用错误目标可能造成身份、消息历史或通知路由混乱。

`.legacy-backup/replace-did/` 中的内容仍然包含旧私钥和旧 JWT 等敏感材料，不要上传、粘贴或分享。

仅在用户明确要求替换 DID 时使用：

1. `awiki-cli id list`
2. `awiki-cli --identity <identity> id replace-did --dry-run`
3. 人类确认目标 identity、旧 DID、影响范围后，再执行 `awiki-cli --identity <identity> id replace-did`

不要在普通注册、恢复、profile 更新或消息任务中主动使用该命令。

## 副作用与确认

- 需要显式确认：
  - `id register`
  - `id bind`
  - `id recover`
  - `id use`
  - `id profile set`
  - `id import-v1`
  - 危险命令 `id replace-did`
  - 隐藏命令 `id create`
- 写操作支持时，优先使用 `--dry-run`
- `id replace-did` 必须把 `--identity <identity>` 视为目标用户选择方式；省略时会作用于默认 identity，因此更容易误操作

## 错误处理

- register 或 bind 的命令形状不清楚 -> 检查 `awiki-cli schema id register` 或 `awiki-cli schema id bind`
- auth 或 token 状态不清楚 -> 恢复或重新注册身份
- 缺少身份 -> 使用 `awiki-cli id list` 和 `awiki-cli id current`
- 本地 store 状态不清楚 -> 使用 `awiki-cli doctor`

## 实现说明

- `id create` 是有意隐藏的
- `id replace-did` 是公开但危险的维护命令；它只适用于 handle-backed DID，会生成新的 e1 DID 来替代旧 DID
- 对外说明应保持 handle 优先
- 本 reference 的公开契约中不包含 `user_id`

## 相关参考

- `01-onboarding.md`
- `08-debug.md`
- `00-installation.md`
