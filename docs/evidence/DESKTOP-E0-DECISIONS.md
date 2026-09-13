# E0-05 设计审查回执

2026-09-13。状态：待总控审查，**尚未冻结、尚未实现**。
提案为 [ADR0007](../adr/0007-desktop-membership-consent-and-clipboard.md)，覆盖E0-review-notes-v1：
成员incarnation/组revision、合法新码重配、组内同事务撤销/互撤竞态、同组幂等不提权、inviter当前组/代际、独立LAN同意、目录事实、剪贴板权限/冲突/时效及有界会话调度。

需要总控判定的具体选择：

1. membership_v2为新组/迁移组的硬授权门槛，旧端明确要求升级；文件帧可保留V1，但旧pin不能绕过会话代际认证。
2. 从组删除的tombstone可由可验证新入组incarnation解除；“仅本机屏蔽”仍须本机显式解除，二者不可混用。
3. LAN跨设备提交允许provisional/等待互认状态，不宣称断网下严格跨设备原子性；只有durable mutual grant可发送正文。
4. 跨组切换为显式事务，不自动合并；LAN的跨网络加入凭证短时、绑定身份，过期后重验。
5. 剪贴板使用本机generation防迟到覆盖，Lamport+origin排序多远端并发，建议10秒TTL与每对端1文件+1事件初始预算，需专项测试冻结。

本批只更新设计文档和原型/验证工具，未改变生产wire/schema/权限；新增兼容/安全用例NOT RUN，属于E1/E4实现门槛。
SPEC/PROTOCOL/SECURITY/ARCHITECTURE仅加本轮提案链接，旧M5事实保留。PROGRESS追加实际E0中间状态。
