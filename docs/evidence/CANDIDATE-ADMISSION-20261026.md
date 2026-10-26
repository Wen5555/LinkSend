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

## 准确提交与服务候选

最小正式版分支已推送99a6c0ee09c1e62074d39abbdf8c5081978bd629，仍在[PR #12](https://github.com/Wen5555/LinkSend/pull/12)供审阅。push core40217679160、desktop40217679157、packages40217679062（三平台），PR core40217682895、desktop40217682925均completed/success。精确状态/完整jobs保存于crossnet证据根ci-99a6c0e-verified.json，不将可靠性开发分支的独立失败抹成通过。

干净99a6c0e源码以GOWORK=off、GOOS=linux/GOARCH=amd64/CGO_ENABLED=0、go build -trimpath ./cmd/rendezvous构建Linux候选；go version -m为原revision、modified=false。D:/apps/Osend/.artifacts/crossnet-20261026/server-candidate-99a6c0e/rendezvous-linux-amd64为17,943,022B，SHA256为8a869f163341ae34c5ecb3c5dc7d9d668f54fed32fcc493de6a8f9a9a78d38a4；同目录BUILD-INFO.json、GO-BUILD-INFO.txt和SHA256SUMS保存来源。

经SSH上传到JP专用测试根后先核对摘要，使用新建隔离库与127.0.0.1临时端口运行该准确二进制。原生-version为0.6.2，HTTP health200/status=ok/protocol1/quic/relay=false，SIGTERM正常exit0；关闭后SQLite只读核对schema4、devices=0。作业/tmp/codex-ssh/job-ls-server-99a6c0e-smoke finished/exit0，同目录native-verification.json为回执。没有连接真实设备、触碰生产配置/数据库、注册systemd或对外监听。该原生启动检查不替代生产部署或真实末尾255拓扑连接。
