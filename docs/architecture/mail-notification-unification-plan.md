# Mail Notification Unification Plan

## 背景

当前 `awiki-cli` 中“普通私信通知”和“邮件通知”已经能够分别工作，但邮件链路仍然保留了几处独立分支：

- websocket 收到 `mail.notification` 后，会被单独标准化成 `mail.message.received`
- `msg inbox` 会把本地 `mail.notification` 与普通 inbox 结果做合并展示
- OpenClaw / Hermes 下游为了展示邮件，又各自兼容一套 mail 语义

这导致需求本身虽然简单，但实现上存在多处历史分叉，排障时必须跨 listener、inbox、host-notify 和 Hermes route 一起检查。

## 目标

在 **不影响其他平台**、**不影响已经正常工作的私信通知** 的前提下，把邮件通知逐步收口为“普通消息通知”的一个入口适配。

最终希望达到的结构是：

```text
mail.notification -> message-style notification -> websocket/inbox/webhook/Hermes/OpenClaw
```

## 约束

本次重构必须满足以下约束：

1. 不能重写或替换现有普通私信通知主链路。
2. 不能破坏 OpenClaw、Hermes 等其他平台已经正常工作的通知能力。
3. 优先做“加字段、加兼容”的收口，不做“大替换”。
4. 任何阶段都必须能通过回归验证：
   - 普通私信 -> CLI 实时通知正常
   - 普通私信 -> Hermes / OpenClaw 正常
   - 邮件 -> CLI / Hermes 能识别为邮件

## 分阶段方案

### 阶段 1：统一 host-notify 事件语义

目标：

- 邮件不再向下游暴露独立的 `mail.message.received` 主语义
- 邮件改为复用 `im.message.received` 这条已经正常工作的私信通知链路
- 同时保留邮件字段，避免 Hermes / OpenClaw 丢失展示信息

实施方式：

- `listener/host_notify` 中，将邮件标准化为 message-style payload
- 新增 `source_kind=mail`
- 保留邮件字段：
  - `mailbox_address`
  - `from_addr`
  - `subject`
  - `preview`
  - `has_attachments`
- 普通私信显式写 `source_kind=im`
- OpenClaw / Hermes 优先根据 `source_kind=mail` 或邮件字段识别邮件，而不是依赖独立 topic

阶段 1 的结果是：

- 下游主 topic 统一
- 普通私信链路不变
- 邮件仍然能被识别和正确展示

### 阶段 2：统一 inbox 读取模型

目标：

- `msg inbox` 最终只展示一种“通知消息”视图
- 尽量减少 CLI 展示层的 mail 特判和合并逻辑

实施方式：

- 优先在 awiki 自己控制的本地存储读取层做统一
- 展示层只根据 `source_kind=mail` 或 metadata 决定标题前缀 `[邮件]`
- 尽量避免继续扩大 `mail.notification` 专属逻辑

注意：

这一阶段可以晚于阶段 1 执行，因为 inbox 是本地视图问题，不阻塞 Hermes / OpenClaw 通知收口。

### 阶段 3：清理历史 mail 专属分支

目标：

- 删除不再必要的 mail 独立分支
- 将“邮件通知 = 消息通知 + 邮件来源标识”固化为长期结构

候选清理项：

- `mail.message.received` 的旧兼容处理
- 下游只为 mail 额外存在的桥接或格式化分支
- `msg inbox` 中仅用于过渡期的 mail 合并逻辑

## 当前实施范围

本次代码变更只启动 **阶段 1**：

- 增加统一语义文档
- 修改 `listener/host_notify` 的邮件标准化输出
- 修改 OpenClaw 对新 payload 的兼容格式化
- 保留现有普通私信行为和旧 mail payload 兼容能力

## 验收标准

至少需要满足以下回归项：

1. `direct.incoming` 仍然标准化为 `im.message.received`
2. `mail.notification` 现在也进入 `im.message.received`
3. 邮件事件中仍然保留 `mailbox_address` / `from_addr` / `subject` / `preview`
4. OpenClaw 对新邮件 payload 仍能输出“邮件格式”文本
5. Hermes host-notify 构建和已有测试不回归
