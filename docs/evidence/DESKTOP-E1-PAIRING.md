# E1-01 配对与成员代际证据

候选源码：`codex/desktop-experience-upgrade`，以本证据文件所在的阶段提交为准。

实现：控制库可从schema 1/2/3迁移到schema 4；旧未消费邀请码在首次代际迁移时失效。设备组保存单调revision，成员保存随机128-bit incarnation；邀请码绑定inviter incarnation与创建revision。同身份重试和同组fresh code幂等且不提权；revoked身份仅能用新有效码取得新incarnation。新增签名、限流、幂等空组初始化；active跨组必须显式提交当前group/incarnation/revision，旧组离开与目标组加入同事务完成。`membership_version=2` 为HTTP/WSS必需能力，旧客户端得到 `VERSION_INCOMPATIBLE`。

专项检查：`go test ./internal/identity ./internal/store ./internal/server ./internal/discovery ./internal/signaling ./internal/app -run "TestMembership|TestLANPair|TestNetworkChange|TestWindowsBroadcast|TestRefreshClears|TestDirectedAnnouncement|TestLocalBlock" -count=1`，全部PASS。覆盖同组fresh code、旧码重放、新码重配、旧端拒绝。公网生产尚未部署本候选。
