# Testing

根模块的单元测试覆盖身份、公钥替换、候选策略、分帧、BLAKE3、危险路径、恢复状态、取消和异常关闭。`tests/integration` 的 `TestLocalDirectDemo` 启动临时 SQLite/WSS 服务，使用两个独立身份执行真实 ICE、TLS 1.3、QUIC 和文件哈希校验。

```powershell
go test ./...
go run ./cmd/devtool demo-local
go run ./cmd/devtool test-nat
```

`demo-local` 只能证明 loopback direct。`test-nat` 在当前 Windows 环境明确返回未运行，因为受控双 NAT 需要 Linux namespace、UDP 规则和特权。相同 bridge 上的容器不能算双 NAT。

目标验收还需要两台真实设备、不同网络、IPv6、睡眠唤醒和公网/家庭热点场景。每次运行应保留候选对、基础 socket、路径、传输哈希、信令/STUN 计数和错误阶段。
