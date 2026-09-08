# Protocol

控制面使用有界 JSON `Envelope`，签名输入是明确的长度前缀二进制编码，不直接签任意 JSON map。字段包括协议版本、消息 ID、会话、发送方、接收方、generation、有效时间、payload 和 Ed25519 签名。

消息类型为 `connect_request`、`connect_response`、`candidate`、`end_of_candidates` 和 `status`。服务端只转发已认证设备组内、会话归属和 generation 正确的消息。未知关键类型、版本错误、重放、过期和超限消息明确拒绝。

文件控制帧使用长度前缀和独立的 QUIC stream，控制 metadata 上限为 8 MiB，文件内容不 Base64 化。传输顺序为 offer、用户 accept、块请求/数据、ack、finish、completed、confirmed。4 MiB 是默认分块，演示使用 64 KiB 以缩短测试时间。

控制帧必须满足对应操作的语义：offer 的 manifest digest 必须匹配；accept 的已验证字节数必须在 `[0,total]`；ack 必须对应刚发送的块、单调且不超过初始恢复字节加实际发送字节；finish/completed/confirmed 必须绑定 manifest digest。空帧、未知关键操作、负索引和不可能的字段组合均会被拒绝。对端 error detail 限制为 4 KiB、有合法 UTF-8 且移除控制字符；未知 JSON 字段暂不作为 V1 兼容性错误。

恢复身份由 `TransferID + manifest digest + chunk policy + peer identity` 共同绑定。接收端重新哈希 staging 块后才继续，完成后通过 `os.Root` 约束路径并以不可覆盖方式提交。
