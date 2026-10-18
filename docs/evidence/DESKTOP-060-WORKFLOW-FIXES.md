# 0.6.0 后桌面工作流修复

日期：2026-10-18。基线main `9d8334c7d5120e985386bbcb57c995e6d48049d6`，分支`codex/fix-desktop-workflows`。本记录先保留本地源码阶段证据；用户随后明确要求发布0.6.1，准确包结果见[0.6.1发布说明](../RELEASE-v0.6.1.md)。不移记为原0.6.0包结果。用户要求优先功能可用，未恢复旧受限原生/双NAT矩阵。决策见[ADR0013](../adr/0013-desktop-transfer-workflows.md)。

## 已实现

1. 记录的文件清单提供系统“打开文件/文件夹”和“打开所在文件夹”；发送可定位原源文件，接收验证真实计划、完成状态与文件。展开清单滚动定位，避免看起来点击无响应。
2. 自动剪贴板修复原生启动时序、失败重试、Windows PNG检测/短暂占用读取；设置增加逐设备同步开关。真实加密会话复现恢复后7秒续期晚于3秒空闲关闭，改为立即续期。
3. 普通删除与拒绝连接拆开：删除立即忘记本地配对及偏好，没有已删除目录；黑名单独立拒绝。跨网络配对下面显示LAN发现目录，手动IP放在“附近没有设备出现，或没有想要的设备？”折叠入口。
4. 一个传输页同时选文件和跟踪收发队列，发送task/queue去重、速度/百分比/取消、批次控制、完成清理和全部清理。默认退出清空本页并取消未完成任务，可在通用设置关闭；文件与历史保留。受信设备新增接收显示导航红点计数，进入传输页清零。

## 验证

- Go1.27.1、Node24.21.0、pnpm12.4.1，沿用仓库固定工具；Wails beta.18用已有Go1.27.1工具生成绑定，无警告。初次旧Go1.26 Wails有警告，随后正确工具完整重生成。
- 两Go模块的`GOWORK=off go test ./...`、`go vet ./...`、`go build ./...`通过。新增发送文件/目录定位、移动/变更拒绝、清理保留历史文件、新attempt再显示、删除旧授权隔离和重新配对/黑名单保护回归。
- `TestClipboardConcurrentEnsureSessionsRemainReady`新增双端暂停/恢复断言，在旧ResetClipboard下真实失败：`unpausing did not immediately renew clipboard leases`。立即续期后通过并收到恢复后新复制文本；loopback不是物理双机。
- 前端`pnpm run typecheck` / `lint` / `test` / `build`通过，119项测试。覆盖统一收发/去重/进度、红点持续到访问、逐设备六项明确授权、删除直接执行/黑名单分离/手动IP折叠和旧文件清单保护。
- 核心`GOWORK=off go test -race ./internal/app ./internal/identity -run 'TestClipboard|TestForget|TestClearTransfer|TestResolveSent' -count=1 -timeout 120s`通过；桌面`GOWORK=off go test -race ./...`通过。收据：`output/desktop-fixes-20261018/{core-targeted-race,desktop-race}.log`。
- Playwright真实页面组件加明确标注模拟任务夹具：1120×760、720×560无横向溢出；压缩发送区后1120首屏可见队列及收发进度；LAN目录在跨网络配对下，手动IP初始关闭、点击可展开。截图：`output/playwright/desktop-{transfer-1120,transfer-720,devices-1120}.png`。夹具不是原生传输证明；浏览器唯一警告为Wails正常预览提示。

最终核心和桌面的workspace/off测试、off vet/build均通过；前端最终119项/typecheck/lint/build通过。Windows生产标签本地试用程序为`output/desktop-fixes-20261018/LinkSend-local-fixes.exe`（未启动、非发布包），准确hash与分层源码来源见同目录`VERIFICATION.json`、`SOURCE-ARCHIVE.json`和`SHA256SUMS.txt`。原生退出确认已按清空设置匹配“取消并退出”或“保存并退出”，避免误报保存。

Mac在`mac-test-102342413`隔离源码上用Go1.27.1/cgo做定向桌面与命名pasteboard测试及production编译，arm64产物成功，job `/tmp/codex-ssh/job-ls-workflows-final-20261018-v2` finished/exit0。归档SHA和app.go收尾覆盖前后SHA均已验证，日志下载为`mac-verification-final.log`。链接器有26/13对象与13/11链接目标警告，保留日志，不宣称macOS13兼容已验证；没有启动新app或操作general剪贴板。

## 边界

未启动新的物理Windows↔Mac原生应用或改宿主防火墙、路由、代理、权限；未做本次物理系统剪贴板双向闭环、系统文件关联点击或新Mac包实测。原0.6.0 Release/tag/资产和生产信令保持原来源。此前A–E记录的外部阻塞与未验项保持；本轮`get_goal`返回null，未创建新Goal或声称整体Goal完成。本轮与旧包原生证据分别记录。
