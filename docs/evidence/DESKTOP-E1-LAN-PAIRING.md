# E1-03 LAN独立同意证据

新增签名 `lan_pair` request/accept|reject/commit/ready/confirm/done 与 query 恢复transcript，绑定双方DeviceID、request ID、nonce、授权generation和最多60秒期限；承载通道是最近签名公告固定公钥后的双向TLS 1.3。发现本身不授予pin；接收端通过 `PendingLANPairings`/`RespondLANPair` 明确同意。双方先保存有界provisional，接收端只有收到confirm后才提交可用pin；ack丢失可按request/nonce查询恢复。并发双发以request ID确定单一事务。

`TestLANPairingRequiresRemoteConsentAndCommitsBothPins` 在两个独立profile、两个真实TCP/TLS控制listener上通过；`TestLANPairingRejectLeavesNoAuthorization` 证明拒绝后双方零pin。控制帧有32KiB上限，pending按request去重且到期移除。

已入组同意方可向服务端提交双方签名transcript，取得仅目标DeviceID可消费的60秒凭证；无成员关系目标自动加入，同组/跨组仍遵循幂等与显式切换。未完成：准确Windows/Mac包上的系统确认入口和物理LAN双向同意；服务不可用时仍准确标为“LAN已配对、跨网络未加入”。
