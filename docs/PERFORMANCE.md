# Performance

2026-09-12 接收端消除了 `VerifiedBytes()` 在每个 ACK 上扫描完整位图的平方级热点，改为写入成功后单调 O(1) 累加。每个块仍先执行文件 `Sync`；恢复 JSON 位图以 8 块或 500ms 为上限批量 checkpoint，进程崩溃后通过重新哈希 staging 恢复落后位图。该改动优先改善大文件高块数和海量小文件，不把少量 loopback 数字外推为物理网络吞吐承诺。

当前数据适用于产品 `0.4.0` 的实现级回归边界；协议为 V1，Pion ICE v4.4.2，quic-go v0.62.0。没有新物理跨平台吞吐基准时，不得把恢复正确性测试的字节计数解释为性能结论。

`BenchmarkTransport` 比较同版本原生 quic-go UDP 与 Pion ICE 集成路径。Windows amd64 loopback 探索结果约为 native 96.88 MB/s、ICE 97.05 MB/s，均是应用有效吞吐，只证明本机 loopback 基线，不代表千兆网、2.5GbE、macOS、Linux、跨 NAT 或网络切换性能。

2026-09-12 物理 Windows `10.234.232.205` → Mac `10.234.171.192` 小文件连续发送对照：修复前每轮新建 WSS 为 3,521–3,749 ms；WSS 读泵复用、网卡预热、响应端预注册 QUIC listener、显式 `confirmed_ack` 与后台 draining 后，10/10 为 1,296–1,604 ms，平均 1,452 ms。发送端 WSS 借用为 0–0.7 ms，endpoint 约 81–90 ms，QUIC 握手多数 11–15 ms；Mac `connection_count=1`。全部是 host↔host `lan_direct`、TLS 1.3/QUIC、`relay=false`，只表示该现场的小文件及时性，不是跨 NAT 或吞吐承诺。

```powershell
go run ./cmd/devtool bench-transport
```

恢复测试必须同时报告实际发送、实际接收、重传、唯一验证和提交字节。当前自动化证据中：12 MiB 无损恢复发送/接收 12,582,912 bytes、重传 0；首个 4 MiB staging 块损坏后发送/接收 16,777,216 bytes、重传 4,194,304 bytes、唯一验证/提交仍为 12,582,912 bytes；完整 Service 重启恢复为 8,388,608 bytes、重传 0。完整重发不得标成字节续传。

正式性能报告必须写明产品/协议/Go/Pion/quic-go 版本、OS/CPU/内存、网卡和链路、拓扑、候选对、文件/块大小、吞吐单位、CPU、峰值内存、分配与 GC。1 Gbps 的 125 MB/s 和 2.5 Gbps 的 312.5 MB/s 只是扣除开销前的换算上限，不是产品承诺。
