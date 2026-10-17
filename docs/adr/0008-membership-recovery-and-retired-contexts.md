# ADR 0008：显式成员恢复与已退休上下文

日期：2026-10-18。状态：已实现的候选；自动测试及准确包/原生/网络边界见[阶段证据](../evidence/DESKTOP-AE-MEMBERSHIP-RECOVERY.md)，当前工作状态仅由[TODO](../DESKTOP-EXPERIENCE-TODO.md)维护。

## 问题与边界

同一信令 URL 的控制库重建后，旧 identity 的 Devices 返回401。401也可能表示普通成员被撤销，不能据此自动初始化或清空 profile。普通 SwitchMembership 需要当前服务成员证明，不能处理空库。用户需要显式选择新建组或使用新邀请码加入，同时保留 identity、文件、历史、偏好、明确本地拒绝与有效独立LAN。

文件 V1、membership_v2、控制库schema4、桌面任务库schema7保持。恢复调用只携带控制元数据；文件和剪贴板正文仍由Go经已认证P2P传输，relay=false。

## 本地策略与状态

`trust.json` 最大支持schema3；没有上下文替换/恢复的普通schema2写入保持schema2。显式恢复及已验证成员上下文替换升级至schema3，旧客户端明确拒绝，不能忽略新字段继续使用旧权限。

schema3增加一个恢复intent、至多128个退休group/local-incarnation上下文及一条最近成功收据。intent只保存随机ID、模式、精确服务地址的SHA256、阶段和创建时间；邀请码不落盘。收据绑定请求ID、endpoint与实际新成员上下文，用于响应丢失/重复点击后的幂等核对。

| 状态 | 操作与持久结果 | 服务端行为 | 取消/重启 |
|---|---|---|---|
| idle / completed | 用户确认后 Prepare；先备份并持久旧任务代次/时间屏障，免确认关闭 | 无注册请求 | 不自动开始恢复 |
| prepared | Continue先正常Devices；已有新结果可原子提交，旧上下文不算恢复成功 | 仅明确401且原intent匹配时，按模式进入正常Initialize/Join | Cancel移除intent；不回退已提升的授权代次 |
| started | 在实际注册前持久该阶段；结果核对后原子提交 | 普通Initialize或新邀请码Join，不上传旧组/owner缓存 | 失败/中断保留；Cancel只转paused |
| paused | 显式Continue重新读取实际成员 | 先Devices；已完成则不重放初始化/已消费邀请码，未完成的Join需新码 | 重启保持paused，不猜测服务端没写入 |
| completed | intent退休，记录成功收据；旧context永久拒绝 | 重复相同请求只核对实际同一成员 | 旧请求不能跨endpoint或后续恢复重新初始化 |

Continue总时限15秒；网络、TLS、403或普通401不会自行创建intent。旧profile的普通Initialize不能在未证明现有成员时静默重新登记。正常被移除后的新码重配继续可用；新的已验证上下文同样退休旧上下文。

## 授权与LAN

准备恢复先提升所有旧组相关peer的聚合generation并更新时间，关闭免确认。旧group+lan立即分离为有效LAN关系并保留地址、历史、request ID；新显式LAN同意仍可建立独立授权。旧group-only在pending期间不能授权任务。旧任务和队列保持原generation，不能借新组或保留LAN关系自动续发；重启也不重绑。

已验证新快照必须含唯一且正确的本机/对端ID、公钥与incarnation，组和revision一致。相同旧context不算恢复完成，已退休context即使携带更高revision也拒绝。退休容量满时保留现有权限和策略，拒绝增加退休项；当前上下文刷新仍可工作。

原子完成恢复时移除旧组授权、保留独立LAN、本地LocalBlock和用户资料，旧非本地撤销记录标记context_retired并停止outbox。新组不继承免确认或剪贴板权限。普通重配保留原删除意图供用户明确继续；不同组的revision不直接比较，同一原目标incarnation仍受删除屏障约束。

旧策略可能只有group而没有actor-incarnation。这种信息只保留为schema3的非待发送本地记录，不伪造actor。待发送记录仍须完整绑定或走已有的明确legacy重新授权；明确LocalBlock始终优先。

## 操作所有权

专属membership mutex串行成员变更、outbox与完整的服务快照读取/应用，不持operationMu做网络IO。这样，初始legacy缓存没有旧incarnation时，在途旧Devices也会先于Prepare应用并被纳入退休集合。已有LAN/session命中本地pin；设备页在其他成员操作期间使用本地目录，不让恢复阻塞独立LAN。

LAN凭据签发/自动补组在恢复期间跳过，LAN同意和安全落盘不依赖补组成功；签发的可选服务调用限500ms。只有成员快照实际同步成功才报告joined。

每次DELETE前先核对当前持久撤销记录和正常Devices返回的live actor/原目标，重复、缺本机、混组、目标换代或旧actor不发删除。原actor可使用同一request ID最多重试一次revision；新actor或legacy必须有新的明确删除动作和CAS生成的新request ID。缺少原目标时只允许原actor下原请求的幂等确认，不自动创造新目标或新意图。

## 备份与兼容

升级前保留精确原字节到profile的 `trust.json.pre-schema-3`，沿用/补齐 `trust.json.pre-schema-1` 失败关闭标记。备份固定、有界、不可覆盖，拒绝目录/异常备份；写入采用已有同步和原子替换。新/legacy profile首次恢复也建立标记，丢失/损坏策略不能自动回到空策略或旧.previous。

不自动用pre-schema3覆盖当前策略以运行旧客户端，那会丢失退休和拒绝屏障。旧端拒绝schema3已用18a88f2真实库编译的只读兼容探针验证；它是库兼容测试，不是旧原生应用验收。
