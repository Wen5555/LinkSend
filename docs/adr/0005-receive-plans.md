# ADR 0005：不可变 offer、协商子集与持久接收计划

日期：2026-09-12。状态：M4 传输层基础已实现；App/UI 接线与真实跨设备原生验收另行完成。

## 决策

V1 的原始 Manifest、TransferID、chunk 策略和 manifest digest 不变。`ReceivePlan` 在接收端保存原始 digest、已选条目及各自目标相对路径、冲突策略；另一个 `SelectionDigest` 只绑定原始 digest 与已排序的选中 ID 集合。选择接收不伪造一个缩小后的 manifest，也不减少请求后仍宣称原 offer 全量完成。

`Directory` 是 ReceivePlan 的 `json:"-"` 本地字段。它可由用户在看到 offer 后选择，必须由 App 保存到既有私有恢复记录；不会写入计划 JSON、QUIC acceptance 或信令。目录相对映射、选择与冲突决定保存在根目录下已有 `.linksend-<TransferID>/state.json`，没有建立第二套块恢复引擎。

## Go 接口

保留 `Send`、`SendWithHooks`、`Receive`、`OpenReceiver` 默认全量接口。新增：

```go
ReceiveWithOptions(ctx, stream, ReceiveOptions{Directory, Peer, Accept, Plan, PlanChanged, Progress})
BuildReceivePlan(ctx, directory, manifest, PlanRequest{SelectedIDs, ConflictPolicy, Policies})
LoadReceivePlan(ctx, directory, peer, manifest) (*ReceivePlan, error)
OpenReceiverWithPlan(ctx, directory, peer, manifest, plan)
receiver.Plan() ReceivePlan
receiver.Summary() PlanSummary
```

`Plan` 回调返回明确同意后的计划或错误；它替代该次旧 `Accept` 布尔决策，不会先接受后再选文件。回调收到的 Offer 包含深拷贝的 Manifest、对端 capabilities、可选 `ResumePlan`。修改确认 DTO 不会修改原始 manifest。

`PlanChanged` 在传输层 checkpoint 已成功后、发 acceptance 或按新名称提交之前调用，可将最终本地 Directory/Plan digest 同步到 App 的私有任务恢复记录。回调失败返回 `RECEIVE_PLAN_PERSIST_FAILED`，不能继续请求正文或提交新名称；不能以乐观 UI 宣布已保存。

`SelectedIDs=nil` 代表全量；非 nil 空数组代表全部跳过。显式选择目录会递归选择其后代；仅选择一个文件则只补齐必需祖先目录，不选祖先的其他后代。`Policies` 可覆盖单个条目的全局冲突策略。输入和目标路径沿用 NFC、便携大小写冲突、Windows ADS/保留名、深度/长度与 root-constrained 策略。

计划持久化 `RequestedIDs`，记录过滤现有冲突前的请求集合。因此同一“全量 + 跳过冲突”请求重启后会复用原先已跳过的结果；即使被跳过文件后来消失，也不会自动扩大授权集合。`BuildReceivePlan` 与 `Offer.ResumePlan` 都会优先提供持久映射，避免把上次已提交文件再次编号。不同选择集合要求新任务或显式处理，不静默修改恢复范围。

## 兼容的 V1 协商

新 sender 在既有首个 offer 上添加可选 `capabilities:["receive_plan_v1"]`。不在 offer 前插入新 hello，旧 receiver 忽略该字段后继续旧 accept。未知可选能力有数量、长度与控制字符限制，不因为出现新能力名称就升级权限。

仅当 offer 明确包含能力时，新 receiver 才在 accept 中附加：

```json
{"selection":{"version":1,"ids":[0,2],"digest":"<selection digest>"}}
```

示意 ID 必须满足实际 manifest 的祖先目录闭包。selection digest 为固定字段顺序 JSON `{version:1,original_digest:<原摘要>,ids:<已排序ID>}` 的 BLAKE3；绝对目录和目标名称不参与线上摘要。accept 的原 `digest` 始终仍是 original manifest digest。

新 sender 读到旧 accept（无 selection）时按完整 manifest 校验与发送。新 receiver 收到旧 offer（无能力）时只允许全量；请求部分或全部跳过返回 `RECEIVE_PLAN_UNSUPPORTED`。本地全量重命名不需要发送方理解保存名称。未来原生内容能力可沿用 offer/accept 协商阶段；本 ADR 不提前实现或宣称 M5 内容能力。

协商后 sender 只响应已选 ID 的块请求，拒绝未选 ID；最终源文件再验证也只检查选中内容，未选文件变化不阻止已协商子集完成。旧 accept 保留原全量再验证。

新协商的 finish、completed、confirmed、confirmed_ack 均携带相同 `selection_digest`，其 `digest` 仍是原摘要。sender 必须看到匹配的显式 confirmed_ack；不能把未读到选择摘要的断流当作确认。旧全量会话保留已有 EOF/真实终态交付观察的兼容路径。

## 计数和全部跳过

`Progress.Total`、唯一 Verified、Committed 和 `Result.Bytes` 都针对已接受内容；Sent/Received 是实际正文数据，重传另计。`OriginalTotal`、`SelectedFiles`、`SelectedEntries`、`SkippedFiles`、`SkippedEntries`、`SkippedBytes` 明确区分原 offer 与结果，目录不冒充文件字节。

空的选中集合不进入 chunk 请求循环，但仍完成一致的 finish/completed/confirmed/confirmed_ack。双方返回 `State="NoContent"`、Bytes=0 和明确跳过计数；不能显示成“原始全部内容已传完”。显式选择空目录是有效内容操作，SelectedEntries 非零、Bytes=0，完成后该空目录真实存在。

`Receiver.Summary().RemainingBytes` 是在本次 staging/已提交文件重新校验后仍缺的内容字节；CommitBytes=0 因为提交使用同文件系统 hard link，没有额外正文复制。此值不包含目录/inode开销，也不消除恢复时防 hardlink 攻击的 staging 复制峰值。App 的空间预检必须另外保留开销余量；ENOSPC、权限改变和不支持 hard link 的文件系统仍返回实际失败，绝不降级为覆盖式提交。

## checkpoint、原子提交与恢复

原 manifest 保存在原 ResumeState.Manifest/Digest。计划另有 `receive_plan` 和 `receive_plan_digest`，后者覆盖请求集合、选择、目标相对路径与策略；加载时验证，避免损坏映射被当作恢复决定。使用计划的 checkpoint 将落盘状态标为 `ReceivePlanV1/<state>`，旧 binary 会拒绝未知状态，不能忽略新字段后扩大成全量恢复。新 reader 仍读取原无计划 state.json；旧格式完整任务可明确升级为同一全量计划。

现有块 checkpoint 的 fsync/批量位图与重哈希顺序保留；仅为已选文件建立 staging。重启后的已提交文件按保存的 path/hash/size 重新验证，不再复制成新的写入 inode；未提交 staging 仍复制验证块到新 O_EXCL inode，避免借 hard link 修改用户其他文件。

文件提交前先持久 `commit_intents`，再执行 `os.Root.Link` 原子 no-replace。Link 成功但 commit record 尚未持久化的恢复窗口，只在目标和 staging 是同一真实 inode/file ID 时收回该提交；同名或同 hash 均不是归属证据。明确 commit record 和目标摘要不匹配时失败，不覆盖或另取名字偷偷重交。

目录使用独立 `DirectoryRecord`：Windows 读取打开目录句柄的 volume serial/file index，Unix 读取 dev/ino；重新打开时校验实际目录身份。`CommitStarted` 不能证明目录已创建。mkdir 前意图已持久而目录 identity record 尚未持久的窄崩溃窗口，归属无法证明时返回 FILE_CONFLICT 并保留原计划，进入需处理，而不是合并进外部目录或另建重复目录。正常部分文件提交发生在目录记录持久之后，可以按原名称恢复。

保留两份预览使用原文件名加有界 `(1)`…`(100)`；真正提交前再次检查父目录中的大小写/Unicode等价冲突，遇到新的文件冲突在更名后的计划 checkpoint 和 App PlanChanged 成功后才重试 no-replace。部分提交后禁止改变已提交文件/目录的映射。预览缓存目录枚举，最终每个提交刷新父目录；扫描有上下文取消和每次最多40,000条目录项限制。并发发生在最后检查之后的精确名称冲突仍由原子 Link/Mkdir 拒绝。

## 验收边界

本分支仅实现传输层、协议与恢复基础。真实 App 接收清单、选择控件、剩余空间 API、最终目录展示、NoContent任务状态、稳定错误文案和原生包仍由后续接线验证。root/core、独立桌面编译测试、故障用例、旧帧固定结构和真实 loopback QUIC 的实际结果见 [M4 evidence](../evidence/DESKTOP-M4-TRANSFER.md)。不把 net.Pipe 当网络，也不把 loopback 当物理 LAN 或双 NAT；原有跨 NAT FAIL/NOT_RUN 不因本次工作改变。
