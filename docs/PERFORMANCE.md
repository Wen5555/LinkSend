# Performance

`BenchmarkTransport` 比较原生 quic-go UDP 与 Pion ICE 集成路径。已记录的 Windows amd64 loopback 探索结果约为 native 96.88 MB/s、ICE 97.05 MB/s，均为应用有效吞吐；该结果没有代表千兆网、2.5GbE、macOS 或 Linux。

```powershell
go run ./cmd/devtool bench-transport
```

正式报告必须同时写明 Go/Pion/quic-go 版本、CPU、网卡、链路、文件大小、块大小、CPU、峰值内存、分配和 GC。1 Gbps 的 125 MB/s、2.5 Gbps 的 312.5 MB/s 只是扣除开销前换算上限，不是产品承诺。
