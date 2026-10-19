# R3：可操作诊断与本地白名单导出（2026-10-19）

基线`565840ec157f933f3d3dacb5100130e6a2c1ea2e`。Go安全投影、任务/记录入口和用户主动选择位置的JSON导出已实现；源码回归与浏览器布局通过。01b5c16准确Windows候选的原生保存/取消和任务导航已验，Mac旅程仍待，R3暂不打整体完成勾。

## 行为与兼容

- Go按稳定码给出原因、阶段、下一步及导航动作，区分发现/配对、在线、协调、候选、ICE、QUIC/身份、接收确认、磁盘、源变化与恢复。未知码不解析任意日志前缀，未知信息显示未知；已完成任务投影不沿用旧连接错误，不修改独立剪贴板/native错误owner。
- TaskSnapshot与InboxItem追加可选安全诊断DTO，界面按需展开阶段、稳定码、连接方法、版本和最近尝试。导航到既有设备/网络/接收/更新/诊断入口，重试/继续保持原授权、队列和幂等约束，不自动派发。host/online/空方法显示未知，仅后端已证实direct_unknown显示路径待确认；health成功不等于文件连接。
- 创建任务/明确开始恢复才记录attempt_started_at；旧记录缺失显示未知，不用UpdatedAt冒充尝试。既有snapshot JSON可选增量，旧端忽略未知字段；schema仍12，无文件wire、权限或额外DB owner变化。投影不写回私有task owner。
- 报告v1从白名单重新构造，最多32条/128KiB：本机编译版本/平台、生成时间、有限health/恢复状态与临时task-N安全摘要。不直接序列化Diagnostics、TaskSnapshot、recovery、cache ledger或未来ShareGrant；不含稳定身份、公钥、URL、候选地址、会话/文件摘要、文件名、完整路径、凭据、邀请码/分享码、正文或原始异常。
- 只有“导出本地诊断”调用SaveFile。先构造安全DTO，再在所选目录O_EXCL创建临时文件、Sync并以原子不覆盖Link发布；已有用户文件保持。只接受绝对.json目的地，失败清理临时输出，前端只收saved/cancelled或安全码。不自动上传；health GET受3秒期限，不发送报告正文。
- Windows SaveFile复用已查证Wails beta.18 CFD精确取消处理，取消不写stale选择；普通失败保持但RPC不带原始路径/异常。没有缓存清理按钮、容量/期限设置或保存发送方案。

## 实际验证

Windows PowerShell，Go1.27.1/Node24.21.0/pnpm12.4.1，Wails beta.18。绑定由真实wails3 generate bindings -ts -i -clean=true生成，109个App方法，未手写绑定ID。

- 将独特伪密钥/路径/URL/身份/候选/错误及伪造旧Diagnostic注入所有私有字段，白名单JSON均不含它们；40条只输出32条，未知文本/非法时间不获展示权限。代表性错误/等待确认有正确原因动作，成功清旧连接建议，原始日志前缀不转业务状态；新尝试时间真实产生，旧UpdatedAt不冒充。
- 真实临时profile/文件系统导出生成可解析报告；第二次同路径返回EEXIST，原文件逐字节保持且临时文件不遗留；取消ctx/非法扩展不产出文件。桌面边界注入覆盖取消、保存、picker/写出错误、已有文件及Windows精确取消码，均不泄露伪私密异常或写旧路径；不冒充原生点击。
- 核心诊断/workspace/inbox组合race3轮退出0，r3-final-core-race.log，39.634秒。两模块workspace/off的test -count=1 ./...、vet ./...、build ./...共12命令退出0；核心完整test113.01秒/off108.47秒，app106.626秒/101.032秒。新增保存边界后又运行桌面双模式test/vet/build，最终dist后双模式embed build另核对。
- 前端typecheck（含绑定）、lint、18文件130项Vitest及build通过，r3-release-frontend-{typecheck|lint|test|build}.log。覆盖主动导出/重复点击合并、取消不显示成功、错误不显示私密异常、无bridge禁用、任务导航至网络设置及完成后旧错误消失。
- Playwright使用实际组件/CSS的隔离夹具，1280×900与900×700截图逐张查看。初次夹具漏必填phase而失败，补齐后console错误0；旧field-help负margin造成说明挤进进度行，已修正局部样式。修后间隔10px、无横向溢出、伪私密文本不可见，未知方法明确未知。

最终图/快照在output/playwright/reliability-r3-20261019/，desktop-diagnostics-final.png、compact-diagnostics-final.png；早期图/失败console保留。浏览器无native bridge，导出禁用；导航由夹具记录network，组件回归核对回调。这些只证明UI，不替代Wails原生保存、P2P、跨网或键盘粘贴。测试只用自有临时profile/伪数据，无日常剪贴板、用户正文、生产或宿主网络改动。

## 来源与接续

上一修复565840e的core [39174347344](https://github.com/Wen5555/LinkSend/actions/runs/39174347344)、desktop [39174347400](https://github.com/Wen5555/LinkSend/actions/runs/39174347400)、三平台包 [39174347352](https://github.com/Wen5555/LinkSend/actions/runs/39174347352)全部success；旧1bc92e0 Mac arm64失败保持。这组通过不转记为R3代码CI或原生通过。

01b5c16的三组CI及三平台包全部success；准确Windows候选原生保存/取消、报告脱敏回读及原任务到诊断设置的导航已通过，见[R4 Windows旅程](RELIABILITY-R4-WINDOWS-UPGRADE.md)。Mac原生仍待；实际旅程发现恢复阶段内部名可见，另行修复验证。R1物理LAN/恢复/跨网、R2完整原生回收/键盘粘贴、R4双平台完整安装退出、R5临时分享与R6同候选收尾保持必做。

实机后修正：taskPhaseLabel将实际恢复阶段restart_recovery_available显示为“已保存，等待恢复”，更新既有阶段回归和dist；不改变恢复授权、wire或schema。前端typecheck/lint/130项test/build及两模块workspace/off的test -count=1 ./...、vet ./...、build ./...共12命令全部退出0，日志r3-recovery-label-*。这是新源码检查，不能把01b5c16原生截图倒记为修后新包实机。
