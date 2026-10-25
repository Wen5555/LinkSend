# R5 签名控制与领取限制（2026-10-19）

基线25f10a29ec7c530e3661ce76ad22e1adbec60642。[ADR0017](../adr/0017-one-time-share-grant.md)补充现有签名函数的运行代次绑定及确定限频参数。此增量只开放显式实验控制面，不发送文件、不开放临时成员WS、不改变V1文件身份校验；R5两端完整产品仍未完成。

## 实现

- 普通配置不激活ShareGrant迁移；实验配置experimental_share_control=true借同一Control连接启用4→5，并生成公开128bit随机epoch。health仅增加实验share_control.version=1/epoch/file_transfer_ready=false，protocol.Supported不广告share_grants_v1或文件分享支持，relay=false。关闭实验开关时路由明确426/SHARE_UNSUPPORTED。
- POST /v1/shares/{create,claim,status,approve,deny,bind,authorize,complete,revoke}使用既有AuthBytes/Ed25519：Header提供公钥、由该公钥导出的DeviceID、nonce、时间、signature及epoch。签名绑定方法、完整RequestURI、身份、epoch:nonce、时间、完整JSON；1分钟旧请求/15秒未来容差，nonce防重放。服务重启随机新epoch，不能只改Header重放旧签名；新的原身份证明可核对持久原任务。
- 领取IP突发20、已签名身份突发10，均每6秒补1；只有真实RemoteAddr参与IP限制，不信任任意X-Forwarded-For。bucket总数2048，空闲5分钟回收；复用既有有界ReplayWindow。拒绝领取统一返回安全原因，不返回其他领取人、码、清单或组。
- JSON体复用16KiB上限，严格未知字段/尾随内容及独立字段上限。返回重新构造的限定ShareStatus，不序列化私有record、code_digest、组/成员、路径或正文。发送者批准额外匹配其明确确认的接收公钥和原清单；临时actor不能调用普通设备枚举/邀请接口。
- 现有server生命周期每分钟触发最多64条授权过期/终态回收，关闭仍加入原owner后关闭唯一数据库。普通信令、HTTP成员认证、WS及文件路径保持。

## 验证

Windows Go1.27.1，fresh测试SQLite/测试身份，真实Ed25519签名与Server Handler；不是物理网络或UI。

- 真实完整控制链创建→领取→发送方批准→接收方绑定→原任务核验→完成，完成重试只返回原终态；完成后不能再authorize或由另一身份使用旧码。错误公钥、清单替换、task替换、跨grant查询及未批准身份被拒绝。
- 错误签名/公钥、正文/路径/epoch篡改、具有有效签名但过期/未来的证明和同nonce重放被拒绝；旧请求拒绝后回读原offer仍revision1。真实Server.Close/New使用同一库，新epoch拒绝旧证明及仅替换epoch Header的旧签名；新签名只恢复原bound且期限/revision保持，撤销后拒绝原任务。
- 两个实际独立成员组不变；未入组身份可提出临时领取但Control.Device仍无该身份，具有其自身有效签名也不能枚举成员/生成邀请，不能查看另一份grant。错误响应不含凭据、源名或其他actor。
- 同身份第11次猜码429，同IP轮换身份并伪造X-Forwarded-For第21次429；未命中猜码统一403。超限字段/正文、未知file_path拒绝且不回显伪私密路径；Status不含码/验证器或私有record字段。
- 首轮health测试夹具使用httptest默认非loopback RemoteAddr，HTTPS边界正确403，但夹具误报能力不符；修正显式loopback并先核对HTTP200后通过。保留r5-share-http-first-test.log，不当作产品宣称开放完整能力。
- 最终普通`go test ./internal/server ./internal/store ./internal/sharegrant -count=3`退出0（5.812s/2.710s/1.353s）；定向race3轮退出0（8.317s/5.124s/1.826s），见r5-share-http-final-{test,race}.log。

## 2026-10-26 提交前复核

- 本增量源码最后修改于2026-10-19 08:29之前；随后08:33–08:36完成两Go模块workspace/off的test -count=1 ./...、vet ./...、build ./...共12命令，日志r5-share-http-{core,desktop}-{workspace,off}-{test,vet,build}.log保留。两次core test的app包99.514s/104.395s，两次desktop包11.038s/8.544s；没有用热修复分支的检查替代本批。
- 2026-10-26补跑`GOWORK=off go test -race ./internal/server ./internal/store ./internal/sharegrant -count=1`，退出0，包耗时4.299s/3.058s/1.351s、命令总12195ms；日志r5-share-http-20261026-review-race.log包含退出码与耗时。定向真实签名/SQLite测试保持通过，独立只读复核未发现阻止该实验增量提交的问题。
- 原工作树同时保留已另行交付的跨网STUN修改及用户未跟踪内容；本增量逐文件暂存，不混入这些修改。没有前端改动、临时文件通道、生产替换或宿主网络修改。

## 下一步与限制

客户端能力检查/请求实现、临时认证ICE/QUIC协调与作用域、文件引擎原任务绑定/撤销取消、两端UI、准确包Win↔Mac和跨网真实分享仍必做。实验HTTP authorize只是服务状态核验，不能单凭这次返回授权未来的文件操作；数据面必须在本地固定授权owner下处理原任务、代次、取消与公钥，不可绕过永久黑名单或把临时对端加入长期信任。

1caddbd的core39179675064、desktop39179675034、三平台包39179675025全部success。2026-10-26准确核对25f10a2：desktop39180492313及packages39180492292均success，core39180492297失败；多个app真实Pion回归报告INVALID_MESSAGE（broadcast/reserved candidate address）及CHECK_TIMEOUT，完整失败日志保存在同证据根r5-core-25f10a2-ci-failure.log。未把该旧失败抹成通过或据此归因ShareHTTP；本次准确提交CI推送后单独补记，旧包结果不转记此增量。
