# R5 显式能力与客户端入口门

日期：2026-10-26。本增量开放已有实现的能力协商，不改文件数据协议、签名用途域、永久成员、用户默认权限或 relay；没有生产部署、配置/数据库写入、SSH 或 GUI 操作。R5 整体与原 Windows 双端问题不因本增量打勾。

## 依据与行为

准确 `701e23b` 的 Windows↔公共 Linux 临时分享核心双向文件/hash/双方 Completion，以及准确 `36cfce3` Wails 从 Linux 实收、原生选择目录、明确保存、逐路径 hash 和正常退出已有[实网](RELIABILITY-R5-CROSS-NETWORK-20261026.md)与[原生](RELIABILITY-R5-NATIVE-GUI-20261026.md)证据。本轮复用这些真实数据面结果，不重跑文件旅程；它们不代表 Windows↔Windows、双 NAT、Mac 专属或反向原生发送通过。

此前 server health 硬编码 `file_transfer_ready=false`，能力结构没有 `share_grants_v1`，客户端却没有用 readiness 阻止控制请求。当前保留历史配置键 `experimental_share_control`，不增加新键或修改生产值。只有明确为 true 且 Server.New 已成功初始化迁移、分享 epoch 与 scoped hub 的实例，才在 health 和普通 WS hello/authenticated 的能力中声明 `share_grants_v1=true`，并在 health 提供 `share_control.version=1`、当前 epoch、`file_transfer_ready=true`。这是该服务可协调已有临时 P2P 功能的声明，不承诺任何 NAT 都可直连。普通配置仍省略该能力与 share_control，路由明确 `SHARE_UNSUPPORTED`。

签名客户端每操作重新验证 HTTPS health、基本 V1/membership2/quic 能力、独立分享版本/当前 epoch、规范 `share_grants_v1=true` 和 ready=true。旧端、旧实验控制端（缺 bit 或 ready=false）明确 `SHARE_UNSUPPORTED`，不发送分享码、状态/副作用请求或开启 scoped WS；不会回退为永久配对。重复/歧义字段、缺失 readiness、错误类型或控制对象别名仍拒绝。普通未知的扩展 health/capability 元数据保持兼容。

Go 的 Create/Claim 在源准备、生成码或保存本地 intent 前先过该门；真正执行每个意图仍再次取新 epoch、签名并核验服务权威响应，不缓存能力或自动重试。Claim 的新增 health 也受 profile 退出取消。Start/Resume、终态错误、原 task/grant/plan 与明确确认门没有改写；数据连接仍走当前签名 scope、固定公钥、新 ICE/QUIC 和原 V1 引擎。能力声明不授予枚举组、普通 WS、自动接收或剪贴板权限。

## 一次定向验证

- 根模块 `GOWORK=off go test ./internal/server ./internal/signaling ./internal/app -run <精确 selection> -count=1 -v`：exit0，15 个顶层用例；三个包实际 3.669s / 4.386s / 5.675s。精确 selection 和日志见 `.artifacts/r5-capability-ready/VERIFY-RECEIPT.json`、`targeted-tests.log`。
- 包括显式/普通实例广告、旧控制与缺能力/false readiness/别名拒绝；在 secret、状态 POST 或 WS 前阻断；Create/Claim 未 prepare、未返回码、未保存分享/任务。复用现有签名生命周期、第三身份/永久成员接口隔离、epoch 重启、严格响应、取消，以及批准/撤销响应丢失的幂等用例。
- 桌面独立模块 `GOWORK=off go test . -run '^TestDesktopTemporaryShareUsesSavedDraftAndRejectsStaleRevisionBeforeRequest$' -count=1 -v`：exit0 / 6.058s；保存草稿与旧 revision 前置拒绝、码不进入 workspace/诊断的既有入口通过。没有原生 clipboard/GUI 操作。

没有扩大全矩阵、race、文件传输或原生场景。最终提交的准确包/CI 仍由该提交的新结果确认，不把 36 的数据面证据改记为新二进制实测。

## 生产边界

生产现有记录只读显示 `https://linksend.oooai.de` 仍为 0.5.0、无分享能力；本轮没有重新 SSH 或变更它。开启分享会进行 Control4→5 的已实现一致性备份迁移，任务库仍 schema12。旧 schema4-only 二进制不能作为迁移后的默认回退。目标、最终准确构建前置条件、私有备份、副本迁移验证及兼容回退见[待授权草案](RELIABILITY-R5-PRODUCTION-UPGRADE-DRAFT.md)；当前仅准备，不能当成部署或生产分享可用证据。
