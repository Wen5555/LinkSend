# E1-02 撤销证据

组删除先写本机trust schema 2拒绝屏障、推进授权generation、清除pin/免确认并保留pending outbox，再取消目标任务并提交签名 `{request_id,target_incarnation,expected_revision}`。服务端事务重验actor incarnation、同组范围和目标revision；普通成员可删除，重复request幂等，跨组及已撤销actor迟到请求拒绝。成功后组内WSS重新认证，健康QUIC不由服务端信令关闭。

本机outbox在 `Devices` 同步时重试原request；旧服务快照、同一incarnation和旧revision不能解除屏障。只有新incarnation+更高revision建立新组grant，旧免确认/LAN grant不恢复；显式本机block永不自动解除。恢复任务和queue创建时间早于新generation的grant时拒绝。

自动测试覆盖普通成员撤销、跨组负例、actor竞态、幂等、旧快照、新关系不恢复 consent。未完成：准确包重启后离线outbox到真实主站的现场复核；没有将自动测试记作该项网络PASS。
