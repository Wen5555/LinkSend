# 2026-10-18 N2 原子多设备分发

实现契约见[ADR0010](../adr/0010-atomic-device-batches.md)，当前状态只在[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)维护。测试profile、DB、原始日志和文件夹具位于忽略的`.artifacts/ae-20261018/`，不进入Git。

## 行为证据

- `TestBatchAtomicRollbackAndConcurrentIdempotency`：SQLite trigger在第三个子项执行`RAISE(ABORT)`，parent和全部子项为0；8个并发相同请求只产生一批。参数冲突拒绝、草稿清空后返回原收据。
- 预检负例覆盖重复目标、撤权、未明确等待的离线目标，多目标不能借普通Enqueue部分入队。迁移保留schema10单目标，损坏peer_ids明确拒绝。
- `TestBatchMixedTargetsAndOnlyRejectedTargetRetriesOverQUIC`：真实生产Pion、固定身份TLS QUIC三目标同机夹具，混合上线、第二目标拒绝、第三目标成功；仅确认第二目标后独立重试成功，目标hash、中文、空目录和N1排除保持。只检查真正提交目录的副本；保留的`.linksend-*`接收暂存目录不计作意外提交副本。
- `TestBatchSourceChangesBetweenTargetsNeedIndependentConfirmation`：派发之间改变源，后续子项需独立SOURCE_CHANGED确认，已成功目标不变。
- 活动取消、源文件保留、取消重启、逐目标重启确认、后续撤权不抹成功、直接Resume/Retry不绕过取消通过。durable完成优先于晚到取消，完成回执修正会通知界面刷新。

最早一轮混合目标等待超时没有足够细粒度状态，根因未确认，不能称已定位。后续增加诊断；一次“重复副本”失败实际把受管暂存目录算作已提交副本，收窄到提交目录后验证。最终混合目标race五轮PASS（32.528s），不以旧失败日志冒充终态或删除其事实。

## 最终本地命令

| 范围 | 命令/日志 | 终态 |
|---|---|---|
| 核心独立模块 | `GOWORK=off go test -count=1 -timeout 6m ./...`及vet/build；`n2-verified-core.log` | PASS，app83.156s |
| 完整应用并发 | `go test -race -count=1 -timeout 6m ./internal/app`；`n2-verified-app-race.log` | PASS，225.334s |
| 桌面 | workspace test、off-race/vet/build；`n2-verified-desktop.log` | PASS，7.862s/7.859s |
| 固定前端工具链 | Node24.21.0/pnpm12.4.1 test/typecheck/lint/build；`n2-verified-frontend-*.log` | PASS，11 files/100 tests |
| Wails绑定 | beta.18 generate bindings；`n2-bindings.log` | PASS，95 methods/66 models |
| 最新完成通知回归 | `go test -race -count=1 -timeout 3m ./internal/app -run '^TestBatch'`；`n2-final-batch-race.log` | PASS，23.751s |

完整race后只增加晚到完成通知与对应故障夹具，最后一行覆盖该增量。批次race三轮58.040s和owner/race23.343s另有日志，不重复执行已稳定的大样本。

旧`0499cb5`导出源码中history.go的Git blob已核对，schema10真实库只读打开schema11准确返回`TASK_STORE_VERSION`且DB hash不变；helper SHA256 `49cf1a4033121141927149d3bba2af1293f4be5719da2d0c0a33a07ae6d9762e`，`n2-old-schema-probe.log`（5.197s）。这不是旧原生应用升级验收。

## 浏览器与资源终态

Playwright技能独立session ae-batch，生产BatchTargets/Queue/CSS与明确mock元数据，960×640浅色、1100×720深色无水平溢出，10条分页与第二页显示正常。Tab/空格恰好一次移除首目标，搜索后同样可移除已撤权目标，revision7→8→9；批次摘要显示1/3完成、独立确认与取消入口。截图实际查看，内部工作区可滚动，不宣称所有操作同时位于小窗口视口。截图在`output/playwright/ae-batch-20261018/`，不入Git。

console为0 errors/1 warning，warning是Wails自带Browser Environment Detected提示。两次用于进一步滚动截图的临时CLI片段解析失败，未形成额外截图/动作，不算新的检查结果。owned浏览器PID153812/session与Vite exec91603已停止，临时HTML/TSX移除；保留原`.playwright-cli`及ios/linux目录。无系统网络或剪贴板修改。

## 阶段同步与Windows CI失败修复

N2源码`77b831bd8897c5e567a706af82ffba9cd49bfba3`已推送且远端SHA一致。core push/PR `39079813148/39079816009`、desktop push/PR `39079813113/39079815913`均success。包run `39079813168`为failure：两Mac job success，Windows在Verify core module的旧`TestRemoteCancelledStreamKeepsAuthenticatedSessionForNextFile`失败，下一sender为opening_stream/DIRECT_FAILED；Windows包未生成，不称三平台包全通过。

原始CI没有足够时间线确认其准确关连接者。进一步故障夹具在缓存连接检查后、首个offer前令对端关闭，确定复现同一opening_stream/DIRECT_FAILED（`pool-expiry-red.log`）。修复仅在OpenStreamSync失败且连接已关闭、context仍有效时，最多新建一次认证连接，固定原authorization generation；尚无offer/body，不重放已发起的传输。取消/撤权不借新授权，发送开始后仍走原恢复语义。

故障夹具、原远端取消测试与N3真实限速/控制组合race五轮PASS（58.504s）；补充固定原generation后另有最终定向回归。该修复另作源码提交、核对CI，不能把原失败run改称success。原生Windows/Mac RPC、物理多目标与双NAT均未由上述夹具证明；N3/N4和其他A–E缺项继续。

修复提交`290c82862c9390ef180744dde4c31262e1dddd3d`已推送、远端一致，最终定向race三轮9.410s。core push/PR39080523720/39080527175、desktop push/PR39080523684/39080527191及包39080523740全部success，三平台包job均success。原77b831b的失败保留；该后续准确构建才恢复阶段包检查。
