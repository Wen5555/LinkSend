# R5 专属授权协调原型（2026-10-26）

基线4f562b3；先在[ADR0017](../adr/0017-one-time-share-grant.md)冻结scope、两阶段、签名、资源及关闭语义，再实现服务与客户端。只在原experimental_share_control=true下启用，health仍file_transfer_ready=false，不广告完整share_grants_v1。本增量不提供文件引擎、UI或正式部署。

## 范围

GET /v1/shares/{id}/ws使用独立linksend.share-signal.v1子协议。canonical query严格绑定claim/task/grant generation/phase/manifest/plan且不含代码/清单/路径；完整RequestURI由现有epoch:nonce/Ed25519请求签名保护。ShareSignalFrame保留原Envelope签名，并用现有AuthBytes再签原Envelope和完整scope/epoch/新nonce/时间。ICE generation与grant generation独立，不能转置到其他grant、任务、角色或阶段。

preview只允许approved的原双方、原task/清单及领取期限；Bind持久成功退役preview，transfer必须bound原plan及恢复期限且使用新ICE SessionID。固定发送方发起、接收方响应；消息只允许严格ICE凭据、候选、空end及白名单失败状态，关闭会话复用/剪贴板/普通授权代次。正文、文件清单与用户路径没有信令入口。

服务用独立shareHub，不把临时actor塞入s.clients/s.sessions或永久数据库成员表。最多32scope/64连接/每actor8，每方向32候选、单negotiation、队列64、60frame/s；frame32KiB、payload16KiB，严格递归JSON拒绝重复及Unicode大小写别名、未知字段与不允许的结构。鉴权/写5秒、空闲读65秒；scope自身真实绝对期限timer主动退役。存储迁移仍由原实验开关负责，没有新DB owner/schema。

分享迁移、注册、入队、writer出队在同一gate检查当前记录和owner指针；退役丢未发队列，旧cleanup不能删新连接，网络写不持全局s.mu。Server.Close禁止新注册、取消并join分享资源后关闭唯一数据库；关闭或ctx取消返回SHARE_UNAVAILABLE，不能伪装持久grant终态。

客户端ConnectShare重新核对原scope/双方key和实际期限，再签专属WSS；ShareSession单reader、有界64队列、可取消写门，实现原directSignalSession六方法及WaitPeer。presence只证明对应socket在场，仍需Pion/固定公钥QUIC。Read交付原已签Envelope，不改payload、不授予永久权限。1008且精确白名单原因才分类：Bind为SHARE_CONFLICT，完成/撤销为SHARE_TERMINAL，到期为SHARE_EXPIRED；Server.Close/普通EOF仍SIGNALING_UNREACHABLE，私有关闭文本不回显或按前缀猜撤权。

## 实际验证

- 真实SQLite/签名/WSS：未入组双方可协调、永久表零增加；第三身份、错内外签名/epoch/key、跨scope/task/phase、旧grant/ICE代次、正文/剪贴板字段、重放及候选/队列/actor/连接预算均拒绝。32scope与真实actor上限有实际边界回归。
- 屏障固定鉴权后注册前撤销、出队后写前撤销、旧cleanup晚于替代连接三处竞争，证明旧回调不复活scope或删除新peer。Bind退役preview、绝对2秒期限timer空闲关闭、服务重启旧证明/原期限不延长、Close与注册/普通连接隔离分别验证。
- 服务/shared普通3轮sharegrant1.051s/server11.621s，race3轮1.991s/23.299s，均exit0，相关vet通过。随后独立审查发现旧ICE replay key含phase，真实preview→Bind→transfer使用新outer证明重用旧SessionID时原实现错误接受，保留r5-share-signal-crossphase-before.log。现key按epoch+grantID+SessionID覆盖两阶段，仍为原2分钟有界ReplayWindow；修后全部TestShareSignal普通3轮11.925s、race3轮24.538s通过，不加数据库或永久ID列表。首次limits夹具的map类型编译笔误保留，不记成产品或网络失败。
- 客户端最终10顶层+36子用例共46项3.079s，race3轮6.312s、signaling独立全包test3.150s及vet均exit0。受控TLS覆盖取消、重定向、错scope/签名/epoch/recipient、私密错误、超限、队列压力及并发Close/Read/WaitPeer。
- 准确服务的真实TLS客户端闭环：approve后preview、Bind关闭双方preview、全新transfer scope/ICE ID、Complete关闭双方；普通成员接口仍拒绝。真实Revoke/Complete与普通Server.Close分类分别核对，不从普通断线推断撤权。
- root另跑sharegrant/server/signaling三个完整包的GOWORK=off test -count=1及vet，均exit0（1.077/5.542/2.968s）；workspace完整包也均exit0（1.723/5.818/4.028s），日志r5-signal-integration-workspace.log。最后跨phase修复由上述完整受影响TestShareSignal普通/race覆盖；两模块完整候选矩阵和准确CI随后核验，不预记通过。

日志：D:/apps/Osend/.artifacts/reliability-20261019/r5-share-signal-final-{test,race}.log、r5-signal-integration-{packages,workspace}.log；客户端.artifacts/share-signal-client-draft/terminal-aware-{targeted-summary.json,race.log}及signaling-off-{test,vet}.log。

本次仅证明专属控制协调；Pion/QUIC元数据预览、原文件引擎、本地签名任务owner/恢复、两端UI、准确Windows↔Mac和真实跨网临时分享仍分别验收。不能以真实WSS代替文件传输或物理网络，不能标R5或整个Goal完成。
