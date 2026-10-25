# 跨网连接修复与验证（2026-10-26）

用户报告：最新版Windows→Windows，两端互联网，显示在线但连接无法验证。优先修复该路径；原可靠性Goal其余范围不因此记完成。

## 已确认故障与代码

- 本机已保存服务地址为正式HTTPS信令，STUN列表为空。系统DNS把stun.oooai.de解析到198.18.2.109；独立HTTPS查询所得真实地址109.66.88.204也没有UDP响应。原生产Pion仅得到host、无srflx，STUN请求3/响应0；直接真实IP亦无srflx。JP新STUN监听虽正常，UFW仍仅开TCP22/80/443，外部3478超时；没有修改其防火墙或DNS。
- 对照Cloudflare STUN真实IP，原生产Pion46ms取得srflx。修后域名探针165ms取得srflx，原ICE/QUIC socket不变。后来旧探针对Cloudflare域名也能取得srflx（56ms），因此不把“新域名成功”单独当作DoH因果证明；Fake-IP解析回退另有真实Pion/受控HTTPS/STUN注入回归。默认服务无回应与Fake-IP风险分别处理。
- 桌面补充默认备用STUN并统一实际配置/只读显示，保留显式自定义偏好；原实现自动值、旧单内置值及空环境列表三项测试实际FAIL，修后通过且没有写用户偏好。
- 通过真实ice.WithNet/stdnet仅适配两个内置公开STUN主机的有界安全HTTPS解析；构造不联网、host/LAN不等外部DNS，关闭取消解析。详细边界见[ADR0018](../adr/0018-cross-network-stun-resolution.md)。文件数据仍走认证QUIC，无TURN或HTTP/TCP文件替代。
- 修复两角色ICE失败时证据全0：现在保留实际STUN/信令计数、本地socket/网卡/地址族、时间线与耗时；没有候选对或TLS时准确保持未知/0。旧实现两角色定向回归均FAIL，修后普通/race通过。

## 自动验证

Windows PowerShell，Go1.27.1。初始可靠性工作树上的最终检查：

- connectivity全包普通3轮6.183s、TestSTUN race3轮5.808s；真实Pion+本地STUN证明Fake-IP→受控HTTPS→同socket srflx，host先交付、关闭取消；TLS、重定向、响应/Question/地址/家族、CNAME最短TTL、并发合并、全局限额与失败退避通过。
- ICE失败诊断组合普通12.065s，race3轮39.780s；仍为CHECK_TIMEOUT/ice_checking，TLS0/ALPN空、无已选中对端和文件字节，不把STUN回复冒充认证成功。
- 桌面默认配置定向测试通过。两模块workspace/off的test -count=1 ./...、vet ./...、build ./...完整12命令退出0；无前端源码或文件协议改动。
- 独立只读复核未发现阻止交付的问题。正式版最小热修复分支基于origin/main 20d875189bce9300c62239c19bd4a369612a12ac，仅移植本次修复；两模块workspace/off完整12命令全部exit0（core test101.921s/98.679s、desktop9.535s/7.490s），TestSTUN race1轮3.864s、双角色ICE失败证据race1轮8.246s通过。日志hotfix-*.log逐份包含实际exit_code与耗时；没有借用其他工作分支通过代替该基线检查。准确源码/包与实网复验单独补记。

本地原始日志根：D:/apps/Osend/.artifacts/crossnet-20261026；desktop-defaults-{before,after-final}.log、stun-resolver-{test,race}.log、ice-failure-evidence-{before,after,race}.log、{core,desktop}-{workspace,off}-{test,vet,build}.log。早期工具链引用路径错误与开发过程瞬间编译错误不记最终通过，最终以锁定工具链的上述日志为准。

## 首轮真实不同网络文件旅程

测试双方为本机Windows amd64和JP Linux amd64公共主机，双方使用专用profile与新建的隔离测试组，正式HTTPS/WSS协调，保留公钥固定；不连接或发送到用户其他设备。测试接受回调只对该自有夹具显式同意，不改变产品默认接收设置。未设置固定端口映射或改宿主网络。

Windows侧STUN观察到公网出口映射；JP为公共地址。现场既有路由/代理配置保持，应用没有文件中继。此拓扑是单边映射到公共主机，不是Windows双端或双NAT证据；ConnectionMethod按实际后端仍为direct_unknown，不强行改名internet_p2p。

| 方向 | 文件/目录与内容 | 建链 | 端到端结果 |
|---|---|---|---|
| Windows→JP Linux | 3文件、合计16,777,310B，含16MiB文件、中文路径和空目录；6个正式落盘的文件/目录条目与hash一致 | 3518.4526ms，srflx→host | 双方Completed，SHA256/目录一致，TLS1.3、linksend/1、relay=false |
| JP Linux→Windows | 同一格式的独立自有源，3文件与空目录；6个正式落盘的文件/目录条目与hash一致 | 3294.114064ms，host→srflx | 双方Completed，SHA256/目录一致，TLS1.3、linksend/1、relay=false |

16MiB发送流程耗时分别202876ms与95252ms，接收含等待分别208715ms与97035ms；不宣称吞吐改善或已达到性能目标。初次集合汇总使用PowerShell成员集合.Count产生错误false，随后通过显式数组计数与逐路径比较复核，实际6条/6条、mismatches=[]；两份原始汇总均保留。

首轮验证器来自可靠性工作树（HEAD25f10a2+开发中修复，非正式版最小分支构建）：Windows二进制SHA256 9c99ced05e25558d57d8512ae8107f7ec9cfb801577b72504f336e11071e5c0f，Linux c08fa05d2922220de0e393fd56afbb4f2dca09b72feb1fd10c0eee20d587e11e。远端两作业均finished/exit0，Windows接收进程已正常结束。源码后续安全门/失败诊断及准确发布包不能直接倒记首轮验证，最小分支准确构建旅程继续。

证据：win-to-jp-verification-corrected.json、jp-to-win-verification-01.json、wan-local-01/{source-hashes,send-result,receive-result}.json、wan-{receive,send}-jp-01.json。远端/tmp/linksend-crossnet-20261026-01；job-ls-crossnet-jp-{receive,send}-20261026-01。源、接收副本及恢复checkpoint保留，未按临时目录名递归删除。

## 准确热修复提交与第二轮实网复验

修复已提交并推送为`edf7cac14d7d17d1491260e918e98be8944f710b`，分支`codex/fix-cross-network-20261026`，供审阅的[PR #12](https://github.com/Wen5555/LinkSend/pull/12)。没有合并main或发布Release/tag。

在该干净工作树中运行`GOWORK=off go build -o <新输出> ./.artifacts/wanprobe`，分别构建Windows及`GOOS=linux GOARCH=amd64 CGO_ENABLED=0`验证器。`go list`确认connectivity来自本工作树，两个`go version -m`均为上述vcs.revision、vcs.modified=false；driver SHA256为cbdcb59dbb17f90f818e1c1d6527fd891c038079980c992fe7a5b8bfcca29fae。Windows验证器SHA256为7afd0ba4b15c89ba733646150b583313f024a99ddcb98702012f8b8c39dd9a0f，Linux为297bfca743ede699eb8aea8c83dcaf21a527469dcca0cc10558fbe286bef1c26。

复用首轮两个自有隔离身份与已核对的源hash，使用`-attempt 02 -reuse-source`保存独立结果/接收目录，不覆盖首轮。两方向均使用正常正式HTTPS/WSS、默认两STUN地址与原固定公钥认证，未改宿主网络或生产服务。

| 方向 | 完整性与终态 | 建链 | 发送/接收流程耗时 |
|---|---|---|---|
| Windows→JP Linux | 3文件16,777,310B，含16MiB、中文路径、空目录；6/6正式落盘条目、逐路径SHA256及目录一致，双方Completed | 3375.9946ms，srflx→host | 183517/189375ms |
| JP Linux→Windows | 同格式独立源；6/6条目、SHA256/目录、清单digest与task一致，双方Completed | 3266.22693ms，host→srflx | 43927/44533ms |

两端均TLS1.3（772）、ALPN linksend/1、relay=false，ConnectionMethod仍为direct_unknown。吞吐仍偏低且方向不同，不从此样本推断性能改善。第二轮结果绑定准确核心源码；验证器不是Wails桌面包，不能将此结果当作Windows双端候选GUI验收。两远端作业finished/exit0，Windows发送与接收进程正常退出，随后专用Windows profile执行`-mode revoke`退出0并输出ISOLATED_TEST_PEER_REVOKED。自有源/接收副本和checkpoint作为证据保留；未删除用户文件或改日常profile。

两端在完成后的关闭阶段仍各有Pion日志`Failed to read UDP packet: use of closed network connection`；实际双边Completed/hash/退出0作为传输结论，保留该日志，不抹成全程零错误日志。

原始证据：hotfix-wanprobe-{windows,linux}-build.txt、hotfix-wanprobe-hashes.json、win-to-jp-verification-hotfix-02.json、jp-to-win-verification-hotfix-02.json、wan-local-01/{send,receive}-result-02.json、wan-{receive,send}-jp-hotfix-02.json及wan-revoke-hotfix-02.log。远端job-ls-hotfix-jp-{receive,send}-20261026-02。

## Windows可交付候选

[程序包构建](https://github.com/Wen5555/LinkSend/actions/runs/40214493558)的Windows job成功，artifact 12607365954；原始Actions ZIP的SHA256为133ad1aaa04e3215cadebd65dd6a9474fc64ae8c59650f4eccd57bc21932cd19，与GitHub API digest一致。内外BUILD-INFO一致，COMMITTED/clean/准确edf7cac，内部SHA256SUMS与PE版本0.6.2均通过。

候选根：D:/apps/Osend/.artifacts/crossnet-20261026/candidate-edf7cac。交付文件为GitHub资产原样副本，只改本地外层文件名：

| 文件 | 字节 | SHA256 |
|---|---:|---|
| LinkSend-crossnet-fix-edf7cac-windows-amd64.zip | 10,024,687 | 3b86d6f95c4ab2690b600536c74c2cbf5fc72a21e80256c47dcd6335ea843aab |
| LinkSend-crossnet-fix-edf7cac-windows-amd64-installer.exe | 75,295,391 | a57d8e9f3604ea38b019d6d1df7ad4e8cf53130b4a08dcfcd1944d016e3c73d5 |
| 解压后LinkSend.exe | 24,473,600 | c2e58406f79e5ca3e5ac8bc59299b548c54788640c1f3e6f330ebf507b742702 |

同目录package-verification.json保存来源与检查回执，修复候选说明.txt及双端试用步骤.txt说明操作。ZIP根部为LinkSend.exe，无bin子目录；便携运行默认仍沿用%APPDATA%/LinkSend（显式LINKSEND_DATA_DIR优先），不是自动隔离profile。两端先从托盘完全退出旧程序，再使用同候选，以免单实例唤回旧进程。自定义STUN及显式环境配置保持；空值/旧单内置默认会补备用。程序仍显示0.6.2，以BUILD-INFO的edf7cac区分；包内旧README的0.6.0为既有文案漂移，不代表载荷来源。安装器本轮未执行，桌面候选未启动，不倒记其他包的原生证据。

准确源码push CI [core](https://github.com/Wen5555/LinkSend/actions/runs/40214493502)与[desktop](https://github.com/Wen5555/LinkSend/actions/runs/40214493466)均success；上述packages的Windows amd64、Mac arm64、Mac amd64三job全部success，PR触发的[core](https://github.com/Wen5555/LinkSend/actions/runs/40215068288)及[desktop](https://github.com/Wen5555/LinkSend/actions/runs/40215068365)也均success。三平台构建不替代Mac原生旅程。

## 仍未证明

用户两台Windows使用准确修复包的现场闭环、双端受限NAT普遍成功、物理LAN/睡眠恢复及原R0–R6其余必做项仍分别验收。STUN可用不保证所有NAT或UDP受限网络可直连；不通过中继/关闭身份验证填补缺口。未合并main、发布Release或替换生产服务。
