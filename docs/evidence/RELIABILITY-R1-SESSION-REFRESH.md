# R1：网络通知保留健康会话（2026-10-19）

基线 `4e2275dff4cf7e7ada9708cc9f090cc07e61906d`，工作分支 `codex/reliability-sharing-plan-20261019`。仅修改网络通知时的会话清理条件与实质回归，不修改文件/剪贴板 wire、schema、授权、公钥固定或正文路径；`relay=false`。

## 实际缺陷与修复

`NetworkChanged` 原先关闭全部没有文件流占用的会话。但这些连接可能持有正常的剪贴板双向租约；系统唤醒或无关地址变化会主动破坏仍可用的认证 QUIC。原测试只检查活动文件 task 的取消状态，没有覆盖这种文件空闲而剪贴板活跃的连接。

新增 `TestClipboardLANNetworkRefreshPreservesHealthySession` 复用真实 LAN TLS 控制、Pion ICE 和固定身份 QUIC。原实现运行退出1，明确失败 `network notification destroyed healthy authenticated clipboard session`；日志 `r1-before.log`。没有修改超时或删除失败断言。

修复后网络通知仍清除未来 endpoint 选择缓存并刷新发现，只清理 QUIC 已报告关闭且无文件流占用的连接。健康连接交给既有 transport/空闲生命周期管理；真实路径故障仍通过原断线/重认证/任务恢复处理，不强行保持失效路径。

回归断言：通知前无文件 task；双端保留同一认证 SessionID；通知后双向剪贴板真实传递；超过文件空闲 TTL 后仍用同一 SessionID 完成文件引擎传输与双方确认，并读取实际目标校验内容；远端随后关闭时等待真实 QUIC context 结束，再确认通知清理该失效连接。

## 本地验证

Windows amd64，`. ./.tools/use-desktop-toolchain.ps1`，Go1.27.1；未操作系统 general 剪贴板，adapter 仅使用测试内存。

- 原代码定向测试退出1；修复后同一用例退出0（app 10.353秒）。
- 以下定向组合 `go test -count=3 -run '<下列五项>' ./internal/app` 退出0（33.614秒）：新网络通知回归、`TestNetworkChangeInvalidatesFutureSelectionWithoutCancellingActiveTask`、`TestDevicesAuthenticatedConnectionLifetime`、`TestClipboardLANConcurrentBootstrap`、`TestPooledPeerRemoteIdleCloseBeforeOfferReconnectsOnce`。
- 根模块和 desktop 模块各自 workspace / `GOWORK=off` 的 `go test ./...`、`go vet ./...`、`go build ./...` 全部退出0。每个命令独立保留日志 `r1-{core|desktop}-{workspace|off}-{test|vet|build}.log`。
- 相同五项组合 `GOWORK=off go test -race -count=3 -run '<上述五项>' ./internal/app` 退出0（42.175秒），日志 `r1-race.log`。提交与远端SHA及CI作为后续交付证据追加，未结束CI不计通过。

日志位于 `.artifacts/reliability-20261019/`。前端未修改，不重复前端矩阵；代码推送触发正常 CI，不跳过。此次同机 loopback 只证明真实协议/会话生命周期，不能代替物理双机、系统睡眠或真实不同网络。

## 仍需完成

R1整体未完成：物理 Windows↔Mac 双向 LAN 文件集合/hash、中断恢复、原生睡眠/网络变化、真实不同网络双向传输待同候选现场核验。R2–R6按唯一 TODO 继续；本补丁不把旧包的任何 PASS 转记到新候选。
