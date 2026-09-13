# E1-05 统一目录与恢复证据

`DeviceInfo` 按DeviceID合并服务成员、持久grant、LAN公告和拒绝屏障，但分别暴露relationship、service_state、lan_control_state、connection_state、online与nearby；LAN公告不再把服务online强行置true。被删除设备显示本机拒绝及pending sync，不进入可信设备状态。

新增 `NetworkChanged(network|wake|interface|address|route)`：消抖后清除未来endpoint选择缓存并刷新发现provider；LAN runtime每5秒比较真实接口地址快照并调用该入口，覆盖DHCP、网卡、VPN/TUN和唤醒后快照变化。刷新不取消当前活动task，现有QUIC继续独立于WSS和发现。设备目录从非终态task的真实session ID合成 `connected`，不再始终硬编码未连接。路径真正失败仍沿用已有新attempt/session/ICE generation及块恢复逻辑。

自动化仅证明后端状态/恢复门闩；Windows/macOS原生网络与睡眠事件的低延迟回调尚未接线，当前依靠5秒快照兜底。准确Windows↔Mac包的双向文件hash、网络切换、睡眠唤醒和双NAT均NOT RUN，不能据此把E1-05网络/原生列标PASS。
