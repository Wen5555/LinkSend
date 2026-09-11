# LinkSend v0.2.0 test candidate

`0.2.0` 是协议 V1 上的测试候选版本，不是 GitHub Release，也不满足合并/公开发布门槛。候选源码为 `a88553180bd1defac5b236d75fe4dff5046734dd`，PR 为 [#6](https://github.com/Wen5555/LinkSend/pull/6)，三平台构建为 [run 34596127738](https://github.com/Wen5555/LinkSend/actions/runs/34596127738)。相对 `0.1.0`，本轮增加或完成：

- owner/session/generation 约束的立即重连与真实 QUIC 终态可靠性；
- schema 2 任务历史、revision guard、损坏记录隔离和迁移前 v1 备份；
- 显式暂停/恢复、重启恢复、staging 重验、缺块请求与准确重传计数；
- 自动接口发现/优先/排除、link-local 排除、地址失效恢复和脱敏结构化诊断；
- Wails 3 Pause/Resume/Cancel/revision 门控、稳定错误说明和原生退出修复；
- Windows EXE/NSIS 与 macOS plist 的 `0.2.0` 元数据、CLI/服务健康版本输出。

兼容边界：协议 `Version=1`、ALPN `linksend/1`、Pion ICE/quic-go 分工、设备身份、Bundle ID 和正文 QUIC 数据路径不变。SQLite 升级到 `user_version=2`；旧程序不能直接打开 schema 2，回滚需使用迁移前备份。旧客户端可解析 V1 error 帧，但可能把新增稳定码显示为通用失败。

当前阻止发布的已知项：MASQUERADE-only 双 NAT `CHECK_TIMEOUT` FAIL；物理网络切换/睡眠/全局 IPv6 未运行；完整 Windows/macOS 原生交互、安装卸载、Intel Mac、签名和公证未运行。香港测试主站已同步 commit `4b7ccf66…` 的 `0.2.0` 服务，公网完成/拒绝后的立即重试 PASS；备份与回滚方法见 [DEPLOY-HK.md](DEPLOY-HK.md)。

## Committed 候选资产

下表是包内实际文件，不是 GitHub artifact 外层 ZIP；保留期为 workflow 默认 14 天。每个包的 `BUILD-INFO.txt` 记录 `source_state=COMMITTED`、`source_checkout_clean=true`、source/workflow SHA、UTC、Go 1.26.5、Node 22.15.0、pnpm 11.19.0、Wails 3 beta.18 和平台工具。Windows runner 使用 NSIS 3.10；macOS runner 使用 Apple clang 17.0.0。

| 资产 | bytes | SHA256 | 构建 UTC | 签名 / 公证 | 验证边界 |
|---|---:|---|---|---|---|
| `LinkSend-v0.2.0-windows-amd64-a88553180bd1defac5b236d75fe4dff5046734dd.zip` | 8,403,530 | `316a11b74cf146762138384c0cd84ce76b8ec19a48943b1b0abf23f89de37690` | `2026-09-11T11:55:19Z` | unsigned / N/A | 包内 EXE 20,635,648 bytes；FileVersion/ProductVersion 0.2.0；Wails 3；本地原生窗口和 idle WM_CLOSE PASS |
| `LinkSend-v0.2.0-windows-amd64-installer-a88553180bd1defac5b236d75fe4dff5046734dd.exe` | 9,996,125 | `aa346acecd333ea8757bb0ed2f6466de4767af82fdc6640332cd2d0c83311092` | `2026-09-11T11:55:19Z` | unsigned / N/A | FileVersion/ProductVersion 0.2.0；NSIS 3 Unicode，含 `LinkSend.exe`；安装/卸载 NOT_RUN |
| `LinkSend-v0.2.0-macos-arm64-a88553180bd1defac5b236d75fe4dff5046734dd.dmg` | 8,170,931 | `79857ce7ba336e6ceec516b19b737a1386100e9f6dfdf328a44deaeaee142e91` | `2026-09-11T11:54:30Z` | ad-hoc / NOT_RUN | runner 挂载、bundle、arm64、strict codesign PASS；物理 Mac 启动 BLOCKED_BY_EXTERNAL_ENV |
| `LinkSend-v0.2.0-macos-amd64-a88553180bd1defac5b236d75fe4dff5046734dd.dmg` | 8,808,731 | `15c34a786f5aba9b72e7e2e09eaf6502aebf315cfee6161b9918104543de621f` | `2026-09-11T11:57:59Z` | ad-hoc / NOT_RUN | runner 挂载、bundle、x86_64、strict codesign PASS；Intel 真机 NOT_RUN |

`9cce3ee` 包曾完成三平台构建，但同 SHA 的 push core run `34592549314` 真实暴露 code-0 QUIC 终态竞态，因此已被本表资产取代；不能选择性引用同 SHA 的绿色 PR run 将其恢复为最终候选。更早 dirty r2 快照同样不属于本候选资产。

所有资产必须从真实 commit 或对应 workflow head 构建，文件名包含 `v0.2.0`、平台、架构和 commit。候选尚未首次发布，同轮可靠性修复不单独虚增为 `0.2.1`；正式发布后的兼容修复再提升 patch，兼容功能提升 minor，破坏性协议变化需同时评审产品 major 与协议版本。
