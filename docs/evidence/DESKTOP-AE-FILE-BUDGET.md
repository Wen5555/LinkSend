# 2026-10-18 N3 文件正文限速

契约见[ADR0011](../adr/0011-process-file-body-budget.md)，当前状态由[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)维护。它是Go正文预算，不是精确UDP线上带宽控制；没有改变ICE/QUIC、身份pin、hash、安全落盘或relay=false。

## 6ec3cc4准确Mac活动限速与控制验收（2026-10-18）

本轮无产品源码修改或新产品构建声明。继续使用`6ec3cc48d89552e2d681a6d3bef88540f2d4fa84`的准确arm64包，EXE SHA256 `e13f0391f5074996c74fdd7e6ebedd241f0966612a827f9ce676cef39cee3e87`，来源仍是已核验五项CI/三平台包/四资产。新根`/Users/wen/.codex/supervision/linksend-ae-20261018/budget-native-6ec3cc4`复制已停止的专用profile/app并保留私有备份；正常配对的独立B profile和控制库副本仅用于loopback，没有操作日常profile、正式信令或宿主网络。

原生“设置→文件发送”保存256KiB/s，磁盘字段262144B/s且其他偏好不变；通过真实系统picker向原main添加32MiB样本并保持原held-main.txt。真实主发送后，在活动中原生保存1024KiB/s，字段1048576B/s；只读100ms采样记录task确认字节和偏好变更。仅统计完全处于同一速率阶段的连续4MiB确认区间，排除变更后的首个混合区间和结尾合并ACK：

| 保存上限 | 稳定4MiB区间（秒） | 两样本正文确认速率中位数 |
|---|---|---|
| 256KiB/s | 16.211204、16.167924 | 259075.012B/s，约253KiB/s |
| 1024KiB/s | 4.078818、4.076241 | 1028638.645B/s，约1005KiB/s |

这是同机确认进度的粗略速率，不是UDP线速/物理LAN吞吐；两个区间不报告p95。32MiB加原38B文件共33554470B，经正常Pion/固定身份QUIC完成，双方确认、relay=false/direct_unknown；实际两目标文件唯一且hash一致，32MiB SHA256 `e09320c5b00b34bb704802136c599a95b3996332ba84d7c7f21112b6231b6bd0`。sender `20261018T043612.369646000Z-00000001`、transfer `de46aae88faeaebcb1ffc7528f9d639d`。已完成后才准备独立取消/解除样本，没有把结束后的设置修改冒充活动修改。

随后原生保存1KiB/s。重进设置时界面按当前1MiB/s自动选择MiB单位，初次KiB前置检查在输入前停止；以0.0009765625MiB/s的精确数值正常保存为1024B/s。真实picker选择4MiB样本、真实主发送后，接收端已建立对应`.linksend-<transfer-id>`暂存，未完成任何块。实际原生暂停到持久paused一次观测1.783964s；原生恢复同Task/Transfer的新attempt后，界面显示同一任务的活动暂停/取消控制，在仍为1KiB/s时实际点击取消到持久cancelled为1.881840s。源hash保持，接收目录没有已提交用户文件；之后真实队列入口取消该needs_attention项，防止后续确认重派。sender `20261018T044319.675476000Z-00000002`，没有将这条被主动取消的恢复任务记作完成传输。

暂停和取消分别使原始CLI receive及重开后的receive退出1并报告DIRECT_FAILED/关闭socket；这是上述有意中断的真实终态，原始日志保留，不记作接收成功。CLI走`ReceiveOnceDetailed`原始传输，不创建桌面接收task；早期观察器错误等待该行而exit1，核实调用路径与实际暂存后续行，未重复发送。恢复观察器还等待库中Transferring而exit1；当时原生界面已显示同一任务可取消，随后核对新attempt、相同TransferID及持久cancelled完成收据，不据此声明库阶段即时更新或已传完恢复字节。

最后单独新建4MiB任务，在1KiB/s、零确认块且接收暂存存在时，从原生设置关闭上限并保存0。一次“操作开始→完成”观测3.133620s，实际4MiB唯一目标/hash一致、双方确认、relay=false；SHA256 `2b07811057df887086f06a67edc6ebf911de8b6741156e7a2eb1416a4b8b1b2e`，sender `20261018T045000.922324000Z-00000003`、transfer `4f1800e09273dd8c5a52294f3c506b2a`。这是真正活动正文等待被解除的独立样本；操作耗时包含AX/UI及落盘，非核心RPC延迟承诺。

真实菜单退出/重启后0上限保持、没有重复派发，main revision7空、quick原revision4整行保持；旧completed task/queue整行、身份、其他偏好和源hash均保持。最终clone有4tasks/4queues（三completed、一cancelled，包含原各一项），clipboard=false/enabled grants=0、两库quick_check=ok；app原生退出、四次CLI receive均终态（两完成exit0、两有意中断exit1）、只读采样器正常终态、专属rendezvous exit0且监听关闭。原profile/文件/私有备份保留，没有更改OS剪贴板或系统权限。

采样器只读`budget-trace.jsonl`，15分钟上限后正常结束，不冒充覆盖其结束后的解除动作；后者使用独立操作/完成收据。初版统计误纳变更后首个混合区间，已从派生统计剔除并重算为每阶段两个稳定样本，上表是最终值。另一次保存脚本误用AppleScript保留名称buttons，在点击保存前停止，改名后重新核对精确输入/磁盘字段通过；不作产品失败。所有失败与原始时序日志保留。

完整非敏感收据`.artifacts/ae-20261018/mac-budget-6ec3cc4-final-receipt.json`已下载。关键job前缀`/tmp/codex-ssh/job-ls-ae-mac-`：`budget-start-6ec3cc4-20261018-01`、`budget-signal-start-6ec3cc4-20261018-01`、`budget-save-256-6ec3cc4-20261018-02`、`budget-save-1024-6ec3cc4-20261018-01`、`budget-picker-{goto,path,confirm}-6ec3cc4-20261018-01`、`budget-send-dynamic-6ec3cc4-20261018-01`、`budget-dynamic-receipt-6ec3cc4-20261018-02`、`budget-pause-6ec3cc4-20261018-01`、`budget-cancel-task-6ec3cc4-20261018-01`、`budget-cancel-queue-6ec3cc4-20261018-01`、`budget-send-release-6ec3cc4-20261018-01`、`budget-release-unlimited-6ec3cc4-20261018-01`、`budget-restart-6ec3cc4-20261018-01`、`budget-final-cleanup-6ec3cc4-20261018-01`均exit0；恢复点击本身已送达但其过严SQL观察器exit1，后续原生取消/新attempt核验单列。其他动作及生成脚本保存在忽略artifact，核心wire/身份/hash路径保持。

范围仅准确Mac原生N3及同机QUIC。OS剪贴板与限速并存、真实双机/物理速率、完整A–E仍未完成；旧6edddad固定预算结果保留原提交，既有内存剪贴板fixture不升级为本轮OS证明。

## 已验证行为

进程共享token bucket覆盖所有文件发送、队列、批次与恢复；默认0不限速、有效上限1KiB/s–1GiB/s。100ms突发、最少1KiB/最多64KiB，闲置不累计无限额度；非零动态修改不新造预算，改0唤醒等待。预算等待只持内部短锁，可被task context立即取消。应用级重复文件块再计费，控制与自动剪贴板不经过该预算。

假时钟覆盖第二发送者共享余额、50/100ms补充、长时间闲置容量、重复/动态保存不充值、改不限速唤醒、取消与timer停止、0/负数/过小/最大int溢出边界。帧夹具核对长度头和正文编码完全相同，真实Send/Receive中PreviouslySent所有块仍与总正文等量扣费，控制未计入。

桌面偏好分类sending只保存本字段，不覆盖名称/剪贴板；CAS冲突、非法值磁盘与运行时均不变、重开保持与旧偏好缺字段默认0通过。format1可选字段由旧程序忽略，回退旧版不支持限速，不能称仍限速；当前任务库11、wire不变。

## 实际命令与结果

| 范围 | 命令/本地日志 | 终态 |
|---|---|---|
| 核心独立模块 | `GOWORK=off go test -count=1 -timeout 6m ./...`及vet/build；`n3-verified-core.log` | PASS，app79.594s/transfer10.650s |
| 完整并发 | `go test -race -count=1 -timeout 6m ./internal/app ./internal/transfer`；`n3-verified-race.log` | PASS，app195.242s/transfer10.617s |
| 桌面两模式 | workspace test、off-race/vet/build；`n3-verified-desktop.log` | PASS，workspace5.820s/off-race8.625s |
| 前端 | 固定Node24.21.0/pnpm12.4.1 test/typecheck/lint/build | PASS，11 files/101 tests；最后CSS增量重build |
| 真实绑定 | Wails beta.18/Go1.27生成；`n3-bindings.log` | PASS，95methods/66models，无前轮旧toolchain警告 |
| 真实QUIC/独立剪贴板 | `go test -race -v -count=1 -timeout 2m ./internal/app ./internal/transfer -run '^TestFileBody'`；`n3-real-first.log` | PASS，app11.637s/transfer2.200s |
| 首offer前连接过期与限速组合 | race五轮；`pool-expiry-and-budget-green.log` | PASS，58.504s；原代次固定后定向三轮9.410s |
| 最终集成增量 | `go test -race -count=1 -timeout 2m ./internal/app -run '^(TestPooledPeerRemoteIdleCloseBeforeOfferReconnectsOnce|TestFileBodyLimit|TestBatchCancel)'`；`n3-final-combined.log` | PASS，15.894s |

完整core/race先于连接过期修复，后两行覆盖该新增路径与取消/限速组合；不把旧二进制测试当新路径执行。最新源码CI/准确包另外核对。

1MiB文件、64KiB块、256KiB/s：16块首块后至末块为3.7633651s，理论余下960KiB约3.75s；接收hash与实际正文大小一致。另一次活跃发送在首块后改0，剩余正文及时结束。1KiB/s文件处于正文等待时，真实认证QUIC上的内存剪贴板adapter在7.1292ms写入；随后Pause、Resume、Cancel各自完成，源文件保持。均是同机小样本，未触碰OS剪贴板，不报告p95或物理两机结论。

## 界面与资源

Playwright独立session ae-budget、生产SettingsPage/FileSendLimit/CSS与明确mock偏好；960×640浅色、1100×720深色无水平溢出，保存按钮在视口内。空值显示错误并禁保存；Tab/字母/Enter切换单位保留精确值，MiB↔KiB换算核对。初次深色焦点select背景白、低对比，已以局部主题token修复并重新截图查看；原始失败截图保留，最终截图在`output/playwright/ae-budget-20261018/*-fixed.png`，不入Git。

0 errors/1 warning，warning为Wails浏览器模式提示；未运行原生保存RPC。owned浏览器/Vite及两份临时入口收尾后核对无监听；原生Windows/Mac、物理限速/剪贴板和A–E其他缺项仍继续。日志/profile/原始正文全部位于忽略的本地artifact，不提交。

## 阶段来源

源码`038e48190427e898a6934c18804b1fd702ba7e9d`已推送，远端一致。core push/PR39080943294/39080947127、desktop push/PR39080943298/39080946995、包39080943292均success；Windows/Mac arm64/amd64三个job均success。构建不等于准确包物理通过。N2/N3的Orion writer收据linksend-ae-n2-n3-038e481-20261018-01和增量索引已核验，项目文件/index SHA一致；后续包成功为本节新增观察。

准确包增量：6edddad Windows原生保存256KiB/s、实际偏好262144B/s，顺序批次向两个CLI各发1MiB，实际接收hash一致；transferring→verifying分别3.979076/3.942472s，仅为含读写/校验开销的loopback粗略口径，非UDP线速/物理吞吐。动态活动任务与OS剪贴板原生矩阵仍待验。来源、双方快照和收尾见[原生检查点](DESKTOP-AE-NATIVE-CHECKPOINT.md)。
