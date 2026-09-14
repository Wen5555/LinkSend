# E1 Windows↔Mac 物理LAN最小闭环

固定产品源码：`e896f92a1e85b98a030984065507228ea951b3a2`。Windows 11 amd64物理地址 `10.234.232.205/16`；Mac别名 `mac-test-102342413`、macOS 26.5 arm64、`en0=10.234.212.116/16`。两端使用全新隔离profile和UDP/53418；临时Go验收入口只调用同提交的真实 `internal/app`/discovery/TLS/ICE/QUIC/transfer实现，不提交产品，也不替代准确Wails包的UI验收。未部署或修改香港服务、宿主防火墙、路由、代理。

首轮固定 `666cbaa` 真实发现和mTLS后，LAN同意因双方约1秒时钟差触发持久层 `now+60秒` 重验并以EOF结束；签名层原本已约束60秒TTL和15秒时钟偏差。修复 `e896f92` 将本地持久上限对齐为75秒，新增74秒接受/76秒拒绝测试；wire签发期限仍为60秒。

修复后从全新profile执行：双方发现目录分别记录真实远端/本地物理地址；Windows发起独立LAN同意，Mac明确接受，双方落盘 `grant_kind=lan`。Windows→Mac发送1,179,648 bytes完成，Windows源和Mac接收文件SHA256均为 `8b1dc5b2d94b705ec68e60a4622c5c0f191b4fcb20750938b8a974df17a336d1`。Mac只读复核作业 `/tmp/codex-ssh/linksend-e1-forward-verify-20260913T125341Z` 退出0，报告1个LAN trusted peer。

反向Mac→Windows没有通过：Mac发送任务在 `lan_control_connect` 约1.5秒后进入 `signaling_connect`，随后 `DIRECT_FAILED`，0 file bytes。独立验证在Windows物理地址临时监听TCP/54446，Mac连接3,002ms timeout，Windows `AcceptTcpClientAsync` 15秒也为timeout；作业 `/tmp/codex-ssh/linksend-e1-mac-to-windows-probe-20260913T125424Z` 退出1。该结果与既有Windows入站策略限制一致，但本轮未修改防火墙，故网络状态为PARTIAL，不能声称双向闭环或准确桌面包通过。

Mac隔离运行作业 `/tmp/codex-ssh/linksend-e1-live-e896f92-mac-20260913T124836Z` 因协调端在反向失败后未发送stop而最终达到3分钟测试deadline；这不覆盖已独立复核的正向文件结果，也不改写为整体PASS。准确 `e896f92` CI包的双向桌面操作、系统确认入口、IPv6/mDNS、网络切换/睡眠和双NAT继续NOT RUN。
