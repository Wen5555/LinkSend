# E1-02 撤销证据

## 2026-09-22 双重关系的离线撤销

新增真实 HTTP 服务关闭 listener、恢复原端点和隔离 profile 重启的回归，复现 `group+lan` 被离线目录误标为 `lan_paired`、删除绕过 membership-v2 outbox：原结果 `LocalBlock=true`，目标 incarnation/request ID 为空且 `PendingSync=false`。修复仅在 `Service.Devices` 和 `Service.Revoke` 两处把 `group+lan` 纳入设备组关系。原协议、身份校验和 incarnation 作用范围不变。

`GOWORK=off go test ./internal/app -run '^TestOfflineMembershipRevokeSurvivesRestartAndSync$' -count=1 -v` 在修复前 exit 1、修复后 exit 0。三场景覆盖普通组/组+LAN 的持久 pending、请求 ID 跨重试/重启不变、旧快照不复活、恢复服务精确撤销、再次重启、保留已完成记录与收到的文件；另一成员先撤销再显式重新配对时，旧 outbox 不得撤销新 incarnation，也不恢复免确认。

同组回归和既有授权负例的定向 race 通过；`TestLANOnlyRevokeDoesNotDeleteUnrelatedMembership` 的 race 也通过，两个独立设备组的 LAN-only 删除实际 HTTP DELETE 次数为 0，其他组成员保留。真实命令/退出码/RED/GREEN 日志在 `.artifacts/astra-resume/u2/`，入口 `SUMMARY.md`。这些是 core HTTP/持久化测试，不替代准确 Windows 包、香港实际 outbox 同步或物理 LAN 验收。

组删除先写本机trust schema 2拒绝屏障、推进授权generation、清除pin/免确认并保留pending outbox，再取消目标任务并提交签名 `{request_id,target_incarnation,expected_revision}`。服务端事务重验actor incarnation、同组范围和目标revision；普通成员可删除，重复request幂等，跨组及已撤销actor迟到请求拒绝。成功后组内WSS重新认证，健康QUIC不由服务端信令关闭。

本机outbox在 `Devices` 同步时重试原request；完整成员快照会将此前同组、现在消失的peer原子转为拒绝并取消旧grant和该peer的活动task，旧服务快照、同一incarnation和旧revision不能解除屏障。只有新incarnation+更高revision建立新组grant，旧免确认/LAN grant不恢复；schema1显式本机block迁移后仍永不自动解除。持久authorization epoch防止解除本机屏蔽后generation回退；任务、恢复记录、queue、PeerSession和迟到确认绑定实际授权generation，实际开流前再次精确比较，重新配对后旧generation拒绝。

自动测试覆盖普通成员撤销、跨组负例、actor竞态、幂等、旧快照、三成员快照取消活动task、旧queue generation不绑定新关系。未完成：准确包重启后离线outbox到真实主站的现场复核；没有将自动测试记作该项网络PASS。
