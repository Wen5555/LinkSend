# R5：等待已批准对端与网络期限分离（2026-10-26）

基线 `8082b2c7f3c0c92746d98a638933fa00189568ae`。此增量解决批准后等待另一端明确进入分享，却消耗网络短期限的问题。原生场景02/03仍保留失败，不将本地回归倒记为原生或公网成功。

## 行为与授权边界

ConnectShare 的 HTTP/WSS 建连继续使用短 CheckTimeout。已认证后以独立 WaitTimeout 等待同一已批准对端，未设置时取同一短预算值；原运行 context 与原阶段绝对期限进一步约束该等待。preview 使用原 ExpiresAt，transfer 使用原 ResumeUntil，不滑动授权期限、不创建新 claim/task。

新本地阶段 waiting_share_peer 显示“等待对方进入本次分享”，沿既有 preparing 外部任务状态，提供安全等待指导。对端到场后，再检查实际 wait context、原运行取消、黑名单、正向 owner/scope 授权及原绝对期限；通过后才创建 endpoint，并为候选/ICE/QUIC启用新的短网络预算。已到场 responder 等待 connect_request 也使用网络预算，不再给另一段人机等待。

ShareSession 的 request/response 直接保留原 typed cause 和权威 Share/Auth 错误，不经过普通 readSignalMessage 的期限改写。原普通 trickle 实现未改，direct.go 仅补 WaitTimeout 注释。成功返回后临时 scope/network context 的清理不会关闭保留的 WSS/QUIC；失败关闭并等待 reader，heartbeat仍绑定原 run。

同期限竞争以实际已发生的 context 原因及原授权到期判断；先发生的明确取消保留 CANCELLED，已到期的 cause 不因后来取消逆转。正常存储路径中 owner 先持久 stopped/cancelled 再取消 worker；持久失败也保留内存拒绝并取消，不宣称持久成功。晚到失败不能覆盖已停止状态。原 TaskRecordID/claim/task/generation/manifest/plan、明确保存、独立 preview→transfer scope、源变化检查、暂停/重启恢复及撤权保持。

只新增本地阶段及安全投影，无 wire/schema/公钥固定/权限默认值变化，不授予普通会话复用或剪贴板权限，不自动重试。临时分享仍为实验能力、file_transfer_ready=false，R5未整体完成。

## 因果旧红与实质回归

旧实现加每 Service 私有双 context 观察屏障，在真实 SQLite/TLS/WSS 认证后，观察到实际初始 Check context 到期才允许另一端进入；发送端在更长 WaitTimeout 前以 SIGNALING_TIMEOUT 失败。`share-peer-wait-before.log`：用例1.07s，app7.600s，exit1。

实际旧红命令（D:/apps/Osend，锁定工具链，GOWORK=off）：

```text
go test -count=1 -run '^TestShareDirectPeerMayEnterAfterAuthenticatedCheckDeadline$' -v ./internal/app
```

修后同一屏障才放行迟到对端，真实建立固定公钥 ICE/QUIC、TLS1.3、relay=false。没有用固定 sleep 猜超过期限，没有延长原 grant。另以隔离库的合法已老化 grant 与真实 server 时钟验证 preview/transfer 各自到期，未修改产品 TTL 或宿主时钟。

中间失败均保留：缺少响应的测试暴露普通 readSignalMessage 丢掉 cause，专属 share 读取路径修复；另一个新断言误把实际任务状态 preparing 写成 connecting，已按真实 owner 状态修正，不为测试改变产品任务状态。原日志见 `.artifacts/share-owner-draft/share-peer-wait-{expanded,initial-state-expectation-failure}.log`。

最终精确命令脚本 `.artifacts/share-owner-draft/verify-share-peer-wait.ps1`，组合包括全部 TestShareDirect/TestSharePreview/TestTemporaryShare 及相关诊断，普通/race各3轮：

- `share-peer-wait-final-test.log`：exit0，app84.740s/命令87642ms。
- `share-peer-wait-final-race.log`：exit0，app194.229s/命令206926ms，无race报告。
- 相关前端55项、typecheck/绑定检查、lint均exit0；root完整19files/172tests通过，3.15s/命令3750ms。
- root `pnpm run build` exit0/6757ms，在 Go embed 前完成。JS `index-C4Ky9D8S.js` SHA256 `c791cc0fef15dc205427378a9479c0222fc9d7b9b9dcd7f2c43e4d0fa810919e`。

覆盖短 HTTP 建连、独立等待/未设置回退、原 run 和两阶段绝对期限、同期限竞争、已 ready 仍不得越过等待到期、到场后 owner/黑名单复核、短网络响应期限、重复取消后重新真实连接、撤销/Auth码保留、持久原任务，以及源/计划/保存/暂停重启/退出回归。原生剪贴板 opt-in 未运行。

## 同版完整矩阵及 CI 边界

root 将上述10份冻结源与独立[trickle 测试前提修正](RELIABILITY-R1-TRICKLE-CAUSAL-CHECK.md)一并核对，再对两个模块各在 workspace/off 运行 `go test -count=1 ./...`、`go vet ./...`、`go build ./...`。12命令全exit0，`peer-wait-final-matrix.json`、`peer-wait-final-{core|desktop}-{workspace|off}-{test|vet|build}.log`：

| 模块/模式 | test ms | vet ms | build ms |
|---|---:|---:|---:|
| core workspace | 149493 | 1376 | 6335 |
| core off | 149855 | 1368 | 6125 |
| desktop workspace | 21256 | 953 | 4528 |
| desktop off | 18025 | 809 | 4059 |

8082的准确core40247491025/desktop40247490982成功，packages40247490996最终失败：Windows/ARM Mac成功，Intel Mac在普通核心回归的原trickle用例超时。该失败触发了单独的真实因果实验及测试修正，不重跑旧CI抹去失败。新提交的准确三平台CI、包与Wails实机另核对；本轮无新公网/双NAT/Win↔Mac或完整Win↔Win证据，不合并main、发Release或替换生产。
