# R1：候选收集结束的因果回归（2026-10-19）

## 真实失败与边界

`1bc92e0754805b01fd6a15b78fdfa3b840bc4f55` 的core [39172841253](https://github.com/Wen5555/LinkSend/actions/runs/39172841253)与desktop [39172841256](https://github.com/Wen5555/LinkSend/actions/runs/39172841256)成功；包 [39172841244](https://github.com/Wen5555/LinkSend/actions/runs/39172841244)的Windows amd64/Intel Mac成功，Mac arm64失败，不能记三平台包全部通过。

失败在既有`TestTrickleNominatedHostPairDoesNotWaitForUnresponsiveSTUN`：总耗时3.002311167秒，旧断言把所有超过2秒的端到端耗时都解释为等待3秒STUN收集。此日志没有分阶段时刻，不能据此断定是应用等待收集结束、Pion检查、调度或其他延迟。保留原FAIL，未用重跑清除该记录。

## 本次更改

生产入口继续把`endpoint.Candidates()`传给同一内部实现，Pion检查、真实UDP、签名交换、角色、身份和完成条件均保持。测试把候选源作为内部参数，透传全部真实Pion候选，但故意永不关闭两端候选流：如果实现依赖收集结束，原6秒父期限内必然失败；只有真实选中host对后的显式结束分支能成功。

取消了“总耗时必须小于2秒”这一无法定位原因的因果推断，保留实际耗时日志及原6秒连接期限。没有提高超时、跳过STUN、伪造ICE或减少签名/身份检查；不再用该用例承诺墙钟性能。另建独立负例，禁用选中后结束分支，确认新用例确实捕获等待错误。

## 实际验证

- Windows Go1.27.1：原用例普通30轮退出0（`r1-trickle-mac-ci-windows-repeat.log`，8.538秒）。新因果用例加现有交换失败/取消/期限等普通30轮退出0（`r1-trickle-causal-ordinary.log`，10.178秒），对应race30轮退出0（`r1-trickle-causal-race.log`，11.301秒）。
- SSH别名`mac-test-102342413`，MacBook arm64/macOS26.5，实际Go1.27.1。原准确`1bc92e0`源码由git archive导出，tar SHA256 `4490aaf1d2981003364a23f1a10b8d71bfe3f20c130e1bd161fafedac11a096c`，远端先核对摘要再解包。旧用例普通50轮/race30轮均退出0（0.580秒/1.739秒）；未复现CI的长尾，不把本机成功当作CI旧失败不存在。
- 只应用本批两个direct文件的补丁后，新因果用例Mac普通100轮/race50轮退出0（0.612秒/1.820秒）；真实host对、两端未关闭候选流、STUN被实际探测且不回答共同成立。日志`mac-trickle-causal-{ordinary|race}.log`。
- 已应用的精确补丁`.artifacts/reliability-20261019/r1-trickle-causal.patch` SHA256为`0becd8ab71d6409f2e6673b8ef525894afe8eac608c7e50d6e4f2039149a6f8c`；完整Mac矩阵的app workspace61.235秒/off59.813秒，transfer8.082秒/6.146秒。
- 独立新临时目录中的负例只禁用`close(stopGathering)`，原用例在6.00秒返回`CANDIDATE_EXCHANGE_TIMEOUT`并退出1；负例验证脚本按预期失败退出0（`mac-trickle-negative.log`）。未在工作区或完整矩阵目录中注入失败。
- Mac准确基线加本批补丁，两个Go模块workspace/off的`test -count=1 ./...`、`vet ./...`、`build ./...`共12命令全部退出0。根与桌面模块独立核对，不混入正在开发的R3诊断文件。

远端请求与证据目录：

- `ls-r1-trickle-mac-ci-20261019-001` → `/tmp/codex-ssh/job-ls-r1-trickle-mac-ci-20261019-001`，退出0。
- `ls-r1-trickle-causal-mac-20261019-001` → `/tmp/codex-ssh/job-ls-r1-trickle-causal-mac-20261019-001`，退出0。
- `ls-r1-causal-mac-matrix-20261019-001` → `/tmp/codex-ssh/job-ls-r1-causal-mac-matrix-20261019-001`，12命令退出0。
- `ls-r1-trickle-negative-mac-20261019-001` → `/tmp/codex-ssh/job-ls-r1-trickle-negative-mac-20261019-001`，按预期负例脚本退出0，内部Go测试退出1。

测试源码/日志在`/tmp/linksend-reliability-1bc92e0-mac-ci-20261019`和独立negative目录。已下载四组普通/race、负例及四组完整test日志至`.artifacts/reliability-20261019/`；完整12命令日志仍在远端`matrix-r1-causal/`。没有操作日常应用、用户剪贴板、安装或宿主网络配置；本批是同机协议/生命周期检查，不替代R1物理LAN、恢复、睡眠或跨网，也不替代R2键盘粘贴。

推送后另核对新提交三组CI，旧Mac arm64包仍记FAIL；只有新准确源码包通过才补新PASS。

后续核对：新提交565840ec157f933f3d3dacb5100130e6a2c1ea2e已推送且远端一致，core39174347344、desktop39174347400、packages39174347352全部completed/success，Windows/Mac arm64/Mac amd64包job均成功。1bc92e0的旧Mac arm64包仍FAIL；本批不宣称物理跨网或墙钟性能通过。

## 2026-10-26 8082 Intel Mac 再次失败与 host 前提

准确8082b2c的core40247491025/desktop40247490982成功；packages40247490996的Windows/ARM Mac成功，Intel Mac job121332050999在 `GOWORK=off go test ./...` 的本用例6.00s返回CANDIDATE_EXCHANGE_TIMEOUT（原日志 `.artifacts/share-owner-draft/diagnostics-ci-8082b2c-macos-amd64.log:409`）。当次无双方 provisional path 日志，不能断言那次具体根因。

干净701验证树中相关三个源文件与8082一致。独立Go overlay只控制真实签名候选到达顺序：暂缓A候选，直到B的真实ConnectProvisional返回，再放行。实际观察B为host/prflx、relay=false、err=nil，双方最终6.0004326s返回同码。Pion可先从真实STUN检查学习prflx；旧fixture只扣住gathering-end，却未保证host已安装，其前提可能与只为host/host结束交换的分支不一致。此为可复现机制，不倒推原CI具体路径。

修正仅在 `direct_lifecycle_test.go`：checks前，两端真实首个loopback host候选经原 `exchangeCandidatesWithBudget` 的签名/session验证与AddRemoteCandidate，之后同候选再通过原永不关闭流，保留6秒总期限、host/host/no-relay断言和STUN sink探测。没有修改生产trickle、伪装candidate type或关闭身份校验。

`.artifacts/trickle-ci-8082b2c-isolation/RESULT.md` 与实际overlay命令/日志保存来源：普通30次PASS/app9.179s、race20次PASS/app7.315s；同一barrier只禁用close(stopGathering)的独立负例仍6.10s按预期FAIL。冻结测试SHA256 `4e0592a090e14517d542d4e166a1a5e3b880f8c6842540757732344a7da778cc`。随后root同版完整12矩阵通过，见[等待增量](RELIABILITY-R5-PEER-WAIT.md)；新Mac CI另验，不声明当前物理Mac或跨网成功。
