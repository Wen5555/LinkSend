# Stage 2B readiness

当前状态：暂缓跨 NAT，2A 同 LAN（Windows 接收、macOS 发送）已由 `docs/STAGE2A-20260909.md` 记录为 PASS。跨 NAT、IPv6、网络切换和桌面任务 UI 尚未验收。

本轮加固已完成：两端脚本固定仓库根目录；`--skip-build` 必须显式提供已构建二进制并记录 SHA256；角色值受限；传输的 JSON 结果与 stderr 分离并保留失败结果。接收端可用 `--wait-timeout` 单独延长等待首个请求的时间，ICE/QUIC 阶段仍使用连接超时。

跨 NAT 验收暂缓。本轮优先完成 Windows/macOS 局域网可靠性与文件矩阵，详见 docs/LAN-VALIDATION.md。每次运行使用独立 `.stage2b/<run-id>` 目录，记录提交、工作树状态、二进制哈希、拓扑、候选对、TLS/ALPN、独立文件 SHA256、端点计数与错误阶段。`transfer_outcome` 与 `test_verdict` 分开；未运行或证据不足保持 NOT_RUN/BLOCKED_BY_EXTERNAL_ENV。

`test-nat` 仍是 Linux 特权 fixture 的独立交付项；香港主站和荷兰小鸡可用于后续 Linux namespace/UDP 规则测试，但本轮未执行。
