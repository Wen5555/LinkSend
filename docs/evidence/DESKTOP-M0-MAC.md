# M0 精确源码的物理 Mac 构建与原生冒烟

**用户最终决策：最低 macOS 13，取消 macOS 12 支持，继续使用 Go 1.27.1，并合并发布 M0/M1。** Go 1.27.1 因此与最终最低系统目标兼容；本页仍只记录旧 M0 提交在 macOS 26.5 的真实验证，最终 Release 必须等待包含 M1 和 macOS 13 元数据的新提交重建，不能直接复用旧 SHA 的包。

兼容性问题及处理历史：官方 [Go 1.27 Darwin 说明](https://go.dev/doc/go1.27#darwin) 明确要求 macOS 13 Ventura 或更高版本；Go 1.27.1 链接器源码 `src/cmd/link/internal/ld/macho.go:449` 的最小版本为 `macVersionFlag{13,0,0}`。原规范仍写 macOS 12 时曾决定回退 Go 1.26.8，但用户随后明确取消 macOS 12，因此该回退不再作为候选。旧包 Info.plist 的 12.0 声明也必须随新提交改为 13.0，不能仅靠 Go 二进制 minimum OS 推定 bundle 元数据已正确。任何 macOS 26.5 上的 PASS 均不追认为 macOS 12 或真实 macOS 13 机器已跑过。

本记录只对应源码提交 `b3d5fc7b6acf49dfc07671d774e2a7f20c28cc94`（M0）。后续 M1–M6 主工作树修改没有被混入本次 Mac 源码。测试包用于本地验收，最终 Release 仍应采用 CI 对同一提交生成、可追溯的资产。

## 源码与环境

源包由该提交 `git archive` 生成，Windows 输入文件 `.artifacts/desktop-six-features/m0-source.tar.gz`，大小 19,944,011 bytes，SHA256 `e942f937303c4d69ed03dc112945a85d5504ac9d0f64eb31806853cc05f01fd5`。本地验证后经 manager upload 上传到 `/Users/wen/linksend-desktop-six/m0-source.tar.gz`，远端再次校验 SHA256，并检查归档内没有绝对路径或 `..` 父路径后才解压。

独立源码目录 `/Users/wen/linksend-desktop-six/m0-b3d5fc7`；日志目录 `m0-evidence`。归档不含 `.git`，因此仅准确记录 `source_archive_commit=b3d5fc7...` 和 archive hash，不声称二进制自带 VCS commit 或 `vcs.modified=false`。Wails production Taskfile 本身还使用 `-buildvcs=false`，两者均在来源说明中保留。

物理机器通过 SSH manager alias `mac-test-102342413` 连接：用户 wen，`10.234.184.6:22`，macOS 26.5 / Darwin 25.5.0 / arm64，CLT/Apple clang 21.0.0 / SDK 26.5。使用隔离工具路径 `/Users/wen/linksend-desktop-six/tools` 下的 Go 1.27.1、Node 24.21.0 LTS、pnpm 12.4.1、由 Go 1.27.1 编译的 Wails CLI beta.18。没有替换 Homebrew、修改全局 shell 配置或关闭现有 0.3.0 用户应用。

所有远程操作均通过 manager upload/run-script。构建 job：`/tmp/codex-ssh/desktop-m0-mac-check-20260912T044515Z`。状态读取使用 POSIX sh 的 manager run-script，规避 manager tail-job 在 Mac 默认 zsh 中使用只读变量 `status` 的既有问题；没有因查询失败重复启动构建。

## 实际检查

所有 Go 命令都显式 `GOWORK=off`、`GOTOOLCHAIN=local`，包代理为本次进程的 `https://goproxy.cn,https://proxy.golang.org,direct`，不修改全局 Go 环境。脚本为每个命令记录独立退出码；任何失败使最后总退出码非零，不用最后一个成功命令掩盖早先错误。

| 范围 | 实际命令 | 结果 / 退出码 |
| --- | --- | --- |
| 根格式 | `test -z "$(gofmt -l internal cmd tests)"` | PASS / 0 |
| 根依赖 | `go mod verify` | PASS / 0 |
| 根测试 | `go test -count=1 ./...` | PASS / 0 |
| 根 race | `go test -race -count=1 ./...` | PASS / 0 |
| 根静态检查 | `go vet ./...` | PASS / 0 |
| 根构建 | `go build ./...` | PASS / 0 |
| 桌面独立依赖 | `go mod verify` | PASS / 0 |
| 类型化绑定 | `wails3 generate bindings -ts -i -clean=true` | PASS / 0；原始日志 warning 数 0 |
| 前端依赖 | `pnpm install --frozen-lockfile` | PASS / 0 |
| 前端类型 | `pnpm run typecheck`（含 bindings 独立检查） | PASS / 0 |
| 前端 lint | `pnpm run lint` | PASS / 0 |
| 前端测试 | `pnpm run test` | PASS / 0；`connection.test.ts` 的 11 项测试，1 个测试文件 |
| 前端产物 | `pnpm run build` | PASS / 0 |
| 桌面独立测试 | `go test -count=1 ./...` | PASS / 0 |
| 桌面静态检查 | `go vet ./...` | PASS / 0 |
| 桌面独立构建 | `go build ./...` | PASS / 0 |
| 原生 arm64 DMG | `bash scripts/build-macos.sh --arch arm64 --format dmg` | PASS / 0；真实挂载、arm64、bundle ID、strict ad-hoc codesign |

前端 build 完成后才开始桌面 Go embed/production build。上述 source archive 在运行期间保持独立；生成 bindings、dist、图标等构建输出不被虚称为“Git 工作树干净”。完整命令状态在 `/Users/wen/linksend-desktop-six/m0-b3d5fc7/m0-evidence/commands.tsv`，每个检查有同名 `.log`。总构建 job 最终退出 0，全部 17 个独立检查均为 0。检查从 2026-09-12T04:45:16Z 开始，包与原生最终复核完成于 04:48:51Z。

## 原生证据范围

原生窗口冒烟已在物理 Mac 执行，job `/tmp/codex-ssh/desktop-m0-native-smoke-20260912T044801Z` 退出 0。使用本轮产品 0.5.0 `.app`、独立 `LINKSEND_DATA_DIR=/Users/wen/linksend-desktop-six/m0-b3d5fc7/native-profile`，信令仅指向 `http://127.0.0.1:9` 并显式允许测试 loopback；没有连接或改写用户真实设备 profile。

通过本地编译的 Cocoa/CoreGraphics 小探针，按测试进程 PID `53735` 确认可见 layer 0 窗口 `7981`，实际边界 `1008 × 684`，窗口数 1；随后 NSRunningApplication `terminate` 返回接受，应用正常退出码 0，没有触发强杀清理。用户原 0.3.0 应用 PID `47950` 在测试后仍运行。测试不使用 AppleScript、Accessibility 或伪造前端事件；不将启动/退出当成文件入口、托盘、通知、睡眠或接收弹窗完整验收。

独立包复核再次以只读方式挂载本轮 DMG，`cmp` 确认镜像内 executable 与上述实际启动的 `.app` executable 完全相同；`codesign --verify --deep --strict` 和 `lipo -info` 再次通过。该复核 job 为 `/tmp/codex-ssh/desktop-m0-export-evidence-20260912T044849Z`，退出 0。挂载点只用于本轮并已卸载。

## 产物和证据

产品 0.5.0；协议 V1；包为 darwin/arm64、ad-hoc 签名、未公证。源归属为上述精确 archive commit，不作为最终 Release CI 资产冒充发布。

| 文件 | 大小 / SHA256 |
| --- | --- |
| `LinkSend.dmg` | 8,481,538 bytes；`135ed3692bc71538ed2f5dd680fe73677551544dac31987028ec56e050b9eef9` |
| 镜像内 `LinkSend.app/Contents/MacOS/LinkSend` | `903e88b9977cc7555cdd92dba853b7f919a406ff8e75f43c16b88bc2add18eea` |
| `m0-evidence-b3d5fc7.tar.gz` | `91ff6d556b40a11c448923a781e9e8d1e63f4a73dd0385831ac1471eeb5b2d3d` |

DMG 已经 manager download 到 `.artifacts/desktop-six-features/m0-mac-arm64/LinkSend.dmg`，证据归档下载到同目录并解压为 `m0-evidence/`；Windows 重新计算两份 SHA256 与远端结果相同。完整证据包含 BUILD-INFO、各独立检查日志、commands.tsv、原生窗口/退出日志与探针源码。没有下载或打包测试 profile 的身份文件。

复现入口是本次保留的 manager job 中的 `script.sh`；构建脚本拒绝覆盖既有工作目录，原生脚本拒绝复用既有测试 profile。复测时应选择新的隔离路径，不能为复用脚本递归删除用户或旧证据目录。

Mac amd64、物理双机、双 NAT、睡眠唤醒、完整原生控件和安装/卸载尚未因上述结果获得 PASS。
