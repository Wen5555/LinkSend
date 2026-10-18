# 桌面 GitHub 更新实施记录

日期：2026-10-18；基线main `3aa51486340ef2ad3eda30c063701ffd3199f2b3`；工作分支`codex/github-desktop-updates`。这是0.6.1之后的本地未发布功能，没有更改既有Release。

## 功能

- 设置增加软件更新页，可检查正式版本、查看纯文本说明、按平台下载、查看进度、取消、打开校验后的安装包及退出应用。
- Go固定访问官方GitHub仓库，校验版本、平台、URL、包大小与SHA256；打开前再校验，失败不能启动。只有用户点击检查/下载才联网，更新功能不依赖信令连接。
- Windows使用安装向导，Mac使用DMG拖拽安装，安装前需退出含托盘的旧程序。配置/文件保留。关于页改用后端版本。

## 已运行验证

- Go1.27.1、Node24.21.0、pnpm12.4.1、固定Wails3 beta.18；绑定已生成。
- `GOWORK=off go test -race ./updater -timeout 90s`通过：数值版本与降级防护、平台选择、无摘要/外仓地址/重复包拒绝、Draft/prerelease拒绝、校验后才打开、文件变化拒绝、取消/重复操作/重试、坏摘要/截断清理、HTTPS/域名/元数据重定向限制与GitHub限流状态。
- `LINKSEND_TEST_GITHUB_UPDATES=1 GOWORK=off go test ./updater -run '^TestLiveGitHubCheckAndDownload$' -v -timeout 4m`真实HTTPS通过，8.25s：以测试用旧版本`0.0.0`触发下载当前正式版0.6.1 Windows amd64安装器，75,189,777 bytes，SHA256 `ad3adaa4c446d49df11b324cf2aebcc70c7bbc8ea202ecc94d252f7854df2628`。正文经Go下载/校验，测试缓存自动清理，**未执行安装器**。日志：`.artifacts/desktop-updates-20261018/github-live-test.log`。
- 前端typecheck（含绑定）、lint、123项测试与生产build通过。新增用例覆盖明确点击才检查、手动下载/打开顺序、进度、返回页面继续读状态、取消重试、校验失败不能打开与纯文本说明。
- 核心`GOWORK=off go test ./...`、`go vet ./...`、`go build ./...`通过。本轮一次桌面Go检查与前端重建并行，旧embed清单撞上新资源，报文件缺失；不是产品行为结论，前端完成后改为顺序检查并通过，见下项。
- 前端完成后，桌面`GOWORK=off go test ./...`、`go vet ./...`、`go build ./...`及更新模块race均通过。`GOOS=darwin GOARCH=arm64|amd64 go test -c ./updater`两个测试二进制交叉编译通过，未在Mac执行。
- 固定CLI生成Windows资源后，以`go build -tags production -trimpath -buildvcs=false -ldflags '-w -s -H windowsgui'`生成本地试用`.artifacts/desktop-updates-20261018/LinkSend.exe`，24,332,800 bytes，SHA256 `c2fc1e460c4cde5d9753b451fc866de9b0c8c1846d28004c1960b7354b6213ed`。资源版本仍为0.6.1，来源是当前未提交工作树，**不是已发布0.6.1的同一个包**。未启动此EXE；来源清单见同目录`VERIFICATION.json`。

## 尚未验证

本轮没有执行实际Windows安装/UAC或Mac原生DMG覆盖、升级后重启与最低OS矩阵；跨架构包选择由确定性测试覆盖。原物理剪贴板、系统入口与双NAT缺项保持。本地构建不替代正式发布CI或包验收。

实现与授权边界见[ADR0014](../adr/0014-desktop-github-updates.md)。

## v0.6.2正式发布后的真实更新检查

PR #11正常合并，准确源码63c86c2c5ce183e1400d27ca0571610d50a42bdc，正式Release https://github.com/Wen5555/LinkSend/releases/tag/v0.6.2 为Latest，六资产digest及准确包检查通过。发布后以LINKSEND_TEST_GITHUB_UPDATES=1、GOWORK=off执行go test ./updater -run ^TestLiveGitHubCheckAndDownload$ -count=1 -v -timeout 4m，真实HTTPS发现并完整下载0.6.2 Windows安装器、SHA256校验通过，未执行安装器。实际日志行：`updater_test.go:320: LIVE_CHECK_DOWNLOAD_VERIFIED version=0.6.2 platform=windows-amd64 bytes=75245859 sha256=d4e5d364d33c5b66fe57331fe588a9772d46874f5d88fd22e5b510040fdd2651 installer_launched=false`；完整日志.artifacts/release-062/live-update.log。0.0.0仅为显式测试旧版本，不是产品版本；旧0.6.1没有本轮更新入口，首次需手动升级。实际覆盖安装/重启与最低OS矩阵仍未验证。
