# 2026-10-18 N2 原子多设备分发

实现契约见[ADR0010](../adr/0010-atomic-device-batches.md)，当前状态只在[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)维护。测试profile、DB、原始日志和文件夹具位于忽略的`.artifacts/ae-20261018/`，不进入Git。

## 6ec3cc4准确Mac原生等待、源变化、独立重试与取消（2026-10-18）

本轮无产品源码修改，沿用`6ec3cc48d89552e2d681a6d3bef88540f2d4fa84`准确arm64包及五项CI/三平台四资产核验；EXE SHA256 `e13f0391f5074996c74fdd7e6ebedd241f0966612a827f9ce676cef39cee3e87`。新根`/Users/wen/.codex/supervision/linksend-ae-20261018/batch-native-6ec3cc4`复制已停止测试A/B/C profile、准确app与一致性控制库，保留私有备份，正常配对关系来自原Initialize/Invitation/Join。三个软件identity表示发送端A和两个软件目标B/C，不能当作三台物理机或三目标原生验收。

独立C正常签名Devices确认A/B在线、C离线。原main保存的B目标保持，实际展开“多设备分发”并原生勾选C，Go持久草稿peer_ids变为有序[B,C]/revision2；未勾等待时批次按钮禁用，原生勾等待后启用。真实系统picker添加batch-source.txt，保留原held-main.txt，main revision3；quick整行与原任务队列保持。原生只点击一次“分发给2台设备”，经既有EnqueueBatch创建一parent/两个独立child，主草稿仅在成功后清空为revision4，B/C目标仍保持，quick原revision4整行不变。

第一批`3f1b7e24277c46d759c1277ee5284d9d`：B child `b4f761c4fb7d7543f4d34c076ed9ac2c`实际完成74B/两个文件/hash/双方确认，C child `75bc34b87c74e084e75bbb01d96c2638`为waiting_peer且尚无task。随后只修改专用batch-source.txt并保留原字节副本；C上线后被SOURCE_CHANGED阻止发送变更内容，留下真实failed/SOURCE_CHANGED task `20261018T050825.617903000Z-00000002`，C无提交文件，B完成row/task/文件均不变。原生菜单退出/真实重启后同一C子项仍needs_attention/SOURCE_CHANGED，B与主/quick草稿保持，没有重复批次或自动发送。

原生队列确实显示“源内容已变化，请核对原始文件后确认。”，实际只点击该C行“确认继续”。其源摘要重新绑定新内容，同一child复用；C一次性接收器已在前一次源保护的连接中退出1/DIRECT_FAILED，确认后子项正常等候离线对端。观察器等完成10秒exit1后，先核对该job终态与waiting_peer，没有再点击确认或重新入队；仅重开C接收器，原child完成98B/两个文件/hash/双方确认。B task/queue完全保持，没有重新发送B；B收到旧36B版本、C在明确确认后收到新60B版本，不宣称批次是冻结快照。

| 来源 | 字节 | 实际源/目标SHA256 |
|---|---:|---|
| held-main.txt（B/C相同） | 38 | eb2adf72e84bb5e0ce1613f1d76311504bbbc218a98c93787cf15cb869b8d6a5 |
| batch-source.txt（B确认的原版本） | 36 | b4330cbcfb3ffdb18fab15b544d3b3dcbc4758415b7d0bae597877d7cbfcc7ba |
| batch-source.txt（C重新确认的新版本） | 60 | aff3afd4c3cf0f12dc7b0c1c6831223453317c6e8cef7b94004473894d636506 |

B sender `20261018T050731.504112000Z-00000001`/transfer `2da09189add4219a1e96a0084d720280`；C新sender `20261018T051201.929417000Z-00000001`/transfer `d369a9ffd40b2a8c9482abe8abc23877`。两次均为正常Pion/固定身份QUIC、bilateral_confirmed=true、relay=false/direct_unknown；实际每个接收目录各两份唯一文件，排除受管暂存后没有额外副本。旧失败task仍保留，不能把最终批次完成解读为历史没有失败。

第二批`886eee4a9bd750b8aa1789be9a05e032`：原生设置256KiB/s以保留有界活动窗口，真实picker选择专用16MiB文件，main revision5；继续B/C两个明确目标并勾等待、真实提交。B子项处于running且接收暂存已建立，C子项queued/未开始且task_id空。原生先只取消C行：C持久cancelled且仍无task，B维持同一running task，未被替换或停止。随后原生点击唯一待完成批次的“取消此批次未完成目标”：parent.cancelled持久为1，剩余B task `20261018T051637.051124000Z-00000002`和两个child最终cancelled，B此次没有提交用户文件，第一批B/C的完成rows/实际hash全保持。

原生恢复不限速0后真实菜单退出/重启，send_batches/send_queue/tasks/send_drafts全部整行保持，取消意图未丢失、没有重复parent或派发。GUI最终原生退出释放profile owner后，才独占同profile用CLI执行`resume --task <上述已取消B task> --bind 127.0.0.1:0 --allow-loopback`：实际exit1/BATCH_CANCELLED，无新parent/queue、task仍cancelled。这里单列直接Resume负例，不扩大成新跑了所有直接Retry负例；既有后端取消屏障测试保留原范围。

终态为2batches/5queues（3completed/2cancelled）/5tasks（3completed/1个预期SOURCE_CHANGED failed/1cancelled），包含原有completed queue/task各1。main revision6空，quick原revision4整行、identity字节和其他偏好保持；16MiB源hash、旧/新小源两版本与原profile/私有备份保留。四次receive均终态：B首次、C确认后exit0；C源保护连接、B批次取消各exit1/DIRECT_FAILED，保留原始日志，后两者不记作接收完成。专属rendezvous正常exit0、监听关闭，两库quick_check=ok，clipboard=false/enabled grants=0，未改宿主网络、生产服务或系统权限。

非敏感完整收据`.artifacts/ae-20261018/mac-batch-6ec3cc4-final-receipt.json`已下载。关键job统一前缀`/tmp/codex-ssh/job-ls-ae-mac-`：`batch-start-6ec3cc4-20261018-01`、`batch-signal-start-6ec3cc4-20261018-01`、`batch-select-c-wait-6ec3cc4-20261018-01`、`batch-picker-{goto,path-small,confirm-small}-6ec3cc4-20261018-01`、`batch-enqueue-first-6ec3cc4-20261018-01`、`batch-change-source-6ec3cc4-20261018-01`、`batch-source-change-check-6ec3cc4-20261018-01`、`batch-restart-first-6ec3cc4-20261018-01`、`batch-verify-c-first-6ec3cc4-20261018-01`、`batch-enqueue-cancel-6ec3cc4-20261018-01`、`batch-cancel-child-c-6ec3cc4-20261018-01`、`batch-cancel-parent-6ec3cc4-20261018-01`、`batch-restart-cancelled-6ec3cc4-20261018-01`、`batch-quit-probe-6ec3cc4-20261018-01`、`batch-final-cleanup-6ec3cc4-20261018-01`均exit0。`batch-confirm-c-first-6ec3cc4-20261018-01`已实际送达确认，但其完成观察器因接收器已退出而exit1；后续仅重新接收和只读复核，没有重复确认。完整脚本和原始日志保存在忽略artifact/对应jobPath。

本节证明准确Mac原生checkbox/picker/一批入队、混合在线离线、源变化反馈与独立确认、实际两目标文件、等待子项独立取消、活动批次取消与两次重启/取消恢复屏障。同机Pion/QUIC不替代物理LAN、多物理目标、双NAT或完整A–E；三目标自动fixture和旧6edddad Windows正常路径继续分别绑定原证据。

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

准确包增量：6edddad五项CI/三平台包和四资产核验通过。Windows原生界面从同一筛选草稿选择两个正常配对的CLI目标，实际一次创建一批两个子项并顺序Pion/QUIC完成，各1MiB/中文空目录/排除集合/hash和双方确认通过；不是三台物理机或完整原生失败/恢复矩阵。精确ID、时序、包来源和资源收尾见[原生检查点](DESKTOP-AE-NATIVE-CHECKPOINT.md)。
