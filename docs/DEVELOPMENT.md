# Development

PowerShell 7 下从仓库根目录工作。先阅读 `docs/SPEC.md`、`docs/PROGRESS.md` 和 `AGENTS.md`。

```powershell
gofmt -w .
go test ./...
go vet ./...
go test -race ./...
Set-Location apps/desktop
$env:GOWORK = "off"
go test ./...
Remove-Item Env:GOWORK
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm run lint
pnpm run test
pnpm run build
```

`go test -race` 需要可用的 C 工具链。当前 Windows 机器自带的 MinGW GCC 8.1 会导致 race 可执行文件以 `0xc0000139` 退出；已安装 Scoop 的 MinGW 16.2.0，并用以下显式环境验证通过：

```powershell
$env:CC = "$env:USERPROFILE\scoop\apps\mingw\current\bin\gcc.exe"
$env:CXX = "$env:USERPROFILE\scoop\apps\mingw\current\bin\g++.exe"
$env:Path = "$env:USERPROFILE\scoop\apps\mingw\current\bin;$env:Path"
go test -race ./...
```

不要通过关闭 CGO 或跳过测试掩盖工具链问题。桌面构建需要 WebView2 和 Wails 工具链。

版本固定在根 `go.mod`、桌面 `go.mod`、`pnpm-lock.yaml` 和 ADR 中。改变协议时同步 `docs/PROTOCOL.md` 并增加兼容性/安全测试。
