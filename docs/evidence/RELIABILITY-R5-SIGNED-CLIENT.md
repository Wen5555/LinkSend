# R5 签名HTTP客户端（2026-10-26）

基线为签名服务ff6a327及随后跨网同步eccb903。[ADR0017](../adr/0017-one-time-share-grant.md)的实验控制API已有真实服务与持久化验证，本增量补客户端，不提供临时P2P、UI或完整share_grants_v1能力。

## 行为与边界

- signaling.Client新增ShareControlInfo及类型化ShareControl。每次操作独立读取当前公开epoch，使用既有AuthBytes/Ed25519签完整URI、身份、epoch:新nonce、时间和有界JSON；不缓存旧epoch，不自动重试有副作用请求。响应丢失只能核对或明确重用原稳定ID重试，不能生成新授权。
- 继续使用New已有的HTTPS校验、HTTPClient和禁止重定向策略；没有另建网络owner或持久数据库。普通旧端/无实验能力明确拒绝，不把file_transfer_ready=false当成可传文件。
- 创建只发代码摘要，领取才发明文码；请求不接受路径/正文。有限ShareStatus严格核对状态结构与当前actor、ID、清单、claim、task、generation、计划；合法创建/领取/批准/绑定的后续幂等状态保留，不能把已完成任务重新开放。返回status不是未来数据面授权，文件动作仍须原scope新核验。
- 外层health拒绝重复status/capabilities/share_control，兼容普通未知health字段；受限DTO拒绝重复/未知字段、未知状态、超限或多JSON。服务错误仅保留白名单稳定码及固定摘要，丢弃任意远端detail和未知错误码，不回显分享码、路径、私有record或HTTP响应正文。
- 客户端不导入server包，不初始化/加入永久组，不写信任或自动接收/剪贴板权限。未入组actor仍不能使用普通设备枚举或成员WSS。

## 实际验证

独立审查发现原重复字段检查大小写敏感，而Go结构体解码会做case folding。新增health根/capabilities/status及control中的ASCII大小写、Unicode long-s回归，旧实现13个子用例实际FAIL；修复用strings.EqualFold拒绝语义重复，并分别检查capabilities。单个普通未知health/capabilities字段仍兼容，原失败日志casefold-before*.log保留。

Windows Go1.27.1，真实TLS httptest服务、SQLite与Ed25519身份；不是物理网络或UI。

- 最终10个顶层、47个子用例，共57项通过，2.699s、exit0；全部新增用例race×3通过5.658s、exit0。最终signaling全包workspace/off test分别2.469/2.512s、两次vet均exit0，含大小写及Unicode语义重复字段修复，不沿用旧43项日志作为最终结果。
- 未入组发送/接收身份经真实实验服务完成create→claim→approve→bind→authorize→complete，核对合法幂等重试、deny/revoke、成员隔离；服务重启后旧epoch证明拒绝，新请求签新epoch且不延长原任务期限。
- 受控TLS覆盖响应actor/ID/清单/task/gen/plan篡改、坏状态/长度/未知及重复字段、未声明能力、重定向不外送凭证、请求取消与安全错误。取消保留context.Canceled及CANCELLED稳定码；没有把取消记作连接超时。
- 首轮HTTP/1取消测试夹具不读POST正文而等待服务context，导致httptest.Close收尾超时；修正为读取有限正文并有独立release收尾后通过。保留首轮日志，不据此宣称产品取消失败或掩掉测试失败。
- 两Go模块workspace/off分别test -count=1 ./...、vet ./...、build ./...共12命令全部exit0（core118813/111465ms、desktop9088/8267ms），该矩阵早于最后语义重复字段修复；最终解析补丁由上述signaling双模式全包test/vet与完整新增race覆盖。准确提交完整CI仍须核对，不能用早期矩阵掩盖已知app磁盘压力恢复竞态；该问题另行修复。没有前端源变化。

本地日志D:/apps/Osend/.artifacts/share-client-20261026：casefold-after-summary.json、casefold-after-race.log、casefold-*-{test,vet}.log；双模块日志D:/apps/Osend/.artifacts/crossnet-20261026/reliability-network-share-client-*.log逐份保留退出码/耗时。生产/测试文件SHA256分别202629fa3d885398c27675aa705340fe46af3bf32ff765e3aa278f6b93117876和7235df5ff8ed1992d3e4f2f0f596ecdd9fa29351a7f8641d2a80d7e15c2f85c4。

下一步先冻结grant专属协调与加密manifest预览：approved只允许独立有界元数据预览，bound后才进入现有V1文件引擎及目标storage；不得绕过普通trust检查或把临时actor塞入永久成员。任务/计划冻结、撤销取消、两端UI、准确包Windows↔Mac和真实跨网临时分享仍必做，R5未完成。没有生产替换、宿主网络变化或新的常驻任务。
