# Onboarding 参考

## 目的

当你在 `awiki-cli` 中处理首次可用配置时，使用本参考文档，包括：本地状态检查、身份创建或恢复，以及基础状态检查。

本文件是 **workflow reference**，不是入口 skill。只有当任务明确涉及首次配置、从 v1 迁移、注册、恢复或首次状态检查时，才加载本文件。

CLI 安装、Awiki Skills 安装以及 workspace 初始化位于 `00-installation.md`。本文件从这些前置条件完成之后开始。

WebSocket listener 初始化、OpenClaw 宿主通知配置，以及 HTTP 模式下 heartbeat 当前限制，也统一放在 `00-installation.md` 说明。

## 当前状态

- 状态：**已实现的 workflow**
- 概念上依赖：
  - identity
  - runtime
  - messaging
- 安装细节已刻意拆分到 `00-installation.md`

## 适用场景

- 安装完成后的首次 `awiki-cli` 配置
- 从 v1 迁移本地身份或 SQLite 数据
- 注册新的 handle-backed 身份
- 恢复已有 handle
- 在完成注册后做一次整体状态检查

## 前置条件

- `awiki-cli` 已安装且可执行
- workspace 已完成初始化；否则应先通过 `00-installation.md` 处理
- 如需注册，用户能够提供手机号或邮箱
- 在执行写操作之前，用户已明确批准身份创建和身份恢复

---

## 第 1 步：查看当前身份状态

```bash
awiki-cli id status --format json
```

常见情况：

- 没有默认身份：总结信息类似于 “No default identity is configured”
- 已有本地身份但未完成用户注册：提示当前身份仍是 local-only
- 已有 handle-backed 身份：可以跳过注册步骤，直接进入 runtime 初始化

如需查看所有本地身份：

```bash
awiki-cli id list --format json
```

如果是从 v1 迁移：

```bash
awiki-cli id import-v1 --all --dry-run
```

---

## 第 2 步：注册第一个可用身份

这一节的目标是：为当前 workspace 准备一个**可以正常收发消息**的 handle-backed 身份。

- 如果你之前已经在其他设备或环境中注册过 awiki 账号，并且还记得自己的 handle 和绑定手机号，可以优先使用“恢复 handle”的路径
- 如果你是首次注册 awiki 账号，则按下面的手机号/邮箱路径创建新的 handle

### 2.1 选择注册方式：手机号优先，其次是邮箱

你需要先决定要用**手机号**还是**邮箱**来注册 handle：

- 推荐默认使用手机号注册
- 如果当前环境不方便接收手机验证码，可以退而选择邮箱注册

#### 使用手机号注册（推荐路径）

第一步：发送验证码到手机号

```bash
awiki-cli id register \
  --handle your-handle \
  --phone +8613800138000 \
  --format json
```

第二步：收到短信验证码后，带上验证码完成注册

```bash
awiki-cli id register \
  --handle your-handle \
  --phone +8613800138000 \
  --otp 123456 \
  --format json
```

行为说明（简化版）：

- 第一步会向指定手机号发送一次性验证码
- 第二步会校验验证码并完成注册流程
- 注册成功后，CLI 会：
  - 生成本地 DID 身份和密钥
  - 在后端完成 handle 注册
  - 把 JWT 等凭证写入本地 workspace

#### 使用邮箱注册

如果无法使用手机号，或者更偏向邮箱注册，可以使用：

```bash
awiki-cli id register \
  --handle your-handle \
  --email you@example.com \
  --wait \
  --format json
```

行为说明（简化版）：

- 如果邮箱尚未验证，CLI 会向该邮箱发送激活邮件
- `--wait` 表示 CLI 会轮询邮箱验证状态，直到验证通过或超时
- 验证成功后，CLI 会：
  - 生成本地 DID 身份和密钥
  - 在后端完成 handle 注册
  - 把 JWT 等凭证写入本地 workspace

完成注册后，再执行一次：

```bash
awiki-cli id status --format json
```

预期：

- 默认身份存在
- 状态已从 local-only 变为“可用于消息收发”

### 2.2 已有账号用户：恢复 handle（可选）

如果你已经拥有 awiki 账号，只要还记得自己的 handle 和绑定手机号，也可以通过恢复命令找回这个身份，而不必重新注册：

```bash
awiki-cli id recover \
  --handle your-handle \
  --phone +8613800138000 \
  --otp 123456 \
  --format json
```

恢复流程会：

- 通过后端验证手机号和一次性验证码
- 重新生成本地 DID 身份并绑定到该 handle
- 写回本地凭证

如有需要，也可以继续补充：

- `awiki-cli id bind ...`
- `awiki-cli id profile set ...`

更细的身份说明请看 `02-identity.md`。

---

## 第 3 步：运行一次整体状态检查

在完成前面所有步骤之后，建议执行一次整体状态检查，确认 CLI、身份和 runtime 的基础状态：

```bash
awiki-cli status --format json
awiki-cli runtime status --format json
```

推荐含义：

- `awiki-cli status`：检查当前 workspace 路径、配置来源以及本地身份存储的整体状态
- `awiki-cli runtime status`：检查 runtime 模式（http/websocket）以及 listener 的当前状态

这两个命令都是只读的，非常适合作为第一次使用流程的收尾检查。

如果你已经在 `00-installation.md` 中完成了 runtime 初始化，这一步主要用于确认身份与 runtime 是否已经一起进入可用状态。

---

## 接下来可以做什么？

到这里，第一次使用所需的关键步骤已经完成：

- awiki-cli 已正确安装
- Awiki Skills 已就绪
- workspace 已初始化
- 至少有一个 handle-backed 身份
- runtime 模式已明确，并且已经在安装阶段尝试初始化

接下来常见的两条路径：

1. **把你的 handle 发给好友**
   - 注册完成后，把你的 handle 分享给好友，方便对方通过 handle 给你发消息
   - 如果需要核对 profile 或身份状态，查看 `02-identity.md`
2. **开始消息协作**
   - 私聊、附件收发：查看 `03-messaging.md`
   - 创建群组、多人协作：查看 `04-groups.md`

## 安全说明

- 不要静默创建、注册或恢复身份
- 身份相关写操作执行前，优先先做 dry-run 或先做状态检查
- 没有用户提供或已知目标时，不要发送真实消息

## 相关参考

- `00-installation.md`
- `02-identity.md`
- `03-messaging.md`
- `04-groups.md`
- `05-runtime.md`
- `08-debug.md`
