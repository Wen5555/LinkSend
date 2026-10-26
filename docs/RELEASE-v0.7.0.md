# LinkSend v0.7.0

本版集中改善跨网络连接、连接恢复与诊断，并提供一次性临时分享。文件仍由两端经认证 QUIC 直接传输，文件协议保持 V1，relay=false。

## 主要变化

- 跨网默认增加备用 STUN；内置公开 STUN 主机在系统 DNS 返回不可用地址时可使用有界 HTTPS 安全解析。修正服务端候选地址误拒绝，保留身份验证和危险地址限制。
- 改善健康会话复用、恢复和剪贴板续租；修复晚到的同策略 lease 吞掉刚发生的文字、链接或图片复制的问题。
- 任务显示实际失败阶段、简短原因和可执行下一步；支持由用户选择位置导出脱敏诊断，不自动上传。
- 一次性分享通过短期分享码领取，发送方确认接收身份，接收方确认内容与保存位置；仅授权原接收方的一份清单和一个任务，不加入永久设备组或开启剪贴板权限。
- 剪贴板文件缓存增加持久所有权、受保护对象判定、确定未发布对象的自动回收，以及普通发送、重发、Open/Reveal 和预览的私有缓存访问保护。Windows 同包 helper 增加准确构建摘要与进程寿命保护。
- 同步 Windows 安装器、应用资源、macOS bundle 和前端的产品版本；保留手动更新与已有退出偏好。

缓存由程序自动管理，没有手动清理按钮、容量或保留期限设置；本版不增加常用发送方案。

## 使用临时分享

两端升级后，在传输页创建或领取临时分享，依次完成双方确认。自托管服务需要支持 `share_grants_v1` 且声明文件传输就绪；管理员须明确启用 `experimental_share_control=true`。旧服务会明确提示不支持。公网服务已从准确 `5ee9a40` 升级并开启该功能，继续保留 schema5 兼容回退程序。

控制库启用分享时会从 schema4 备份迁移到 schema5。升级后的库不能直接交给旧 schema4-only 服务；回退须使用兼容 schema5 的程序并关闭分享，保留当前数据。[生产事务与回退](evidence/RELIABILITY-R5-PRODUCTION-UPGRADE-DRAFT.md)。

## 已有验证与适用范围

- 准确 `36cfce3` 核心完成 Windows↔公网 Linux 普通双向传输及原 Task/TransferID 暂停恢复，核对落盘文件、hash 和双方完成；准确 Windows Wails 候选从 Linux 实收临时分享并正常退出。[普通传输](evidence/RELIABILITY-R1-ORDINARY-CROSSNET-20261026.md) · [原生接收](evidence/RELIABILITY-R5-NATIVE-GUI-20261026.md)。
- 生产准确 `5ee9a40` 经一致性备份、副本迁移/兼容回退及现场后验后上线；同源核心随后通过正常公网 TLS 控制面完成 Windows→Linux 一次 1KiB 临时分享，核对双方任务、清单、hash 和无永久授权残留。[公网分享](evidence/RELIABILITY-R5-PUBLIC-SHARE-5EE-20261026.md)。
- 本版发行包来源、CI、版本与 SHA256 以发行页附带的 `RELEASE-MANIFEST.json` 和 `SHA256SUMS.txt` 为准；上述真实旅程保留各自准确候选来源，不冒充最终包实机复验。

## 保留的限制

- 设备在线表示控制面可达；跨网仍需网络允许 UDP 直连。没有 TURN 或文件中继，不能保证所有公共网络互通。原用户两台 Windows 现场、受限双 NAT、最终候选双向 LAN 与睡眠/网络变化尚未完成验证。
- Windows Explorer / macOS Finder 的实际键盘文件粘贴、已发布缓存读取结束证明和完整自动回收仍未完成。新 Shell publisher 与分页元数据原语未激活；可能已发布对象继续保护，Native 删除保持关闭。不能把本版称为 R2 全部完成。
- 最终包的覆盖安装、完整退出再启动旅程与 Mac 原生运行未实测。Windows 未签名；Mac 为 ad-hoc 签名、未公证。
- 生产原永久数据迁移前后摘要一致，但使用当前默认本机身份的只读普通鉴权探针失败，尚未确认该身份原本的成员状态；不能宣称现有普通用户正向鉴权已实测成功。

R0–R6 整体 Goal 保持未完成，准确状态见[唯一 TODO](DESKTOP-EXPERIENCE-TODO.md)。
