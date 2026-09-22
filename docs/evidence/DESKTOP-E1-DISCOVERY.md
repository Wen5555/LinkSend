# E1-04 发现证据

## 2026-09-23 暂停时的本地 U3 候选（未合入主分支）

**用户已终止实施。以下描述的是 `8fcc76d` 之后保留的本地未提交候选，不是主分支当前行为。** 最后 reader 状态原子性修复仍待最终验证，源文件与补丁已备份，详见 E5-X；当前不继续执行本节后续工作。

原初始固定 UDP bind 失败会关闭已经开启的 TLS listener。本批在固定端口不可绑定时使用 OS 分配的临时 UDP，保持 cfg 的发现目标端口和公告内的 TCP/TLS 控制端口；Manager.ChannelErrors 的 `listen=fixed_port_unavailable` 只在固定入口实际恢复后清除。显式 Refresh 与既有四秒 tick 先尝试绑定新 socket，失败保留健康临时路径，成功交换后关闭旧 socket、重新加入组；readLoop 跟随新 socket。临时端口不能接收发送到被占用固定入口的公告，不把降级表述为完整发现能力。临时 UDP 也无法绑定时仍明确失败。

重叠网段的受限单播不再于第一条 Write 成功后停止，而是尝试全部匹配路由，最多 32 条；超过上限明确失败。Windows 活跃源 socket 仍最多 32 个，关闭后禁止新发送/worker 并取消已有响应等待。所有探测仍只针对 on-link 地址，不扫描网段；UDP 写成功只是已发出探测。

LAN 配对落盘、经验证的 done 恢复，以及认证连接的地址持久化成功后，会把地址加入当前 Manager 的有界记忆重探，不再等应用重启。拒绝、未提交及被撤销身份不添加；Manager 满或不可用不会把已完成的持久配对变成失败。原有重启装载、最多八条持久历史和 64 个 runtime 地址上限保持；地址提示不授信，后续仍核对签名和 TLS 身份。

本机真实自有 wildcard 占位复现了初始失败，修复后验证临时源签名 UDP 应答、篡改公告与错误 pin 拒绝、双向 TLS heartbeat、占用持续时保留原 socket、释放后的手工/ticker 恢复、新 reader 接收、两个真实 loopback 源地址与 32 路限制、Close 竞态及端口释放。最初仅绑定 127.0.0.1 的占位夹具在 Windows 不排斥 wildcard，已纠正并保留失败日志，没有把它作为端口冲突证据。app 的真实 LAN 同意/恢复和拒绝、身份/容量边界也通过定向 race。完整集成与来源见 E5-X，原始日志 `.artifacts/astra-resume/discovery-recovery/` 与 `lan-memory/`。

这轮没有修改宿主网络、连接 Mac 或进行物理 LAN/NAT；Linux/macOS 交叉编译不等于执行，多源 fixture 在不能绑定额外 loopback 地址的主机明确 SKIP。刷新 burst/抖动、打开设备页触发发现及向桌面诊断暴露 provider 降级仍是明确下一步，不能因 Manager 内部状态存在就写成用户诊断已完成。

## 原 E1 实施记录

`refreshInterfaces` 现保留同一接口的全部有效IPv4地址；route记录远端、接口、本地源、family、network generation和最后出现时间。同一DeviceID多地址合并且每设备最多16条route、全局最多512 peer；replay/响应限流表和TLS握手并发均有界，过期清理会回收状态。

组播加入失败只记录provider降级，不再关闭广播、受限单播或TLS控制；同index地址/前缀变化会退组重入，多地址签名排序后比较，无变化刷新不推进generation，无接口时清空旧route/组成员。连续socket读错进入有界退避并重建固定发现socket；重建与Close共享生命周期锁，取消后不能安装或遗留新socket，TLS控制独立保留。记忆地址保留最多8条历史并在Manager内有界退避重试，因此对端晚启动或公告过期后仍继续按on-link条件定向探测；不扫描网段。

Windows的 `x/net/ipv4` 不落实发送ControlMessage源选择，本候选改用最多32个按本地IPv4绑定的临时UDP socket，同一socket有界接收回复；`TestWindowsBroadcastUsesBoundSourceWithoutMulticastFlag` 实际观察固定入口收到的源为指定 `127.0.0.2`。Darwin/Linux保留pktinfo/cmsg。LocalSend一手固定提交仍为方案记录的 `25b3019...`；本批没有把其机制误称mDNS，也没有替换LinkSend传输协议。

E5-D 已在荷兰隔离同链路 ULA namespace 完成 IPv6 signed discovery/DeviceID 两地址合并和 B→A 一次 TLS pin heartbeat 的 test-only prototype，也完成 Pion mDNS IPv6 DNS-SD publish/browse。生产 IPv6 discovery 与 mDNS provider 均未启用；mDNS 仅保留未信任候选 hint 的选择，后续接入必须经过 LinkSend 签名 announcement 和 TLS pin 验证。准确包物理多网卡、VPN/TUN、DHCP、睡眠、IPv6与 mDNS 矩阵仍未运行。E5 的固定 UDP 映射双 NAT QUIC 正例不验证发现 provider，也不推广为其他 NAT 类型可用，MASQUERADE-only `CHECK_TIMEOUT` 仍为已知失败。
