# E1-03 LAN独立同意证据

新增签名 `lan_pair` request/accept|reject/commit/ack 控制transcript，绑定双方DeviceID、request ID、nonce、授权generation和最多60秒期限；承载通道是最近签名公告固定公钥后的双向TLS 1.3。发现本身不授予pin；接收端通过 `PendingLANPairings`/`RespondLANPair` 明确同意，成功双方才写独立LAN grant，服务状态保持 `not_joined`。

`TestLANPairingRequiresRemoteConsentAndCommitsBothPins` 在两个独立profile、两个真实TCP/TLS控制listener上通过；`TestLANPairingRejectLeavesNoAuthorization` 证明拒绝后双方零pin。控制帧有32KiB上限，pending按request去重且到期移除。

未完成：准确Windows/Mac包上的系统确认入口和物理LAN双向同意；服务可用时把LAN确认兑换为绑定peer的短期入组凭证尚未实现，因此当前结果准确标为“LAN已配对、跨网络未加入”。
