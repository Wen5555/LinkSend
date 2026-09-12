# M0/M1 合并候选的物理 Mac 验证

本记录对应精确提交 `2f2656d940c96e142d5639b2bc3012ab0ce5fb6e`，产品 0.5.0 / 协议 V1。用户已明确最低 macOS 13、使用 Go 1.27.1，不再要求 macOS 12。实际机器是 macOS 26.5 / arm64；最低版本元数据验证不能冒充在 macOS 13 实机运行。

## 来源和隔离

源 archive SHA256：`4eb0079654e9fd987cb2d79522d9e25f5c8a4de6eb3e7839130099a1ba752667`；Windows 本地和 Mac 上传后均校验。archive 来自指定 commit，不含 `.git`，BUILD-INFO 准确记录 `source_archive_commit`，没有声称原生二进制附带 `vcs.modified=false`。M2/M3 工作树代码没有混入。

独立工作目录 `/Users/wen/linksend-desktop-six/m1-2f2656d`，profile 为其下新建的 `native-profile`，信令仅指向无人监听的 loopback `http://127.0.0.1:9`。使用 `/Users/wen/linksend-desktop-six/tools` 的 Go 1.27.1、同 Go 编译的 Wails beta.18、Node 24.21.0、pnpm 12.4.1。所有 SSH 通过 `mac-test-102342413` 的 manager 执行，没有替换或关闭用户应用。

## 自动化结果

构建 job `/tmp/codex-ssh/desktop-m1-mac-check-20260912T054325Z` 最终退出 0，17 个独立检查全部通过。每条命令单独记录退出码，前端 build 与后续 Go embed/production build 串行。

| 范围 | 实际命令 | 结果 |
| --- | --- | --- |
| 根格式 | `test -z "$(gofmt -l internal cmd tests)"` | PASS / 0 |
| 根独立模块 | `GOWORK=off go mod verify`、`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...` | 全部 PASS / 0 |
| 绑定 | `wails3 generate bindings -ts -i -clean=true` | PASS / 0；warnings=0 |
| 前端 | `pnpm install --frozen-lockfile`、`pnpm run typecheck`、`lint`、`test`、`build` | 全部 PASS / 0；3 个测试文件、29 项测试通过 |
| 桌面独立模块 | `GOWORK=off go mod verify`、`go test -count=1 ./...`、`go vet ./...`、`go build ./...` | 全部 PASS / 0 |
| DMG | `bash scripts/build-macos.sh --arch arm64 --format dmg` | PASS / 0；实际挂载、bundle ID、arm64、strict ad-hoc codesign |

所有 Go 命令还显式设置 `GOTOOLCHAIN=local`，直接 CGo 检查和构建设置 `MACOSX_DEPLOYMENT_TARGET=13.0`、`CGO_CFLAGS=-mmacosx-version-min=13.0`、`CGO_LDFLAGS=-mmacosx-version-min=13.0`，没有默认 SDK target 漂移。

## Mach-O 与真实原生冒烟

job `/tmp/codex-ssh/desktop-m1-mac-minimum-20260912T054703Z` 直接解析 Mach-O 的 LC_BUILD_VERSION，结果：minimum `13.0.0`、SDK `26.5.0`；plist `LSMinimumSystemVersion=13.0.0`，两者一致，产品 0.5.0，bundle `com.linksend.desktop`。这已解决旧 M0 archive 的 Go 13 / plist 12 声明冲突。

job `/tmp/codex-ssh/desktop-m1-native-smoke-20260912T054705Z` 退出 0。CoreGraphics 对本轮测试进程 PID `58280` 查得真实可见窗口 `7983`，大小 `1008 × 684`；NSRunningApplication 的正常退出请求被接受，应用退出码 0。用户原 0.3.0 进程 PID `47950` 仍运行。没有使用浏览器截图、伪造前端事件或系统权限绕过。

再次只读挂载 DMG 后，`cmp` 验证镜像内 executable 与这次实际启动的 executable 完全相同；codesign strict 和架构检查再次通过。复核 job `/tmp/codex-ssh/desktop-m1-export-evidence-20260912T054759Z` 退出 0。

## 下载产物

| 项目 | 实际值 |
| --- | --- |
| arm64 DMG | 8,540,090 bytes；SHA256 `09f9d9d9bdab20a89b183179097f3c79b777d630cda453d953c50d4f6485123c` |
| 镜像内 executable | SHA256 `4d3e059301f7a885829936b9e3a23045f84677b5556c30cfa51d0f8fc6d3ea2b` |
| 完整日志归档 | SHA256 `743e0f4833100140c95cf624c679003761e0d31ef59ac752496c30c7aa981eac` |
| 本地目录 | `D:/apps/Osend/.artifacts/desktop-six-features/m1-mac-arm64/`，下载后再次计算两份 SHA256 一致 |

目录中有 `LinkSend.dmg`、`m1-evidence-2f2656d.tar.gz` 和已展开的 `m1-evidence/`，包含 BUILD-INFO、17 条命令退出码、各测试日志、最低系统元数据、原生窗口/退出日志和测试探针源码。测试 profile 身份没有打入日志包。

该包为 ad-hoc 签名、未公证；最终 Release 仍采用精确同提交 CI 的正式归档资产。此处验收范围为 M0/M1 的构建、真实空闲窗口启动/退出；完整 M1 原生控件、macOS 13 实机、Finder/SendTo、托盘/通知、睡眠、物理双机和跨 NAT 均不能因本记录新增 PASS。
