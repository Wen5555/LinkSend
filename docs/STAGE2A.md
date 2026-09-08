# Stage 2A 实机验收

2A 只接受两台真实设备的证据：Windows 与 macOS 在同一局域网完成一次直连传输。loopback、单机 demo 和模拟 NAT 不计入通过。macOS 端使用仓库中的 `scripts/stage2a.sh`，无需预先安装项目到固定目录。

## 一键入口

Windows PowerShell：

```powershell
.\scripts\stage2a.ps1 -Role preflight -Server wss://linksend.example.com -Stun stun:stun.example.com:3478
.\scripts\stage2a.ps1 -Role receiver -Server wss://linksend.example.com -Stun stun:stun.example.com:3478 -Bind 192.168.1.20:0 -DataDir .stage2a/win -ReceiveDir .stage2a/received
.\scripts\stage2a.ps1 -Role sender -Server wss://linksend.example.com -Stun stun:stun.example.com:3478 -Bind 192.168.1.30:0 -DataDir .stage2a/mac -Peer PEER_ID -File .\fixture.bin
```

macOS/Linux：

```bash
./scripts/stage2a.sh --role preflight --server wss://linksend.example.com --stun stun:stun.example.com:3478
./scripts/stage2a.sh --role receiver --server wss://linksend.example.com --stun stun:stun.example.com:3478 --bind 192.168.1.30:0 --data-dir .stage2a/mac --receive-dir .stage2a/received
./scripts/stage2a.sh --role sender --server wss://linksend.example.com --stun stun:stun.example.com:3478 --bind 192.168.1.20:0 --data-dir .stage2a/win --peer PEER_ID -- ./fixture.bin
```

先在两端完成 bootstrap/join、OOB 指纹确认和 `trust`。`preflight` 会拒绝 loopback 信令地址、解析 DNS、记录 Go 版本并构建 CLI；`receiver`/`sender` 会把 `run.json`、构建日志和带 `--evidence` 的传输输出写入 `.stage2a/<时间戳>/`。不要把该目录中的身份数据库或令牌提交到 Git。

## 判定与记录

通过条件是：两端日志均无错误，输出包含已认证 TLS/QUIC、选定候选对为直连、`relay=false`，接收文件 BLAKE3 与发送端一致，且信令日志没有文件大小级转发。失败时保留完整 `transfer.log`，按 signaling、ICE、TLS、QUIC、transfer 阶段归类；不要把“连接超时”改写成 NAT 或防火墙结论。跨 NAT、IPv6、睡眠唤醒和 macOS 桌面运行仍需单独记录为未运行。
