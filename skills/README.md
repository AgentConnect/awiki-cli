# awiki Skills

当前仓库采用两层 Skill 结构：**单入口 + references**。

## 结构

- `SKILL.md`：唯一默认入口
- `references/`：领域、workflow 与低频 reference 文档
- `manifests/skills.yaml`：结构化索引，便于维护、路由审阅与工具消费

当前 reference 顺序：

- `references/00-installation.md`
- `references/01-onboarding.md`
- `references/02-identity.md`
- `references/03-messaging.md`
- `references/04-groups.md`
- `references/05-runtime.md`
- `references/06-pages.md`
- `references/07-discovery.md`
- `references/08-debug.md`
- `references/09-people-planned.md`

## Current Source of Truth

Skill 维护当前遵循三类真相源：

1. `internal/cmdmeta/catalog.go`：命令是否存在、flag 形状、implemented/partial/planned 的实现事实
2. `skills/SKILL.md` 与 `skills/references/*.md`：实际入口文档、路由规则、加载策略与用户可读说明
3. `skills/manifests/skills.yaml`：结构化索引与维护辅助，不参与 CLI 运行时执行

如果它们出现不一致：

- **命令存在性、flag 与实现状态** 以 `cmdmeta` 为准
- **入口加载策略与 reference 路由** 以 `SKILL.md` 和实际 references 为准
- `skills.yaml` 应同步修正，不应覆盖前两者

## Maintenance Rules

- 所有命令示例统一使用 `awiki-cli`
- 默认只加载 `SKILL.md`
- 单领域任务只补读一个匹配的 reference
- workflow 任务优先进入 `00-installation.md`、`01-onboarding.md` 或 `07-discovery.md`
- `08-debug.md` 仅作为最后兜底 reference
- `09-people-planned.md` 只用于说明 future contract，不能描述成已实现能力
- 文档中不要发明当前仓库里不存在的命令、flag 或输出字段
