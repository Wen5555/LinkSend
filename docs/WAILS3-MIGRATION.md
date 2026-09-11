# Wails 3 迁移记录

## 当前实现

- 产品版本 `0.2.0`；Wails CLI/Go 依赖固定为 `v3.0.0-beta.18`，属于预发布依赖。
- `apps/desktop/main.go` 使用 Wails 3 `application.New`、`application.NewService`、`Window.NewWithOptions` 和 `BundledAssetFileServer`。
- `apps/desktop/app.go` 通过 `ServiceStartup`/`ServiceShutdown` 管理共享核心；`ShouldQuit` 空闲时返回 true，活跃任务时异步显示保护对话框。
- 文件/目录选择使用 Wails 3 原生 dialog，打开目录通过受控后端方法完成；文件正文不经过 binding。
- TypeScript bindings 生成到 `apps/desktop/frontend/bindings`，前端使用 `@wailsio/runtime` 并按 task revision 丢弃旧响应。
- `Taskfile.yml` 和平台 Taskfile 全部调用 `wails3 task`，production 前端通过 Go embed 进入二进制。

## Wails 2 残留审计

有效源码和构建入口没有 `github.com/wailsapp/wails/v2`、`wails.Run`、v2 `wails.json` 或 `frontend/wailsjs`。AGENTS.md 中“Wails 2 desktop”属于旧项目说明，不能据此回迁；当前仓库实际依赖和任务列表以 Wails 3 为准。

## 必须执行的检查

```powershell
$env:GOWORK = 'off'
go test ./...
go vet ./...
go build ./...
$env:Path = 'D:\apps\Osend\.wails-bin;' + $env:Path
wails3 version
wails3 task -list-all
wails3 generate bindings -ts -i -clean=true
Set-Location frontend
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm run lint
pnpm run test
pnpm run build
Set-Location ..
wails3 task build ARCH=amd64
```

bindings 生成后必须核对生成模型、React 类型和 Go DTO；前端构建和桌面编译串行，并检查二进制嵌入的是最新 dist。当前自动化覆盖 1 个 service、27 个 methods、14 个 models。

## 原生证据边界

2026-09-11 的 r2 测试快照已在 Windows 11 amd64 通过 `WM_CLOSE` 空闲退出，并在 macOS arm64 创建真实窗口后通过 quit Apple Event 空闲退出。这只证明原生窗口与 idle quit，不证明文件/目录对话框、打开目录、红点实际点击、Cmd+Q 按键、活跃任务保护或恢复入口。辅助功能权限未具备时这些项目保持 NOT_RUN；不得用浏览器截图或修改 TCC 数据库替代。

历史 commit/run/资产记录保留在 PROGRESS 和 BUILD-MACOS 中；旧 `0.1.0` 包不能通过改写元数据冒充 `0.2.0`。

`v0.2.0` Release 源码 `426d58b6ab62ab7213475007305c0a403955c00f` 在 main run `34600609161` 的 Windows 2022、macOS arm64 和 macOS Intel runner 上均使用 Wails 3 `v3.0.0-beta.18` 重新生成 1 service / 27 methods / 14 models 并完成构建。Release Windows ZIP 中 EXE 又通过真实窗口、非零句柄和 idle `WM_CLOSE`；macOS workflow 只完成 DMG 挂载、bundle/架构和 strict ad-hoc codesign，Release 包未完成物理 Mac 启动，因此不把 runner 构建等同于完整原生交互验收。
