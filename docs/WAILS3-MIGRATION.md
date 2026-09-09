# Wails 3 迁移记录

## 当前实现

- Wails CLI/Go 依赖固定为 `v3.0.0-beta.18`；这是预发布版本，不能当作稳定版描述。
- `apps/desktop/main.go` 使用 `application.New`、`application.NewService`、`Window.NewWithOptions` 和 `BundledAssetFileServer`。
- `apps/desktop/app.go` 通过 `ServiceStartup`/`ServiceShutdown` 管理核心服务，`ShouldQuit` 在原生退出钩子阶段保护活动任务；文件和目录选择使用 v3 `app.Dialog.OpenFile()`。
- TypeScript 绑定由 `wails3 generate bindings -ts -i -clean=true` 生成到 `apps/desktop/frontend/bindings`，前端导入 `@wailsio/runtime` 与绑定接口，不再依赖 `frontend/wailsjs`。
- `apps/desktop/Taskfile.yml` 及 `build/{Taskfile.yml,windows, darwin}/Taskfile.yml` 统一使用 `wails3 task`；生产前端通过 embed 进入二进制。

## v2 残留审计

已移除：`github.com/wailsapp/wails/v2` 依赖、`wails.Run` 入口、v2 `wails.json`、旧 `frontend/wailsjs` 生成物。源码检索应只在历史文档或迁移备份中看到 v2 字样；有效构建入口不调用 `wails build`。

## 实际检查

在 Windows 11 amd64：

```powershell
$env:GOWORK = 'off'
go test ./...
go vet ./...
go build ./...
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm run lint
pnpm run test
pnpm run build
..\..\.tools\bin\wails3.exe task build ARCH=amd64
```

以上 Go/前端检查和 Wails 3 production build 已通过。`go version -m apps/desktop/bin/LinkSend.exe` 显示 `github.com/wailsapp/wails/v3 v3.0.0-beta.18`；Windows 产物 SHA256 以验收记录为准。

未在当前 Windows 环境运行 macOS 原生 WebKit、DMG 挂载或红点/Cmd+Q 人工验收，标记为 `BLOCKED_BY_EXTERNAL_ENV`；Mac runner 与本机构建命令见 [BUILD-MACOS.md](BUILD-MACOS.md)。

## 候选构建确认（2026-09-10）

任务分支 `codex/wails3-hk-dmg-20260909` 的候选 commit
`8087f876ac4a94a43a1c46536a78493064092d32` 已通过 GitHub Actions run
[34384170503](https://github.com/Wen5555/LinkSend/actions/runs/34384170503)：Windows amd64、
macOS arm64 DMG、macOS amd64 DMG 三个 job 均成功。离线核验显示两个 DMG 都包含完整
`LinkSend.app`、`com.linksend.desktop`、测试说明和对应 `GOARCH`，`go version -m` 均报告
Wails `v3.0.0-beta.18`。这证明构建链和包结构，不等价于真实 Mac 原生窗口验收。
