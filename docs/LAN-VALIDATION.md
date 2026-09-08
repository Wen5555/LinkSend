# LAN validation matrix

## 当前证据边界

历史 `docs/STAGE2A-20260909.md` 证明 macOS→Windows 单次 16 MiB LAN PASS。本轮 Windows 端可执行的自动化验证已通过；无法从当前环境直接读取或控制 macOS，因此 LAN-01 双向六次实机重复仍为 `NOT_RUN`，不得将历史单次结果扩展为双向重复通过。

## Windows 当前网络前置（2026-09-09）

`go run ./cmd/devtool network-info` 显示物理接口“以太网”已启用，IPv4 `10.234.232.205/16`；`Mihomo` 为 `198.18.0.1/30` 虚拟接口，不用于 LAN bind。默认路由和 macOS 当前接口仍需在两端现场确认。

## LAN-01 / LAN-02 / LAN-03 状态

- LAN-01：`NOT_RUN`（需要 macOS 现场双向 3+3 次、独立接收目录和双端 SHA256）。
- LAN-02：自动化 transfer 矩阵覆盖空文件、目录、冲突、路径安全和取消；Windows/macOS 实机矩阵 `NOT_RUN`。
- LAN-03：底层取消、超时、checkpoint 测试已通过；CLI 应用层 `cancel/status/resume` 仍未实现，进程重启恢复不宣称完成。

每次实机运行应保存 `.lan-tests/<run-id>/` 的 run.json、双端 result/stderr、network-info、哈希与退出码。接收端使用 `--wait-timeout`，发送端和接收端均先构建并记录各自二进制 SHA256。

## macOS 操作包

在仓库根目录执行 `bash scripts/stage2a.sh --role receiver|sender ...`，使用现场 `ifconfig`/`route -n get default` 确认物理 IPv4；不要填写 198.18.x.x、utun 或代理地址。每次使用新的 `--data-dir`/接收目录和唯一证据目录，回传 result.json、stderr.log、run.json、binary.sha256 及最终文件 SHA256。

## 本轮桌面任务接口

Wails 桌面端通过 `internal/app` 的进程内任务服务调用现有真实直连链路。发送支持文件和目录；接收先进入等待，收到已验证对端提议后由任务页确认或拒绝。任务快照提供状态/阶段/字节计数/错误码，取消显示请求后等待后端确认，失败的发送可重新发送并生成新任务 ID。任务不持久化，应用重启恢复仍未实现。

桌面启动前可设置 `LINKSEND_SERVER_URL`、`LINKSEND_BIND`、`LINKSEND_STUN`（逗号分隔）和仅用于 loopback 开发的 `LINKSEND_ALLOW_INSECURE_LOOPBACK=true`。生产 LAN 应使用实际物理接口地址，不使用 198.18.x.x 虚拟接口。
