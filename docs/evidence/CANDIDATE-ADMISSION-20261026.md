# IPv4候选准入修复（2026-10-26）

## 故障与范围

核对旧25f10a2的core CI39180492297时，发现多个真实Pion应用回归报INVALID_MESSAGE（broadcast/reserved candidate address）及后续CHECK_TIMEOUT。原始日志在D:/apps/Osend/.artifacts/reliability-20261019/r5-core-25f10a2-ci-failure.log；其中没有实际候选IP或网段，不能据此断言该runner使用某个地址，也不能直接归因用户两台Windows的失败。

独立复现了协议准入缺陷：ValidateCandidate把所有IPv4末字节255视为广播，但ICE候选没有网段信息；10.0.0.255/23、172.16.0.255/16及部分公网映射是合法单播。旧实现对12个host/srflx/prflx正例实际FAIL。IPv4映射IPv6还可绕过原Is4分支对0/8、240/4及有限广播的限制，负例同样实际FAIL。

生产调用点是信令服务Server.forward的候选准入。修复先拒绝zone，再用netip.Addr.Unmap统一IPv4判定，移除按末字节猜测广播；0/8、多播/保留高地址、有限广播、链路本地、未指定及未显式启用的loopback仍拒绝。Pion继续负责解析、STUN认证和连通性检查，文件协议字段、身份验证和relay=false保持。单独更新Windows候选不会替换生产服务的准入逻辑。

## 验证

- 原代码负例/正例失败日志candidate-admission-before.log保留；修后protocol全包普通3轮0.671s、race3轮1.499s，均exit0。
- 真实SQLite、HTTP入组及双方认证WSS先建立原session，再调用生产forward。合法末尾255的host/srflx/prflx原样转发，原签名、SessionID与generation保持；错签名、错sender、其他组、未知session、旧generation和有限广播均拒绝，且拒绝后转发计数不增加。
- forward定向普通3轮2.216s、race3轮3.817s，均exit0。日志candidate-forward-{after,race}.log在同一reliability证据根。这是实际服务准入回归，不是向这些地址建立了物理网络连接。
- 最小正式版分支另行移植三文件，定向TestCandidate在该基线通过；两模块workspace/off完整test -count=1 ./...、vet ./...、build ./...共12命令全部exit0。core test107506/102556ms，desktop13436/7537ms；逐命令日志server-admission-hotfix-{core,desktop}-{workspace,off}-{test,vet,build}.log在D:/apps/Osend/.artifacts/crossnet-20261026，包含退出码和实际耗时。无前端源码变化。

没有修改生产服务、服务器防火墙、路由、代理或DNS。edf7cac Windows包和双向实网验证保持原准确来源；本次服务端候选修复不倒记到该包，也不抵扣用户暂时无法试用的Windows双端现场证据。服务部署不在本轮默认授权内。
