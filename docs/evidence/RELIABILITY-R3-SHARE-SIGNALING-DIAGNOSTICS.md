# R3/R5：保留信令原因及安全下一步（2026-10-26）

产品基线52f9bf2，随后文档提交7c6ea3f。此增量修复已知信令阶段的错误分类与展示，不改变连接期限、身份、授权、wire、schema、自动重试或文件计划。准确新提交/CI另核对，不把旧包或原生失败改记通过。

## 因果复现

[准确3d原生场景](RELIABILITY-R5-NATIVE-GUI-20261026.md)实际停在 signaling_connect/DIRECT_FAILED，报告 guidance 为 unknown。该旧现场没有原始 cause，不能仅凭文本倒推必为某条超时分支。

本轮以真实 SQLite、固定身份的 TLS/WSS 和单独到场的合法 actor 复现：每 Service 私有屏障证明已经完成 scoped WSS 认证、即将 WaitPeer，随后才触发期限或取消。旧实现返回 DIRECT_FAILED，但 errors.Is 分别确认原 cause 是 context.DeadlineExceeded/context.Canceled；真实任务与签名 share owner 也把已知信令超时持久成 DIRECT_FAILED。旧实现定向 FAIL/app7.872s/命令28971ms/exit1，`share-signaling-diagnostics-before.log`；前端旧实现41项中15 FAIL/26 PASS，`share-signaling-ui-before.log`，18505ms/exit1。没有以日志字符串猜业务原因。

## 修改及保护边界

- 仅在 connectSharePeer 的 ConnectShare/WaitPeer 已知 signaling 阶段包装裸 context 原因，保留 errors.Is；任何已有 protocol.Error，包括 Share/Auth/终态/InvalidMessage/Replay，在取消竞争时原样保留。普通文件操作的 deadline 不全局改成信令超时。
- 旧记录只在稳定 DIRECT_FAILED 且精确 Phase=signaling_connect 时显示“连接协调未完成”及网络设置/原任务明确继续入口；缺失、其它或拼接 phase 仍 unknown，完成任务清对应旧错。不读取 ErrorMessage 来分类，不推断 ICE/NAT 根因。
- 临时分享行增加固定 P2P、协调及 RATE_LIMITED 提示；不显示任意 detail，不自动重试、不重复创建/领取、不改变原 intent/claim/local ID。没有手动缓存清理、容量或期限配置。
- 每 Service 的 beforeSignalWait 仅是测试前赋值的私有因果门，无全局 hook、线上调用者或 context 存储；测试 fixture 改用真实 TLS/WSS 及 httptest 的独立证书信任，不装系统 CA 或关闭验证。

开发中撤销负例曾错把原 client 的拒绝预期为 SHARE_REJECTED，实际是原有 INVALID_MESSAGE；失败保留在 `share-signaling-diagnostics-after-first.log`。最终比较同一真实 client 的原 typed 结果，要求 wrapper 不覆盖且不能进入 WaitPeer；没有为通过测试修改该 client 协议解析。

## 实际验证

PowerShell7，`. .tools/use-desktop-toolchain.ps1`，Go1.27.1、Node24.21.0、pnpm12.4.1。日志根 `.artifacts/share-owner-draft/`：

- `GOWORK=off go test -count=3 -run '^(TestShareDirect|TestSharePreview|TestTemporaryShareMissingSignalPeer|TestDiagnosticLegacyDirectFailure|TestDiagnosticGuidance|TestDiagnosticReportAllowlist)' -v ./internal/app`：exit0，app14.049s/命令18106ms，`share-signaling-diagnostics-final-test.log`。
- 同一受影响组合 race3轮 exit0，app26.071s/命令41398ms，`share-signaling-diagnostics-final-race.log`。
- TemporarySharePanel 41项通过；前端完整19files/171tests通过，3.10s/命令3759ms（`.artifacts/share-ui-draft/share-signaling-ui-final-all-tests.log`）。typecheck/绑定检查和 lint exit0，未补造当次未记录的耗时。
- root 同版完成 `pnpm run build`，exit0/7241ms，先生成 dist 后才执行 Go embed 检查；原始 `diagnostics-final-frontend-build.log`。JS `index-BG_-w4sY.js` SHA256 `e6b3333f7429592c6b596c321d8cbd6f71a2848fc99d4af91ab8ef9f13b9d009`。

两个 Go 模块分别在 workspace 和 GOWORK=off 运行 `go test -count=1 ./...`、`go vet ./...`、`go build ./...`，12条均 exit0。`diagnostics-final-matrix.json` 与 `diagnostics-final-{core|desktop}-{workspace|off}-{test|vet|build}.log` 保存实际命令和结果，冻结七份源码无漂移；耗时毫秒：

| 模块/模式 | test | vet | build |
|---|---:|---:|---:|
| core workspace | 144322 | 2626 | 6452 |
| core off | 136940 | 1644 | 6288 |
| desktop workspace | 28619 | 1255 | 4710 |
| desktop off | 19652 | 1126 | 4986 |

覆盖真实认证后的期限/取消、权威协议码优先、持久任务/share 错误、撤销、精确 legacy phase 与未知输入、完成清错、私密字段不入安全 DTO/DOM，以及限频后重渲染、推进60秒、本机重读均不自动派发；显式刷新仍用原 localID，pending claim 重试保持原契约。七份冻结源 SHA 保存在 `share-signaling-diagnostics-source-hashes.json`。

原30/35秒等待另一端进入的体验问题未改；需要单独区分网络连接 CheckTimeout 与已批准对端的人机等待，并受原 grant 绝对期限及取消/撤销约束。本增量未重跑准确候选原生/P2P旅程，没有运行本机 native clipboard opt-in。R1公网现场、R2完整回收/键盘粘贴、双平台旅程及 R0–R6 整体保持未完成。
