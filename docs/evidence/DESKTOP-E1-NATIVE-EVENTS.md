# E1-C3 原生网络与睡眠事件证据

候选源码：`codex/desktop-experience-upgrade`，以本证据文件所在的阶段提交为准。

桌面生命周期现把 Wails 3 beta.18 的 `SystemWillSleep`/`SystemDidWake` 原生应用事件接入 Go core。Windows 网络变化使用 `NotifyIpInterfaceChange(AF_UNSPEC)` 并以 `CancelMibChangeNotify2` 注销；macOS 使用 `SCDynamicStore` 订阅全局与各接口 IPv4/IPv6 state key，在独立 CFRunLoop 上回调，并在关闭时 stop、wake、join 后释放 handle。所有回调先进入容量1的串行事件泵，避免阻塞系统回调并合并突发事件；core仍保留250ms门闩及5秒真实接口快照兜底。关闭顺序先注销Wails/系统订阅并等待事件泵退出，再关闭core，避免回调访问已释放服务。

Windows本机专项：桌面 `GOWORK=off go test ./... -run 'TestNative(SystemEventPump|NetworkMonitor)' -count=1`、`go vet ./...`、`go build ./...`，前端typecheck/lint/58项测试/build，以及 Wails 3 beta.18 `windows:build` 均PASS。注册/停止测试调用真实 `NotifyIpInterfaceChange`/`CancelMibChangeNotify2`，事件泵回归覆盖关闭后不再投递和订阅stop幂等。

Mac别名 `mac-test-102342413` 已按用户最新地址更新为 `wen@10.234.212.116`；manager resolve/probe/audit PASS（macOS 26.5 arm64）。固定源码提交 `844e286` 的干净archive SHA256为 `946378e8d867a5a7853f524353f723297a2e6b39672c11ed10a819195b82894e`。远端 `GOWORK=off go test ./... -run 'TestNative(SystemEventPump|NetworkMonitor)' -count=1`、`go vet ./...`、`go build ./...` 全部退出0，真实创建并停止 `SCDynamicStore`/CFRunLoop monitor；保留作业 `/tmp/codex-ssh/linksend-e1-c3-mac-build-final-20260913T114047Z`。链接器报告部分Wails/CGO对象的deployment target警告，但没有未解析符号或运行失败；这不等于准确DMG的最低macOS运行验收。

本项源码和编译通过不等于物理网络切换或睡眠唤醒通过。准确候选包上的Windows/macOS事件到达延迟、DHCP/Wi-Fi/VPN变化、睡眠恢复后LAN/跨NAT重连与文件hash仍须独立实测；健康QUIC不因通知本身被取消。

阶段提交 `248e1ee24069800ef7416cce59eb0610d8c41b04` 的GitHub core、desktop、Windows amd64、macOS arm64/amd64包检查全部PASS；总控接受C3源码及两平台注册/停止和构建闭环。物理项状态保持上述NOT RUN。
