# E1-05 统一目录与恢复证据

`DeviceInfo` 按DeviceID合并服务成员、持久grant、LAN公告和拒绝屏障，但分别暴露relationship、service_state、lan_control_state、connection_state、online与nearby；LAN公告不再把服务online强行置true。被删除设备显示本机拒绝及pending sync，不进入可信设备状态。

新增 `NetworkChanged(network|wake|interface|address|route)`：消抖后清除未来endpoint选择缓存并刷新发现provider；不取消当前活动task，现有QUIC继续独立于WSS和发现刷新。路径真正失败仍沿用已有新attempt/session/ICE generation及块恢复逻辑。`TestNetworkChangeInvalidatesFutureSelectionWithoutCancellingActiveTask` 覆盖该边界。

自动化仅证明后端状态/恢复门闩；准确Windows↔Mac包的双向文件hash、网络切换、睡眠唤醒和双NAT均NOT RUN，不能据此把E1-05网络/原生列标PASS。
