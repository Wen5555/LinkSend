# Architecture

2026-09-14 E3 原生共享采用“短生命周期系统适配器 → 本机提交日志 → 唯一 Go owner”结构。Windows Share Target 与 Mac Share Extension 自己显示设备小面板；Go 将最多 256 个已授权设备的 `id/name/reachable` 快照发布到当前用户目录或正式 App Group。适配器提交 schema 2 记录后才向系统报告完成；Go 原子消费记录，调用现有幂等 `Enqueue`，提交数据库后删除记录。崩溃发生在入队与删除之间时，同一 request ID 重放只返回原队列项。

macOS App Group 同时承载设备快照、请求元数据和仅在临时表示场景生成的 owned 文件。原位文件以 security-scoped bookmark 交给宿主，宿主解析后保持 scope；Windows 当前只交接 broker 已验证且有绝对路径的普通文件。两端都不通过适配器 IPC 传正文。后台唤起分别使用受包身份约束的宿主进程和注册 URL；失败时请求仍已持久化，下次 LinkSend 启动继续消费。正式签名安装与冷启动结果仍需 E3/E5 准确包验证。

2026-09-14 E4-03：自动剪贴板 wire、原生读写和授权交集已有源码候选；实现与验证边界见 [E4行为证据](evidence/DESKTOP-E4-CLIPBOARD-BEHAVIOR.md)。

E0-ADR-review-v1提案将剪贴板并行解码与唯一原生提交owner分开：origin sequence用于来源去重，Lamport用于因果排序，OS generation加应用revision门闩防止两个候选共用过时检查。lease由接收端单调期限管理；不以应用锁声称macOS全系统剪贴板CAS已实现。

2026-09-12 M0 更新：Go1.27.1 + Wails3 beta.18，当前源码产品版本 0.5.0。
`internal/app.New` 在身份/SQLite 前取得 profile 内核排他锁，退出待所有 owner 结束后释放。
授权拒绝由唯一 identity trust schema 1 所有者持久化；应用的所有新连接、确认和恢复路径共用检查。
保存退出建立派发屏障并保留可恢复任务，迟到事件不覆盖控制意图。
设备/草稿/队列、接收计划与原生内容已整合至M5；用户后续要求在M5合并发布后停止，详见 [ADR 0003](adr/0003-desktop-ownership-and-local-denial.md)。

M1元数据现为task schema3：DeviceProfile、SendDraft及send_queue共用任务库，身份授权仍在trust。
队列先持久关联task再启动worker，准备时与派发时比较源摘要；重启需要确认，离线项不阻塞其他在线设备。
Wails只发送有epoch/revision的合并失效事件，React Query重取Go快照，命令无通用重试。
最低macOS13由用户明确授权，未声称当前macOS26.5原生检查等同macOS13实机。

M2 把原生文件入口接入同一 Go 草稿：Wails New 前持久化有界路径日志，首实例事务合并后确认，
第二进程不打开身份和数据库。通知失败由定时消费补偿；入队与发送仍需用户命令。
细节和原生证据见 [M2 执行记录](evidence/DESKTOP-M2-EXECUTION.md)。

M3 的桌面 owner 管理托盘、可选通知、防睡眠与原生关闭选择；Wails仍只注册App一个service。
后台选项不重启接收连接；真正退出先停止原生入口，再释放系统资源并有序关闭core。
Windows自定义确认使用经实际窗口验证的TaskDialog，而非beta.18仅支持Yes/No的Question实现。

当前产品版本为 LinkSend `0.4.0`，协议为 V1。根 module `github.com/Wen5555/LinkSend` 包含协议、身份、LAN 发现、信令客户端、Pion/quic-go 集成、文件传输、存储、应用服务、服务端和 CLI；`apps/desktop` 是唯一嵌套 Go module，使用 Wails 3 `v3.0.0-beta.18`。

```text
LAN 发现：签名 UDP 组播/广播/受限单播 → 临时 mTLS 控制通道 ┐
WSS 信令：设备/会话/候选
        └───────────────────────────────────────────────> Pion ICE：检查、提名、保活
        -> quic.Transport：唯一 UDP socket 所有者
        -> TLS 1.3 QUIC：可靠双向流
        -> transfer：manifest、缺块、校验、staging、提交
        -> app/store：任务状态、revision 与 schema 2 恢复元数据
        -> CLI / Wails DTO / React：一致的状态和稳定错误语义
```

`quic.Transport` 是 UDP 的唯一读写所有者；Pion 通过受限 STUN `PacketConn` 使用同一 base socket。文件协议不导入 signaling，服务器不导入 transfer，文件正文不进入 WSS、HTTP 或 JavaScript。Pion ICE 与 quic-go 的职责边界保持不变。

连接生命周期由 `session_id + generation + connection owner` 约束。客户端用单一后台读泵持有认证 WSS，业务等待取消不会关闭底层连接；桌面空闲接收与发送可借还同一连接。双方候选结束后服务端立即释放协商记录，因此连续 session 不需要重新 TLS/WSS 登录。断开或被替换的连接仍清理自己拥有的协商，健康 QUIC 数据连接不依赖 WSS 持续存活。

逻辑任务在恢复过程中保留 `task_id`，每次执行创建新 `attempt_id`，每次连接创建新 `session_id`，ICE generation 只在该 session 内有效；完整任务快照通过单调 `revision` 防止旧回调覆盖。公开状态为 Preparing、AwaitingAcceptance、Transferring、Verifying、Paused、Recovering、Completed、Rejected、Cancelled、Failed。Completed 要求接收端唯一块验证、提交成功以及 completed/confirmed 双边终态完成。

E4 的 direct session owner 以 peer ID 和本机当前授权 generation 为池键短时保留已认证 QUIC；双方通过ICE描述交换各自本地generation，剪贴板lease/event按方向使用对应值，不要求两端数字相等。一个文件任务对应一个双向 stream；连接两端都保持 AcceptStream owner，因此最初的响应方可以在同一连接上发起反向文件流。取消或 reset 只回收该 stream；授权撤销、路径/连接失败、3 秒空闲、空闲网络快照变化或进程退出关闭池中连接；活动连接只在真实 path watcher 失效时关闭。每 peer 仍至多一个活动文件发送流，不开放无界并发。文件复用要求双方显式声明session_reuse；剪贴板owner还要求双方显式声明独立clipboard_sync能力，旧端只声明session_reuse时不会解析剪贴板流。

同一复用连接另有唯一的剪贴板单向流owner，与文件双向流owner并行。接收方按当前receive permission revision续签lease；发送方只在持有对方有效lease且本机send grant仍为相同revision时读取一次原生剪贴板并发送。正文到达后先锁定header候选和原始deadline，再占用全局两个receive slot之一读取正文；图片解码等纯验证在状态机锁外完成。最终Commit按grant→state一致锁序重验application revision、OS generation、peer generation、receive permission revision、暂停状态和deadline，最后调用平台CAS写入。单流失败或过期不关闭文件连接；只要任一剪贴板方向ready，3秒文件空闲回收不会关闭连接。暂停或全部方向失效会恢复空闲回收。

桌面watcher使用容量1的latest-only变化队列；本机一次系统变化先推进唯一状态owner，再按可读格式准备一个origin事件并绑定各peer lease。发送worker异步执行、全局最多两路，每64 KiB复核新复制/暂停/撤权/连接/deadline并节流；会话Ensure按peer并行，变化交付不等待坏peer连接。首次启动、恢复、授权变更和每5秒尝试为已授权peer建立认证会话，不创建文件Task。会话建立后的lease以当前系统generation为baseline，因此离线、无权限、格式不支持或尚未建连期间的旧复制不会补发。

QUIC 成功终态使用 `completed → confirmed → confirmed_ack` 显式闭环，并兼容旧端以 EOF/application code 0 表示已读确认。终态完成后标准 QUIC close 仍发送，但 quic-go draining 和 endpoint 回收在后台进行；任何非零 close、reset、deadline 或普通传输阶段的 close 都不能转换为成功。响应端在发送 `connect_response` 前先注册固定身份的 QUIC listener，避免首个 Initial 因监听窗口尚未建立而等待 PTO 重传。

任务控制状态优先于同一 attempt 的迟到进度：Pause/Cancel 建立调度屏障后，ACK 可以补齐计数，但不能重新开放 `CanPause`、覆盖 `pause_requested` 或把任务误终结为 Cancelled。接收准备和测试同步使用明确的后端 phase/checkpoint 事件，不依赖固定 sleep。

恢复仍复用现有 V1 transfer 帧，不建立第二套协议栈。恢复身份绑定 TransferID、manifest digest、块策略、源内容、接收目录、对端身份/指纹；接收端重验 staging 和 commit records 后请求缺失或损坏块。`sent_bytes`/`received_bytes` 是实际数据面字节，`retransmitted_bytes` 是旧 attempt 已发送又重发的子集，`verified_bytes`/逻辑完成量只计算唯一已验证数据。

多网卡层自动枚举可用单播地址，并支持接口优先和排除；IPv6 link-local 当前排除。自动排序在普通 MTU 相同时优先非 point-to-point 接口，仍允许用户显式选择隧道；安全筛选结果按接口、当前 IP 和用户策略预热缓存，每次复用前核验地址仍存在，DHCP 变化会失效并重新发现。桌面保存的旧字面量 IP 回退自动选择。地址失效会废弃旧 endpoint 并进入新协商/恢复，不宣称 QUIC 透明迁移。

LAN discovery 对每个可组播 IPv4 接口加入同一受限组，Windows 无法提供入站 ifindex 时按实际本地 CIDR 验证来源；不会按 `Wi-Fi`、`en0` 或私网名称硬编码。收到对端签名公告的本地接口直接成为本次 ICE host 绑定提示，避免 TUN 抢占。组播被校园网/访客网络过滤时尝试每接口定向广播；仍失败可探测一个用户给出的 on-link IP。成功路由只按已验证设备记忆并定向续租，服务端 WSS/ICE 始终保留为已配对设备的回退。

连接请求/响应不等待完整候选收集；双方开始 gathering 后立即交换 ICE credentials，候选回调、签名转发和 Pion checks 并行。响应端前置建连失败通过受限、签名且绑定 session/generation 的 `status` 返回稳定错误码，避免发起端把明确的本地绑定失败误报为 30 秒信令超时。

真实网络证据见 [ADR 0001](adr/0001-ice-quic-integration.md)，任务/恢复决策见 [ADR 0002](adr/0002-task-recovery-and-history-schema.md)。独立双 NAT 固定映射场景已 PASS；MASQUERADE-only 仍 FAIL，因此不能宣称所有 NAT 类型均可直连。

M5 内容生命周期见 [ADR 0006](adr/0006-owned-content-lifecycle.md)：schema5 将内容草稿、队列引用与实际内容协商映射分离；Go 拥有不可变快照、重启恢复及按 task ID 的原生内容动作。短文字输入可经有界表单进入 Go，已读取正文与 PNG 不从 Go 返回 JavaScript。
