# E1-05 统一目录与恢复证据

## 2026-09-22 事实状态与 LAN 入队修补

后续源码核对推翻了早期“非终态 task 的 SessionID 等于当前连接”的判断：SessionID 在发送 connect_request 之前就持久化，暂停/恢复任务也保留历史值。本批改为进程内观察所有三条建连成功路径的真实 QUIC Conn；只有完成身份验证、授权代次仍匹配且 Conn.Context 尚未关闭才显示 connected。覆盖不进复用池的旧端/直接调用、同时发起的 responder、无活动任务的空闲池；blocked 优先，关闭回调仅清理内存，不改变连接所有权。无 wire/schema 变更。

设备目录和发送选择器区分“已发现 · 连接待验证”“在线 · 直连待检查”“未见在线记录”“连接状态待检查”和“已连接”。签名公告不再显示“局域网可达”，服务未知不再自动标离线；服务器在线列表同步与可传文件仍分别判断。已移除关系不会被旧 nearby/online/connected 覆盖。

真实 LAN 同意后，原 Enqueue 仍只读 Online、误报 PEER_OFFLINE，已复现并修复。入队与调度统一接受服务 presence、当前 LAN 路径或仍存活的认证连接作为尝试条件，保留本地 grant/代次检查与后续握手。发现未配对仍拒绝；HTTP 服务关闭但 QUIC 存活时可入队，QUIC 关闭后恢复显式等待要求。入队通过不承诺每次后续路径都成功。

定向 RED/GREEN、race 与完整集成命令见本批 E5-W；原始日志 `.artifacts/astra-resume/device-state/`。这轮没有运行准确桌面包、Mac、物理 LAN/NAT 或系统网络切换，旧包/旧实机结果不迁移。

## 原 E1 实施记录

`DeviceInfo` 按DeviceID合并服务成员、持久grant、LAN公告和拒绝屏障，但分别暴露relationship、service_state、lan_control_state、connection_state、online与nearby；LAN公告不再把服务online强行置true。被删除设备显示本机拒绝及pending sync，不进入可信设备状态。

队列可调度条件分别接受已认证服务presence或可信LAN附近路径，不再要求LAN-only设备伪装为online。队列保存的授权generation通过StartSend、建连后的PeerSession和开流前检查一直传递，期间删除/重配不能把旧队列重新绑定到新grant；接收Session也在认证建立时固定generation。

新增 `NetworkChanged(network|sleep|wake|interface|address|route)`：消抖后清除未来endpoint选择缓存并刷新发现provider；LAN runtime每5秒比较真实接口地址快照并调用该入口，覆盖DHCP、网卡、VPN/TUN和唤醒后快照变化。刷新不取消当前活动task，现有QUIC继续独立于WSS和发现。原设备目录从非终态task的session ID合成 `connected`，该推断存在误报，已由上节真实连接观察替代。路径真正失败仍沿用已有新attempt/session/ICE generation及块恢复逻辑。

Windows/macOS原生网络与睡眠事件已接入同一恢复门闩，5秒快照继续作为兜底；实现与编译证据见 [E1-C3原生事件](DESKTOP-E1-NATIVE-EVENTS.md)。准确Windows↔Mac包的双向文件hash、事件到达延迟、网络切换、睡眠唤醒和双NAT均NOT RUN，不能据此把E1-05网络/原生列标PASS。
