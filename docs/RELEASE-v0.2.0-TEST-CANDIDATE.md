# LinkSend v0.2.0 test candidate

`0.2.0` 是协议 V1 上的测试候选版本，不是 GitHub Release，也不满足合并/公开发布门槛。相对 `0.1.0`，本轮增加或完成：

- owner/session/generation 约束的立即重连与真实 QUIC 终态可靠性；
- schema 2 任务历史、revision guard、损坏记录隔离和迁移前 v1 备份；
- 显式暂停/恢复、重启恢复、staging 重验、缺块请求与准确重传计数；
- 自动接口发现/优先/排除、link-local 排除、地址失效恢复和脱敏结构化诊断；
- Wails 3 Pause/Resume/Cancel/revision 门控、稳定错误说明和原生退出修复；
- Windows EXE/NSIS 与 macOS plist 的 `0.2.0` 元数据、CLI/服务健康版本输出。

兼容边界：协议 `Version=1`、ALPN `linksend/1`、Pion ICE/quic-go 分工、设备身份、Bundle ID 和正文 QUIC 数据路径不变。SQLite 升级到 `user_version=2`；旧程序不能直接打开 schema 2，回滚需使用迁移前备份。旧客户端可解析 V1 error 帧，但可能把新增稳定码显示为通用失败。

当前阻止发布的已知项：MASQUERADE-only 双 NAT `CHECK_TIMEOUT` FAIL；物理网络切换/睡眠/全局 IPv6未运行；完整 Windows/macOS 原生交互、安装卸载、Intel Mac、签名和公证未运行。香港测试主站已同步 commit `4b7ccf66…` 的 `0.2.0` 服务，公网完成/拒绝后的立即重试 PASS；备份与回滚方法见 [DEPLOY-HK.md](DEPLOY-HK.md)。

所有资产必须从真实 commit 或对应 workflow head 构建，文件名包含 `v0.2.0`、平台、架构和 commit；旧 dirty r2 快照不属于本候选发布资产。
