# LinkSend v0.6.0

发布日期：2026-10-18。产品版本 `0.6.0`，文件协议仍为 V1，`relay=false`。

## 新功能

- **目录预设**：默认完整发送；可选源码目录预设，排除依赖与构建目录，也可保存有限的相对路径排除规则。包含/排除预览与实际发送共用 Go 筛选逻辑，规则随草稿、队列和恢复保存。
- **多设备分发**：一个草稿选择 2–16 台已配对设备，原子创建批次与独立目标队列，顺序发送。可明确等待离线设备、单独取消或确认源变化，也可取消批次的未完成目标。
- **文件正文限速**：设置 → 文件发送中选择不限速或 KiB/MiB 每秒上限；保存后作用于当前与后续文件发送，暂停和取消保持响应。协议开销另计，自动剪贴板使用独立预算。
- **托盘快速发送**：托盘或 macOS 菜单栏 → 快速发送文件，使用独立草稿选择目标和来源，经既有认证队列发送，保留主窗口选择。

## 修复与改进

- LAN 发现固定端口不可用时可降级并恢复；保护 socket generation，合并重复刷新、记忆已认证地址，显示发现 provider 的降级状态。
- 已验证的 Pion host 路径可及时结束候选收集；身份校验、QUIC 加密与文件完整性边界保持。
- 新增“设备 → 服务重建后重新连接”，支持显式建立新组或加入新组，保留本地身份、文件、历史、偏好和有效 LAN 关系，隔离旧组任务及旧授权。
- 修复 Windows picker 取消错误、Mac 设备读取被系统共享存储阻塞，以及快速发送把本机列为可发送目标的问题。
- Mac Finder Services 入口复用主草稿；后台唤回、去重与原生菜单操作的证据分别记录。

## 下载与升级

从 [v0.6.0 Release](https://github.com/Wen5555/LinkSend/releases/tag/v0.6.0) 下载 Windows x64 便携 ZIP/安装程序，或 macOS Apple Silicon/Intel DMG；最低 macOS 13。关闭旧应用后升级，保留原 profile 和接收文件。

Release 提供 `SHA256SUMS.txt` 和 `RELEASE-MANIFEST.json`。程序包由同一主分支提交的 Actions 干净构建，准确提交、平台、版本和摘要以发布清单为准；不重命名或重新标记旧 0.5.0 包。

原信令数据已丢失时，请使用显式恢复入口重新建立设备关系；新组的免确认和剪贴板权限需重新开启。普通连接失败不会自动重置身份或信任。此次发布不部署或替换生产信令服务。

## 已知限制与验证边界

- Windows 包未代码签名；macOS 包为 ad-hoc 签名，未公证。Intel 与最低 macOS 版本的实机范围仍有限。
- 本轮只做版本一致性、构建和产物来源/哈希检查；复用已有功能证据，不扩展重复原生矩阵。Mac 四项功能的原生实际传输证据仍绑定 `6ec3cc4`，旧 Windows 分项绑定 `6edddad`，不转写为 0.6.0 包已完成物理验收。
- 当前准确包的 Windows ↔ Mac 物理双向传输、系统剪贴板、Windows 托盘/系统入口和完整双 NAT 闭环仍保留未验项。Windows 原生执行条件未解除，用户已明确要求保留未验范围；A–E Goal 仍为 blocked。
- 当前没有文件中继或 TURN allocation；UDP 或 NAT 条件不允许直连时，传输可能失败。新日本 STUN 的 UDP3478 补充授权仍待处理，未修改宿主防火墙、路由或代理。

详细证据见[原生检查点](evidence/DESKTOP-AE-NATIVE-CHECKPOINT.md)、[目录预设](evidence/DESKTOP-AE-SOURCE-SELECTION.md)、[多设备分发](evidence/DESKTOP-AE-DEVICE-BATCHES.md)、[限速](evidence/DESKTOP-AE-FILE-BUDGET.md)、[快速发送](evidence/DESKTOP-AE-QUICK-SEND.md)和[唯一 TODO](DESKTOP-EXPERIENCE-TODO.md)。
