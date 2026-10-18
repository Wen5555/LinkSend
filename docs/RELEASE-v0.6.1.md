# LinkSend v0.6.1

发布日期：2026-10-18。产品版本`0.6.1`，文件协议V1、`relay=false`。

## 本次修复

- **传输记录**：点击查看文件后展开并定位清单，支持直接打开文件/文件夹或打开所在文件夹；发送记录也能定位原始来源。缺失、未完成接收或发生变化的文件会明确报错。
- **自动剪贴板**：修复原生窗口未就绪时注册监听、Windows PNG格式识别和复制时短暂占用导致漏同步；暂停/锁屏恢复后立即续期授权，避免7秒续期晚于3秒空闲关闭。设置页可逐台开启文字、链接、图片双向同步，两端都需保存总开关并开启对应设备权限。
- **设备管理**：删除立即忘记本地配对及偏好，可重新请求配对；黑名单独立拒绝连接，不再保留“已删除设备”目录。局域网发现列表在跨网络配对下，手动IP放在“附近没有设备出现，或没有想要的设备？”折叠入口。
- **统一传输队列**：发送和接收在同一页显示速度、进度、取消与批次控制，支持清除已完成记录或取消并清空全部任务。默认退出清空本页并取消未完成任务，可在通用设置关闭；文件与历史保留。受信设备新增接收显示导航红点计数，进入传输页后清零。

## 升级与使用

关闭旧程序后升级，保留原profile和接收文件；首次启动将按默认设置清空旧传输页待办，历史仍在“记录”。升级前请先完成当前未结束任务；如希望后续退出时保留待办，请在新版本通用设置中关闭自动清空。普通删除不删除对方的服务端组成员；重新连接仍须明确配对，旧授权与免确认不会恢复。

自动剪贴板不会补发离线、暂停或锁屏期间的旧内容。每台设备的总开关与对应设备权限均默认关闭。文件关联仅在明确点击“打开文件”后调用，不会自动执行收到的文件。

## 验证与限制

修复已通过119项前端测试、两Go模块workspace/off测试与构建、相关race检查，以及Windows生产构建和Mac arm64定向测试/编译；剪贴板暂停恢复故障有真实认证QUIC回归的失败→通过证据。准确发布源码与三平台新包的CI、版本、架构和SHA256已记录在随包发布的RELEASE-MANIFEST.json与SHA256SUMS.txt中；此前本地可用收尾不自动当作新发行包的物理验证。

Windows包未代码签名，macOS为ad-hoc签名、未公证；最低macOS13和Intel原生范围仍有限。当前Win↔Mac物理系统剪贴板双向闭环、系统文件关联点击、Windows托盘/系统分享及完整双NAT未新增验收，不宣称这些缺项通过。本次不部署生产信令或改动宿主防火墙、路由、代理与权限；不提供中继或TURN allocation。

详细实现与命令见[工作流修复记录](evidence/DESKTOP-060-WORKFLOW-FIXES.md)、[ADR0013](adr/0013-desktop-transfer-workflows.md)和[唯一TODO](DESKTOP-EXPERIENCE-TODO.md)。

## 发布来源与资产

- [Release](https://github.com/Wen5555/LinkSend/releases/tag/v0.6.1)已公开，Latest，非Draft、非prerelease；tag与新包源码均为`72161e3f806d742d1d401564031ab4844fe88b2d`，PR [#10](https://github.com/Wen5555/LinkSend/pull/10)已正常合并main。
- [三平台包CI](https://github.com/Wen5555/LinkSend/actions/runs/39113644308)及准确源码core/desktop检查全部通过；六项资产GitHub digest均与本地SHA256一致。
- 两Mac DMG只读挂载核验主app/Share Extension版本、arm64/x86_64架构、最低OS元数据和严格签名，挂载已卸载；未启动新包，不替代物理验证。

| 平台 | 资产 | SHA256 |
|---|---|---|
| windows-amd64 | [LinkSend-v0.6.1-windows-amd64-72161e3f806d742d1d401564031ab4844fe88b2d.zip](https://github.com/Wen5555/LinkSend/releases/download/v0.6.1/LinkSend-v0.6.1-windows-amd64-72161e3f806d742d1d401564031ab4844fe88b2d.zip) | `db68c623ef351057275f8fd193baefd901792c970839eaccc378b83ba2245a32` |
| windows-amd64 | [LinkSend-v0.6.1-windows-amd64-installer-72161e3f806d742d1d401564031ab4844fe88b2d.exe](https://github.com/Wen5555/LinkSend/releases/download/v0.6.1/LinkSend-v0.6.1-windows-amd64-installer-72161e3f806d742d1d401564031ab4844fe88b2d.exe) | `ad3adaa4c446d49df11b324cf2aebcc70c7bbc8ea202ecc94d252f7854df2628` |
| macos-arm64 | [LinkSend-v0.6.1-macos-arm64-72161e3f806d742d1d401564031ab4844fe88b2d.dmg](https://github.com/Wen5555/LinkSend/releases/download/v0.6.1/LinkSend-v0.6.1-macos-arm64-72161e3f806d742d1d401564031ab4844fe88b2d.dmg) | `ef06abefdc12c3475748d3ad61e99c0e6dc1f1aab98f56a7d284d5368605edf6` |
| macos-amd64 | [LinkSend-v0.6.1-macos-amd64-72161e3f806d742d1d401564031ab4844fe88b2d.dmg](https://github.com/Wen5555/LinkSend/releases/download/v0.6.1/LinkSend-v0.6.1-macos-amd64-72161e3f806d742d1d401564031ab4844fe88b2d.dmg) | `204a752d39e39156cae7bf60435d0fed8776f7a8660d8172ece4e42375e8d3bd` |
