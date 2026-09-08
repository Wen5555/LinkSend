# Testing

根模块的单元测试覆盖身份、公钥替换、候选策略、分帧、BLAKE3、危险路径、恢复状态、取消和异常关闭。`tests/integration` 的 `TestLocalDirectDemo` 启动临时 SQLite/WSS 服务，使用两个独立身份执行真实 ICE、TLS 1.3、QUIC 和文件哈希校验。

```powershell
go test ./...
go vet ./...
go run ./cmd/devtool demo-local
go run ./cmd/devtool test-nat
```

`demo-local` 只能证明 loopback direct。`test-nat` 在当前 Windows 环境明确返回未运行，因为受控双 NAT 需要 Linux namespace、UDP 规则和特权。相同 bridge 上的容器不能算双 NAT。

`net.Pipe` 测试只覆盖 transport-neutral fallback，不能证明 quic-go 行为。Stage 1B 的真实 loopback 测试现在使用 quic-go v0.62.0 的 client/server stream，覆盖 acceptance read、receive read、ACK wait 和重复 abort；它们不能替代 LAN/NAT 验收。

Candidate exchange 测试覆盖发送失败、接收失败、缺少 `end_of_candidates`、父 context 取消和 malformed message，并使用确定的 done/context 同步点。阶段错误保留 `CANCELLED`、`CANDIDATE_EXCHANGE_TIMEOUT`、`CHECK_TIMEOUT`、`NO_VIABLE_CANDIDATE`、`ICE_FAILED` 和 QUIC handshake 分类，不能统一写成一个连接超时。

传输测试还覆盖恶意 accept/ack 的 verified 上界、受限 peer error、恢复 JSON 尾随数据和未知文件 ID。`linksend send --evidence` 和 `receive --evidence` 会导出实际候选对、共享 socket、STUN 计数和 TLS/ALPN，不导出 ICE credentials、令牌或私钥。

桌面模块必须从 `apps/desktop` 且 `GOWORK=off` 执行，并确认 `go env GOMOD GOWORK` 指向桌面 `go.mod` 和 `off`。前端检查从 `apps/desktop/frontend` 执行。Wails production build、Docker runtime、HTTPS 证书身份验证、coturn STUN Binding/TURN Allocate 负例均需单独记录，不能由 compose config 或 liveness healthcheck 代替。

目标验收还需要两台真实设备、不同网络、IPv6、睡眠唤醒和公网/家庭热点场景。每次运行应保留候选对、基础 socket、路径、传输哈希、信令/STUN 计数和错误阶段。`tests/natlab` 当前没有执行器；`test-nat` 的非零结果表示 not-run，不是 NAT 通过。
