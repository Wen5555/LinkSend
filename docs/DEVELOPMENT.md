# Development

PowerShell 7 下从仓库根目录工作。先阅读 `AGENTS.md`、`docs/SPEC.md` 和 `docs/PROGRESS.md`，再保存当前 branch、HEAD 与 `git status --short`。仓库可能有用户未提交改动和大量 untracked；禁止 `reset --hard`、`clean` 或整目录盲目暂存。

产品版本当前为 `0.2.0`，协议版本为 V1。产品版本的唯一 Go 常量是 `internal/protocol.ProductVersion`，并同步到 Wails `build/config.yml`、Windows manifest/NSIS、macOS plist 与前端 `package.json`。版本更新后必须重新生成 bindings/build assets，并检查 CLI、`/healthz`、桌面 DTO 和安装包元数据一致。Wails 3 beta.18 的 build-assets 会把 Windows fixed version 生成为三段且省略 `FileVersion` 字符串；提交前必须将 `info.json` 的 fixed file/product version 规范为四段 `<semver>.0`，使用 `0409` string table 并保留三段显示字符串，`TestProductVersionMetadataSynchronized` 会阻止该回归。

```powershell
gofmt -w <本轮修改的.go文件>
git diff --check
$env:GOWORK = "off"
go mod verify
go test ./...
go test -race ./...
go vet ./...
Remove-Item Env:GOWORK

Set-Location apps/desktop
$env:GOWORK = "off"
go mod verify
go test ./...
go vet ./...
go build ./...
$env:Path = "D:\apps\Osend\.wails-bin;$env:Path"
wails3 generate bindings -ts -i -clean=true
Remove-Item Env:GOWORK

Set-Location frontend
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm run lint
pnpm run test
pnpm run build
```

前端 build 与桌面 production build 必须串行，因为 Vite 会重建被 Go `embed` 的 `frontend/dist`。随后从 `apps/desktop` 执行 `wails3 task build ARCH=amd64`，并核对 EXE 中嵌入的最新带 hash 资产名。

Windows race 需要可用 C 工具链；本机使用 Scoop MinGW 16.2.0，不能通过关闭 CGO 或跳过 race 掩盖工具链失败。Wails CLI 固定为 `v3.0.0-beta.18`，当前工程没有 Wails 2 构建入口。

## 测试配对入口

`configs/server.dev.toml` 可显式设置 `test_pairing_code = "orion123"` 和目标测试组，用于隔离 UI/e2e 测试。默认值为空；缺少明确 `test_pairing_group` 时服务端启动校验拒绝。注册仍需 Ed25519 签名。香港入口属于测试主站，可按授权同步版本，但每次都必须执行 `inspect → backup → change → verify → rollback-ready`，不得记录固定码、令牌或完整配置。

协议变化必须更新 `docs/PROTOCOL.md` 和兼容/安全测试；schema 变化必须更新 ADR 0002、迁移与回滚说明。完整门槛见 [TESTING.md](TESTING.md)。
