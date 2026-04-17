# awiki-cli 发布、安装与回滚手册

本文档描述 awiki-cli 当前正式采用的发布模型：

- **GitHub Release / artifact**：发布跨平台 binary 产物
- **awiki update metadata**：发布运行期版本真相
- **npm 包 `@awiki/cli`**：只提供 bootstrap 安装入口
- **公开 root skill URL**：发布正式版本的 root `SKILL.md`

本文档不再把 npm registry 视为运行期版本策略真相源。运行期版本检查、最小支持版本策略、以及平台 artifact 选择，统一以 **awiki update API** 为准。

---

## 1. 版本输入与 Tag 约定

### 1.1 版本输入

发布输入仍然来自仓库根目录的：

- `package.json.version`

约定：

- stable：`X.Y.Z`
- prerelease：`X.Y.Z-<pre>`，例如 `0.2.0-beta.1`

### 1.2 Git Tag 规则

- 正式版：`vX.Y.Z`
- 预发布版：`vX.Y.Z-<pre>`

### 1.3 Go 构建版本

GoReleaser 继续通过 `.goreleaser.yml` 把 tag 版本注入：

- `internal/buildinfo.Version`
- `internal/buildinfo.Commit`
- `internal/buildinfo.BuildDate`

### 1.4 重要裁决

- `package.json.version` 仍是发布输入真相源之一
- **npm registry 不是运行期版本真相源**
- 运行期版本真相来自 awiki update metadata / update API

---

## 2. 发布产物模型

每次正式 release 需要产出四类 canonical 产物：

1. **binary artifact**
2. **update metadata**
3. **public root skill**
4. **npm bootstrap 包**

### 2.1 Binary artifact

由 GoReleaser 生成并上传到 GitHub Release。

当前格式：

- Unix：`tar.gz`
- Windows：`zip`

当前命名：

- `awiki-cli-<version>-<os>-<arch>.<ext>`

同时生成：

- `awiki-cli-<version>-checksums.txt`

### 2.2 Update metadata

作为运行期版本策略真相，canonical 文件为：

- `dist/update-metadata.json`

至少包含：

- `schema_version`
- `channel`
- `latest_version`
- `min_supported_version`
- `published_at`
- `artifacts[]`
- `skill_bundle`

该文件由：

- `scripts/release/generate-update-metadata.js`

生成，并作为 release asset 上传。

### 2.3 Public root skill

canonical 文件为：

- `dist/public-skill/awiki-cli/SKILL.md`

该文件必须来自当前 release binary 的渲染结果，而不是手工维护的第二份文本。当前通过：

- `scripts/release/render-public-skill.sh`

调用 release binary 执行 `awiki-cli skill export --all --include-root --clean` 生成。

### 2.4 npm bootstrap 包

npm 包名：

- `@awiki/cli`

职责固定为：

- 下载 bootstrap binary 到 npm 包内的 `bin/`
- 通过 `scripts/run.js` 包装启动

职责边界：

- **不负责运行期 update check**
- **不负责运行期 self-update**
- 运行期升级由 `awiki-cli upgrade apply` 负责

---

## 3. 发布流程

### 3.1 正式 stable 发布

前置要求：

1. 当前分支工作区干净
2. 当前分支已推到远端
3. `package.json.version` 为稳定 semver
4. npm 凭据可用（若要发布 bootstrap 包）

执行：

```bash
scripts/release/tag-release.sh
```

该脚本会：

- 从 `package.json.version` 生成 `vX.Y.Z`
- 校验当前分支已 push
- 创建并推送 annotated tag

### 3.2 CI / GitHub Actions 行为

当前 tag workflow 需要完成这些步骤：

1. Run GoReleaser
2. Render public skill package
3. Generate update metadata
4. Verify canonical release assets
5. Upload canonical release assets to GitHub Release
6. stable tag 时执行 npm publish

canonical release assets 至少包括：

- `dist/update-metadata.json`
- `dist/public-skill/awiki-cli/SKILL.md`
- `dist/public-skill/skill-manifest.json`

### 3.3 外部部署系统职责

本 repo 的 release workflow 只负责**生成并上传 canonical 产物**。后续由外部部署系统消费：

- `dist/update-metadata.json`
- `dist/public-skill/awiki-cli/SKILL.md`

并将其发布到：

- awiki update metadata 服务
- `https://awiki.ai/skills/awiki-cli/SKILL.md`

重要约束：

- update metadata 只有在 artifact 已可下载后才能发布
- root skill URL 只有在 root skill 渲染产物生成成功后才能切换

---

## 4. 预发布（beta / rc）

### 4.1 版本调整

将 `package.json.version` 修改为带预发布后缀的版本，例如：

- `0.2.0-beta.1`
- `0.2.0-rc.1`

### 4.2 创建 Tag

运行：

```bash
scripts/release/release-prerelease.sh <dist-tag>
```

示例：

```bash
scripts/release/release-prerelease.sh beta
```

### 4.3 预发布输出要求

预发布同样必须生成：

- GoReleaser artifact
- `dist/update-metadata.json`
- `dist/public-skill/awiki-cli/SKILL.md`

区别在于：

- channel 为 `prerelease`
- npm 发布使用指定 dist-tag
- 外部部署系统可选择是否把 prerelease metadata 暴露给生产 update API

---

## 5. 安装模型

### 5.1 npm bootstrap 安装

bootstrap 安装通过：

- `npm install @awiki/cli`
- `npm install -g @awiki/cli`

触发：

- `scripts/install.js`

现在的 bootstrap installer 行为是：

1. 计算 channel（stable / prerelease）
2. 调用 awiki update API
3. 获取当前平台 artifact URL
4. 下载并解压到 npm 包内 `bin/`

这意味着：

- npm bootstrap 安装与运行期 CLI 自升级使用**同一套 artifact 事实源**
- 不再依赖本地拼接 GitHub Release URL

### 5.2 运行期升级

运行期升级统一通过 CLI 本身完成：

```bash
awiki-cli upgrade check
awiki-cli upgrade apply
awiki-cli upgrade status
```

其版本策略统一来自 update API，不依赖 npm registry。

---

## 6. root skill URL 发布模型

公开 root skill URL：

```text
https://awiki.ai/skills/awiki-cli/SKILL.md
```

要求：

- URL 版 root skill 必须来自当前 release binary 的同一份渲染结果
- 不允许手工维护第二份 URL 专用 `SKILL.md`
- 该 URL 作为 Agent bootstrap 入口使用
- root skill 内容中必须包含：
  - `npm install -g @awiki/cli`
  - `awiki-cli version`
  - `awiki-cli skill index --json`
  - `awiki-cli skill get <path>`
  - `awiki-cli schema <resource>.<method>`

---

## 7. 回滚模型

出现 bad version 时，回滚必须同时考虑四层：

### 7.1 artifact 层

处理 GitHub Release / release assets / 下载产物。

### 7.2 update metadata 层

处理：

- latest 回滚
- minSupportedVersion 调整
- 某平台 artifact 下线

### 7.3 npm bootstrap 层

处理：

- `npm deprecate`
- `npm dist-tag` 调整

### 7.4 root skill URL 层

将：

- `https://awiki.ai/skills/awiki-cli/SKILL.md`

回滚到上一个稳定版本内容。

### 7.5 推荐回滚顺序

1. 停止 update metadata 将 bad version 作为 latest 返回
2. 必要时提高 `min_supported_version`
3. 处理 GitHub Release / artifact 可见性
4. 调整 npm bootstrap 包的 deprecate / dist-tag
5. 回滚 root skill URL
6. 重新做 smoke check

---

## 8. 运维 smoke check

每次发布完成后至少执行：

1. 检查 update metadata 是否正确
2. 检查 artifact URL 是否可下载
3. 检查 sha256 是否匹配
4. 执行 npm bootstrap 安装
5. 执行：

   ```bash
   awiki-cli upgrade check
   ```

6. 检查 root skill URL：

   ```bash
   curl https://awiki.ai/skills/awiki-cli/SKILL.md
   ```

7. 确认 root skill 内容与当前 release 产物一致

---

## 9. 与运行期版本策略的关系

awiki-cli 运行期版本策略由：

- `internal/update`
- update API
- `update.*` config

共同决定。

其中：

- `latest_version`
- `min_supported_version`
- `artifact`
- `skill_bundle`

都来自 awiki update metadata。

**npm 不再参与运行期版本策略。**

---

## 10. 当前脚本与工作流清单

当前 Phase 5 需要涉及：

- `scripts/install.js`
- `scripts/run.js`
- `scripts/release/tag-release.sh`
- `scripts/release/release-prerelease.sh`
- `scripts/release/withdraw-release.sh`
- `scripts/release/generate-update-metadata.js`
- `scripts/release/render-public-skill.sh`
- `scripts/release/verify-release-assets.sh`
- `.github/workflows/release.yml`

---

## 11. 验收标准

Phase 5 完成时必须满足：

- bootstrap install 与 runtime self-update 使用同一套 artifact 事实源
- update API 是唯一运行期版本真相源
- npm 只承担 bootstrap installer 角色
- root skill URL 已进入正式发布/回滚链路
- `docs/publish.md` 已完全反映新体系
- 回滚 runbook 已覆盖 artifact / metadata / npm / root skill URL 四层
