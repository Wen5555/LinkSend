# M4/M5 接受确认整合

2026-09-12。隔离分支 `codex/desktop-acceptance-integration` 以主分支提交 `1a28213` 为基线（已含 M6 schema4 与 M5 wire），整合 M4 App `1788a9f`。未修改 desktop 或 shutdown.go，不覆盖主分支的原生退出修复。

## 最终协议与接口

- sender 的原始 V1 offer 保留 `receive_plan_v1`、`acceptance_commit_v1`，有内容描述符时另外包含 `content_v1`。Manifest、TransferID、ALPN 与签名不变；正文仍仅经认证 QUIC。
- receiver accept 同时回传 selection、原生内容 capability/content_digest 和 `commit_barrier=true`。sender 先校验能力、明确降级、内容摘要、选择和 verified 范围，再执行 SelectionAccepted 与 ContentAccepted；两个回调成功后才发送 `accepted`。
- accepted 同时绑定原始 digest、selection_digest、实际协商的 content_digest。receiver 在请求任何 chunk/ready 前核对三者；内容摘要缺失或不匹配返回 CONTENT_MISMATCH。旧端未协商屏障时不插入新控制消息。
- `SendHooks.ContentAccepted func(ContentAcceptance) error` 的参数包含 `Content *ContentDescriptor`、`ContentDigest string`、`FileFallback bool`。描述符为独立副本；普通文件也以 nil/空/false 调用，明确降级为 nil/空/true。回调修改参数不会改写网络解释。
- `Offer.FileFallback` 将实际降级事实交给接收 Plan 回调，且保持降级时 Content=nil；M5 App 可以持久化实际解释，旧 M4 回调不会因新字段自动获得原生内容权限。

源文件准备、摘要与内容语义验证可以在 offer 前进行；持久化回调约束的是传输正文帧的调度，不声称本地预处理不读源文件。M5 App 的内容数据库与安全动作继续由独立代理接线，不能把这里的 wire 通过当作完整原生 UI 已验收。

## 真实回归

`content_acceptance_test.go` 使用真实双 UDP socket、QUIC TLS 1.3 与生产 pin 校验。正文选择为与持久失败 error envelope 完全相同的 JSON，测试覆盖：原生文本、全跳过、明确文件降级、普通文件、SelectionAccepted 失败、ContentAccepted 失败、accepted 内容摘要篡改。

成功回调实际创建 JSON 接受记录并调用 File.Sync。观察到 accepted 时必须已经存在两份记录；失败时没有 accepted、发送与接收正文计数为零，目标文件不存在。全跳过保持 NoContent；普通 JSON 正文则完整落盘且字节数准确。描述符副本和 Offer.FileFallback 也有实际检查。

现有 text/URL/PNG 原生内容、全部跳过、旧端双向兼容和 checkpoint 测试继续保留。内容摘要篡改矩阵补入 accepted；测试首次运行发现把 CommitBarrier 插到 JSON 第一个字段会让旧测试的固定 op 前缀探针漏测 accept，已将字段追加到既有 control 字段之后，并验证所有摘要篡改点。

实际验证均退出 0：

- `go test ./internal/transfer -run 'TestContentSelectionAcceptance|TestContentEveryTerminal' -count=3`（3.002 s）。
- 根 workspace `go test ./...`；根 `GOWORK=off go test ./...`、`go vet ./...`、`go build ./...`。
- 根 `GOWORK=off go test -race ./...`：所有包通过；App 48.471 s、transfer 8.129 s、content 2.153 s。
- 桌面 module `GOWORK=off go mod verify`、`go test ./...`、`go vet ./...`、`go build ./...`，含 nativeclipboard 包。
- `git diff --check`；对基线的文件列表未包含 desktop 或 shutdown.go。

工具入口为 `. 'D:\apps\Osend\.tools\use-desktop-toolchain.ps1'`，版本沿用已核验的 Go1.27.1。没有执行本分支 push/deploy/release。这是真实本机 QUIC，不代表物理 LAN、双 NAT 或 macOS 原生运行。
