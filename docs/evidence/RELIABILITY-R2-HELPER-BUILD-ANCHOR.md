# R2 Windows helper 同次构建与私有镜像锚

日期：2026-10-26。范围仅为构建、ZIP/NSIS 载荷与私有 resolver；系统 publisher、剪贴板、Explorer、自动删除和安装旅程均未运行。本增量不标 R2 完成。

## 来源与实现

本地唯一实际构建捕获 `3ee8edf2945db4d80fdab6bbcb00c369ec1defcf` 加工作区源码，`source_clean=false`，构建输入摘要 `aec8098bc006a44744d9ad4a5e35d4ca73e50108fc85db9623d9c92603b01a34`。不能把它称为后续 `8e8745b` 或最终提交的 clean 准确包。helper 源码已由[生命周期增量](RELIABILITY-R2-NATIVE-PROCESS-LIFECYCLE.md)冻结；本次增加 `-trimpath -buildvcs=false` 后的二进制摘要与该增量原局部构建不同，未复用旧摘要。

`scripts/build-windows-desktop.ps1` 在同一事务内先以 CGO1 编译独立 static helper，读取实际 PE machine、SHA256 和 Go build metadata，再以 CGO0 编译桌面。锚包含版本、windows/架构、实际 helper 摘要、源提交、构建输入摘要和真实 clean 状态，经 ldflags 编入桌面初始化路径。构建前后提交、输入摘要或 dirty 状态变化即失败；每次先撤掉旧打包 metadata，失败不能借旧 manifest 兜底。未安装新编译器或更改宿主 PATH。

摘要范围为 Git 枚举的已跟踪及未跟踪 `go.mod/sum/work/work.sum`、`internal/`、`cmd/`、`apps/desktop/` 与该 builder；排除 bin、node_modules、工具生成目录、WebView bootstrapper 和明确无关的用户 `build/ios`、`build/linux`，不读取这些用户文件正文。生成的本架构 Wails syso 实际字节纳入摘要，只在 clean 判断中豁免这个确定文件。其余真实 Git 状态仍计 dirty，包括本地保留的三个用户未跟踪目录；没有更改 Git excludes。

桌面只使用编入锚和自身实际 EXE 目录，固定寻找 `native-clipboard/<sha256>/LinkSendClipboardFileHelper.exe`。空锚、非规范字段名、重复/未知/缺失键、null 布尔、版本/平台/摘要/提交非法、缺失或被替换的 PE、路径 reparse 均拒绝。没有用户路径/hash 配置，不以 sidecar、BUILD-INFO、helper 自报或 PID 为信任根。resolver 不启动 helper，也没有 Wails API、profile key/DB 或 publisher；未来调用仍必须通过 core 实际镜像 pin/recheck。

Windows 原生构建现在生成同次 manifest/NSIS include。发行 ZIP 只增加当前摘要对应的一个 helper；BUILD-INFO 只记审计字段。NSIS 只复制当前文件，并在卸载时只删除同一个确定文件及空目录；不扫描旧 helper、不递归删除。不同摘要使用不同目录，不能据此声称同摘要活跃进程的安装锁问题已解决。非 Windows Go 编译仍保留为 compile check，NSIS 对非 Windows 主机构建明确拒绝，避免遗留 include 被当作新锚。MSIX 和其他平台未新增 helper 载荷，不作为本增量的包级通过范围。

## 最小验证

现有工具链：Go1.27.1、Wails3 beta.18、Node24.21.0/pnpm12.4.1、MinGW16.2.0、既有 .NET9.0.319/NSIS；没有 GUI、系统剪贴板、注册或宿主网络操作。

| 命令/检查 | 实际结果 |
|---|---|
| `CGO_ENABLED=0 go test ./internal/clipboardhelper -count=1 -v`（desktop module/workspace） | exit0；严格解析、实际 PE/平台、缺失与篡改、固定目录通过 |
| 同包 `GOWORK=off go test ./internal/clipboardhelper -count=1`、`go vet ./internal/clipboardhelper` | 各 exit0；没有重复全矩阵/race |
| PowerShell Parser 检查 builder/verifier，`git diff --check` | exit0 |
| `GOWORK=off wails3 task build ARCH=amd64` | 唯一实际 build exit0；真实 helper CGO1、主程序 CGO0、前后源稳定性及 buildinfo 检查通过；来源保持 dirty |
| 同次 `.nsh` + `makensis -DWAILS_INSTALL_SCOPE=user -DREQUEST_EXECUTION_LEVEL=user ... project.nsi` | exit0；主 EXE/installer ProductVersion 均 `0.6.2` |
| `7z e` 只提取 installer 的确定 helper，回读 SHA256 | exit0；与编入锚的当前 helper 摘要相同；没有执行安装器 |
| `.artifacts/r2-helper-packaging/verify-static.ps1` | exit0；同次 manifest/主程序/实际 helper 一致、锚字节存在；用生产 ZIP guard 测试合法载荷及重复、traversal、旧 helper、缺失、篡改五个负例，均符合预期 |

目录符号链接负例因当前 Windows 缺少创建符号链接权限而 `SKIP`，没有提升权限或改宿主配置；源码仍拒绝 reparse。ZIP 测试只运行真实生产名称/hash guard，不伪造 `source_clean=true`，不把本地 dirty 包送作正式来源验收。测试 ZIP 的 BUILD-INFO 明示 fixture/WORKING_TREE/false。

实际静态收据：`.artifacts/r2-helper-packaging/STATIC-RECEIPT.json`、`NSIS-HELPER-RECEIPT.json`；日志 `build.log`、`nsis.log`、`nsis-inventory.log` 和 `nsis-extract.log`。关键摘要：

| 载荷 | SHA256 |
|---|---|
| helper，10,177,330 bytes | `a6c2ffb5bf1cc029b7a843fb6e543cbc122482e993b97dc0aea4e4d97b2e51ce` |
| 桌面 EXE | `8c95e9a8f97ed8c183dd47d1ba6baee9eddfefc188c55df32e163f6b5d075e5b` |
| NSIS installer | `c129ead4ab4c24973598244df6d115a66104dd027e508c03e3a8c319625b0b0c` |

.NET 首次运行打印了通用“已安装开发证书”文案。只读核对 CurrentUser/My 后只看到 2026-09-13 既有 localhost 开发证书，没有本轮对象；没有删除、信任或更改证书。本轮已将 Share Target task 的 `DOTNET_GENERATE_ASPNET_CERTIFICATE=false` 限于子进程，未改用户全局设置。

## 仍未覆盖

本次 builder 之后只补了任务局部 .NET 环境、非 Windows NSIS 入口保护和末尾仅清理本次确定 syso、文档，没有重跑 Go build；本地包保持上述原工作区来源。Taskfile不再使用`*.syso`清理其他文件；没有为此实际删除未知文件。最终准确 clean 来源、GitHub Windows 同包 metadata/hash 仍由本增量提交后的实际 CI 核对。没有宣称默认 publisher、系统剪贴板、Explorer 键盘、Native 删除、安装覆盖/卸载或旧 helper 自动清理通过。旧 helper/未知文件保守保留，活跃同摘要镜像的安装锁行为另验；不新增用户缓存设置。
