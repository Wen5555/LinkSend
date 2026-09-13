# E1-04 发现证据

`refreshInterfaces` 现保留同一接口的全部有效IPv4地址；route记录远端、接口、本地源、family、network generation和最后出现时间。同一DeviceID多地址合并且每设备最多16条route、全局最多512 peer；replay/响应限流表和TLS握手并发均有界，过期清理会回收状态。

组播加入失败只记录provider降级，不再关闭广播、受限单播或TLS控制；无接口时清空旧route/组成员并推进generation。连续socket读错进入有界退避。记忆地址保留最多8条历史，启动逐个执行on-link校验和定向探测，不扫描网段。

Windows的 `x/net/ipv4` 不落实发送ControlMessage源选择，本候选改用最多32个按本地IPv4绑定的临时UDP socket，同一socket有界接收回复；`TestWindowsBroadcastUsesBoundSourceWithoutMulticastFlag` 实际观察固定入口收到的源为指定 `127.0.0.2`。Darwin/Linux保留pktinfo/cmsg。LocalSend一手固定提交仍为方案记录的 `25b3019...`；本批没有把其机制误称mDNS，也没有替换LinkSend传输协议。

未完成：准确包物理多网卡、VPN/TUN、DHCP、睡眠、IPv6和mDNS矩阵；双NAT仍未证明。
