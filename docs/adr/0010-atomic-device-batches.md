# ADR0010：持久化多设备分发批次

日期：2026-10-18。状态：源码候选与本地自动测试通过；原生/物理与阶段CI分别记录于[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)。

同一Go持久草稿保存多个明确目标，批次命令提交精确草稿revision、request ID和等待离线设备的选择。Go核对全部目标当前已配对身份、授权代次、来源和规则，准备一次初始内容身份后，在单个SQLite事务内写入批次及全部独立子队列；任一预检/插入/提交失败都不能留下部分批次。不由JavaScript循环Enqueue。批次与子请求各自稳定、重复目标拒绝，同request不同来源/规则/目标/等待策略拒绝；完成请求在草稿清空后仍返回原收据。

首版继续原有进程级顺序派发，一个活动文件任务。每个目标保存授权代次、源摘要、规则和独立queue/task状态；每次派发重新核对源，源变化要求新确认，不声明冻结快照。一个目标离线、撤权、失败或取消不撤销其他目标已提交的文件。继续/重试复用该子任务，只重试用户选择的未完成者；主草稿后来修改不改变已有批次。

批次取消先持久化取消意图并停止尚未开始的子项，再通过现有任务控制取消唯一活动子项；已完成目标保持。取消与恢复/派发需使用同一Go owner门闩，重启后不自动派发旧批次，明确确认仍受原授权代次和源规则限制。批次状态由持久子项和取消意图汇总，不以按钮状态冒充全部完成。

任务库10→11，沿用可读备份/校验/事务迁移。`send_drafts.peer_ids`保存最多16个有序identity；非空列表第一项保持旧peer_id，旧行默认[]，损坏null/不一致明确拒绝。`send_batches`保存ID/request/signature、草稿来源、初始摘要、revision、取消与时间；send_queue增加batch_id/batch_position及批次内peer/position唯一索引。旧schema10真实库拒绝11，DB hash不变；不能降低版本或用旧备份覆盖后续业务写入。文件wire、真实Pion/QUIC、身份pin、hash、安全落盘和relay=false保持。

## 已实现命令与屏障

1. `EnqueueBatch({request_id,draft_id,draft_revision,wait_for_peer,expires_at})`只读精确持久草稿的路径/规则/2–16目标；普通Enqueue拒绝消费多目标草稿引用。提交内核对revision、规则、目标与路径；parent/全部子项单事务，第三子项trigger失败证明全回滚。队列未完成上限1000，默认7天/最长30天期限。
2. 相同请求引用在草稿清空后仍返回原收据；改变参数拒绝。子request以域分隔的request ID/peer派生。每目标保存原授权generation、来源摘要与N1规则，派发前重验，不声明内容冻结。
3. `GetBatch(id)`和Workspace按持久子项汇总，最多附带50批、活动优先；一个活动文件任务。UI有搜索/10条分页/16目标上限、可移除撤权目标，只调用一次Go批次命令；成功后才清草稿。
4. `CancelBatch(id,revision)`按queue owner→operation owner先事务取消，再释放operation owner并取消活动任务。TaskSnapshot与私有recovery保存batch ID，关闭队列关联失败窗口。批次任务必须从队列确认，直接Resume/Retry返回BATCH_TARGET_REQUIRES_CONFIRMATION；取消后BATCH_CANCELLED，StartSend内部同样校验。
5. 启动/调度以durable completed回执优先修正子项，晚到完成发出状态更新；仍在停止时显示cancelling。取消崩溃窗口不会恢复发送，重启不自动派发，逐子项明确确认仍受原代次和规则限制。取消不删除源或接收文件。

自动测试覆盖三目标夹具、重复目标/request、事务故障回滚、混合在线/离线、撤权、单目标失败/独立重试、批次取消、重启、晚到完成、派发间源变化及N1规则保持；命令与边界见[证据](../evidence/DESKTOP-AE-DEVICE-BATCHES.md)。真实Windows/Mac两机另记，同机Pion/QUIC不等于三台物理机或双NAT，现有N1包source0499cb5也不等于N2包。
