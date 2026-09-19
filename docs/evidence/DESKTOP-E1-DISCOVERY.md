# E1-04 发现证据

`refreshInterfaces` 现保留同一接口的全部有效IPv4地址；route记录远端、接口、本地源、family、network generation和最后出现时间。同一DeviceID多地址合并且每设备最多16条route、全局最多512 peer；replay/响应限流表和TLS握手并发均有界，过期清理会回收状态。

组播加入失败只记录provider降级，不再关闭广播、受限单播或TLS控制；同index地址/前缀变化会退组重入，多地址签名排序后比较，无变化刷新不推进generation，无接口时清空旧route/组成员。连续socket读错进入有界退避并重建固定发现socket；重建与Close共享生命周期锁，取消后不能安装或遗留新socket，TLS控制独立保留。记忆地址保留最多8条历史并在Manager内有界退避重试，因此对端晚启动或公告过期后仍继续按on-link条件定向探测；不扫描网段。

Windows的 `x/net/ipv4` 不落实发送ControlMessage源选择，本候选改用最多32个按本地IPv4绑定的临时UDP socket，同一socket有界接收回复；`TestWindowsBroadcastUsesBoundSourceWithoutMulticastFlag` 实际观察固定入口收到的源为指定 `127.0.0.2`。Darwin/Linux保留pktinfo/cmsg。LocalSend一手固定提交仍为方案记录的 `25b3019...`；本批没有把其机制误称mDNS，也没有替换LinkSend传输协议。

E5-D 已在荷兰隔离同链路 ULA namespace 完成 IPv6 signed discovery/DeviceID 两地址合并和 B→A 一次 TLS pin heartbeat 的 test-only prototype，也完成 Pion mDNS IPv6 DNS-SD publish/browse。生产 IPv6 discovery 与 mDNS provider 均未启用；mDNS 仅保留未信任候选 hint 的选择，后续接入必须经过 LinkSend 签名 announcement 和 TLS pin 验证。准确包物理多网卡、VPN/TUN、DHCP、睡眠、IPv6与 mDNS 矩阵仍未运行。E5 的固定 UDP 映射双 NAT QUIC 正例不验证发现 provider，也不推广为其他 NAT 类型可用，MASQUERADE-only `CHECK_TIMEOUT` 仍为已知失败。
