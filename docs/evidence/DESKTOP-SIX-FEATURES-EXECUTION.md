# 六项桌面能力执行记录

启动日期：2026-09-12；基线 `fbfc250159ea65dbb5840d6dadb03fe4423cf868`；
工作分支 `codex/desktop-six-features`。初始未跟踪的规划、`.playwright-cli/`、
`apps/desktop/build/ios/` 与 `apps/desktop/build/linux/` 已保留。

用户本轮明确授权每阶段推送、部署和发布，覆盖总提示词中的默认禁止条款。
阶段产物发布为测试预发布，只有通过的实际结果列为 PASS。2026-09-12 又授权通过 SSH
在 Mac 安装软件并测试，当前给定地址为 `10.234.184.6`；连接沿用管理器现有 alias。

| 阶段 | 状态 | 当前内容 / 下一步 |
|---|---|---|
| M0 | IMPLEMENTED / RELEASE_PENDING | 官方依赖、升级、本地拒绝、profile 锁、保存退出及Windows构建已验证；精确提交的跨平台构建/部署/发布进行中 |
| M1 | PENDING | 设备元数据、类型化事件快照、持久草稿、幂等队列 |
| M2 | PENDING | Wails 单实例、拖放、SendTo、Finder Services |
| M3 | PENDING | 托盘、后台设置、通知与防睡眠生命周期 |
| M4 | PENDING | ReceivePlan、子集协商、原子保留两份、恢复 |
| M5 | PENDING | 文本/URL/图片快照、内容能力协商与接收动作 |
| M6 | PENDING | 分页收件箱、调度完善、跨功能/原生/包级验证 |

M0 依赖与原生源码核查分别见 [依赖](DESKTOP-M0-DEPENDENCIES.md)、
[原生](DESKTOP-M0-NATIVE.md)，所有权和迁移见 [ADR 0003](../adr/0003-desktop-ownership-and-local-denial.md)。

## M0 已执行

- 基线 Go1.26.5 `go test ./internal/app ./internal/identity`：exit 0。
- Go1.27.1 `go test ./internal/app -run 'TestPairingCodePersistentInboxAndAlwaysAccept|TestProfile' -count=1 -timeout=60s`：exit 0。
- Go1.27.1 `go test ./internal/app -run 'TestSaveExit|TestFailedConstruction' -count=1 -timeout=45s`：exit 0；在真实 QUIC 第一块确认后保存退出，超时期间第二 writer 被拒绝，重启保留实际块计数及恢复身份。
- 初次授权回归出现三项 fixture 失败：旧测试用无效 peer ID，及删除迁移后的 trust 再重建 pin。
  后者现应 fail closed，测试将改成显式替换身份的负向验证；不以关闭授权检查修复。
- 初次保存退出测试编译发现 StartInbox 返回值数目错误，修正后通过；不计为产品运行失败。

## 未验证边界

M0 最终提交前追加：根 `GOWORK=off go test ./...`、`go vet ./...`、`go build ./...`、
`go test -race ./...`，以及 workspace `go test ./...` 均 exit 0；双模块 mod verify、
桌面独立 test/vet/build 均 exit 0。首次完整测试因产品版本断言仍为0.4.0失败，统一为0.5.0后通过。
Go1.27.1 重编 Wails beta.18 后生成绑定 warnings=0；前端 frozen/typecheck/独立bindings/lint/11 tests/build 通过。
`wails3 task package ARCH=amd64 INSTALL_SCOPE=user` exit 0，Windows EXE/NSIS 产品版本0.5.0。
原生冒烟由 Win32 HWND 与实际 `WindowRuntimeReady` 事件确认窗口及renderer就绪，再发送WM_CLOSE，exit 0；
EXE SHA256 `4b35e597cfba6d6af18f3278fb55aaf0ff5597cdfb67abd080e3684042f9cde3`，来源明确为未提交测试快照。
最初用 MainWindowHandle 不能识别隐藏窗口；随后过早WM_CLOSE导致尚未完成的WebView2 controller被中止(exit1)。
工具改为等待真实runtime ready后通过，不用固定sleep冒充页面准备完成。此项只证明原生创建/正常关闭，不证明控件交互。
Mac隔离工具链官方哈希校验、profile锁正常退出/强杀真实测试已通过，完整新包待精确提交构建。

原生入口、托盘通知、Finder Services、真实截图剪贴板、安装卸载及完整跨功能矩阵待各阶段实现后验证。
浏览器预览、窗口存活、CI 编译和 loopback 不能替代相应原生或物理双机证据。
既有固定映射双 NAT PASS / MASQUERADE-only CHECK_TIMEOUT FAIL 均保留原作用范围。
