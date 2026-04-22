# 升级参考

## 目的

当你在 `awiki-cli` 中处理版本升级任务时，使用本参考文档，包括：升级 CLI、刷新 Awiki Skills，以及在需要时重启 listener。

本文件是 **reference**，不是入口 skill。只有当任务明确涉及 upgrade、update、npm 升级、版本过旧或 skill 刷新时，才加载本文件。

## 适用场景

- 用户要升级 `awiki-cli`
- CLI 提示有新版本可用
- CLI 提示当前版本低于最小支持版本
- 用户要刷新当前 Agent 中的 Awiki Skills

## 升级 `awiki-cli`

`awiki-cli upgrade` 会先检查版本；当存在新版本或当前版本低于最小支持版本时，会直接执行：

```bash
npm install -g @awiki/cli@latest
```

推荐路径：

```bash
awiki-cli upgrade
```

如果你希望直接执行 npm 全局升级，也可以使用：

```bash
npm install -g @awiki/cli@latest
```

升级完成后，新开一个 shell，再执行：

```bash
awiki-cli version
```

## 刷新 Awiki Skills

升级 CLI 不会自动刷新当前 Agent 中已安装的 Awiki Skills。要刷新 skill，请重新执行安装命令。

如果当前环境可以稳定访问 GitHub：

```bash
npx skills add https://github.com/AgentConnect/awiki-cli.git --agent <你的-agent-id> -y -g
```

如果你在中国大陆环境中安装，推荐优先使用 Gitee：

```bash
npx skills add https://gitee.com/agentconnect/awiki-cli.git --agent <你的-agent-id> -y -g
```

如果你不确定 `--agent` 的值，回到 `00-installation.md` 查表。

## 验证

先确认当前版本：

```bash
awiki-cli version
```

如果当前使用的是 websocket listener，再执行：

```bash
awiki-cli runtime listener restart
awiki-cli runtime listener status
```

## 相关参考

- `00-installation.md`
- `05-runtime.md`

