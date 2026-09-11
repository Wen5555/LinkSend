# LinkSend v0.2.0 test prerelease

`v0.2.0` 是协议 V1 上的 [GitHub 测试预发布](https://github.com/Wen5555/LinkSend/releases/tag/v0.2.0)，不是稳定版或生产可用声明。tag/源码为 `426d58b6ab62ab7213475007305c0a403955c00f`，已合并 [PR #6](https://github.com/Wen5555/LinkSend/pull/6)，Release 三平台构建来自 [main run 34600609161](https://github.com/Wen5555/LinkSend/actions/runs/34600609161)。相对 `0.1.0`，本轮增加或完成：

- owner/session/generation 约束的立即重连与真实 QUIC 终态可靠性；
- schema 2 任务历史、revision guard、损坏记录隔离和迁移前 v1 备份；
- 显式暂停/恢复、重启恢复、staging 重验、缺块请求与准确重传计数；
- 自动接口发现/优先/排除、link-local 排除、地址失效恢复和脱敏结构化诊断；
- Wails 3 Pause/Resume/Cancel/revision 门控、稳定错误说明和原生退出修复；
- Windows EXE/NSIS 与 macOS plist 的 `0.2.0` 元数据、CLI/服务健康版本输出。

兼容边界：协议 `Version=1`、ALPN `linksend/1`、Pion ICE/quic-go 分工、设备身份、Bundle ID 和正文 QUIC 数据路径不变。SQLite 升级到 `user_version=2`；旧程序不能直接打开 schema 2，回滚需使用迁移前备份。旧客户端可解析 V1 error 帧，但可能把新增稳定码显示为通用失败。

发布时仍存在的已知项：MASQUERADE-only 双 NAT `CHECK_TIMEOUT` FAIL；物理网络切换/睡眠/全局 IPv6 未运行；完整 Windows/macOS 原生交互、安装卸载、Intel Mac、签名和公证未运行。用户在知悉这些边界后明确要求合并和发布，因此本版仅标为 Pre-release 且不是 Latest。香港测试主站已同步 commit `4b7ccf66…` 的 `0.2.0` 服务，公网完成/拒绝后的立即重试 PASS；备份与回滚方法见 [DEPLOY-HK.md](DEPLOY-HK.md)。

## Release 资产

下表是 GitHub Release 实际文件，不是 Actions artifact 外层 ZIP。三个 workflow artifact 的 `BUILD-INFO.txt` 记录 `source_state=COMMITTED`、`source_checkout_clean=true`、source/workflow SHA、UTC、Go 1.26.5、Node 22.15.0、pnpm 11.19.0、Wails 3 beta.18 和平台工具；Windows runner 使用 NSIS 3.10，macOS runner 使用 Apple clang 17.0.0。Release 另附合并 SHA256 清单和构建 manifest。

| 资产 | bytes | SHA256 | 构建 UTC | 签名 / 公证 | 验证边界 |
|---|---:|---|---|---|---|
| `LinkSend-v0.2.0-windows-amd64-426d58b6ab62ab7213475007305c0a403955c00f.zip` | 8,403,529 | `dcdd7127c527b1464f0ea025046ad871a35ed6eaba4a2c725884779071d81577` | `2026-09-11T12:49:31Z` | unsigned / N/A | 包内 EXE 20,635,648 bytes；FileVersion/ProductVersion 0.2.0；Wails 3；Release EXE 原生窗口和 idle WM_CLOSE PASS |
| `LinkSend-v0.2.0-windows-amd64-installer-426d58b6ab62ab7213475007305c0a403955c00f.exe` | 9,996,124 | `99a4d5d25dbc9761e4839ff435f362ec65aa75fcdafdf46f285f05defdde7370` | `2026-09-11T12:49:31Z` | unsigned / N/A | FileVersion/ProductVersion 0.2.0；NSIS 3 Unicode，含 `LinkSend.exe`；安装/卸载 NOT_RUN |
| `LinkSend-v0.2.0-macos-arm64-426d58b6ab62ab7213475007305c0a403955c00f.dmg` | 8,170,928 | `c637897290677c7deb8c50395b95568f55a93a14bdf51e2babbf7eaf974fdf4c` | `2026-09-11T12:48:16Z` | ad-hoc / NOT_RUN | runner 挂载、bundle、arm64、strict codesign PASS；Release 包物理 Mac 启动 NOT_RUN |
| `LinkSend-v0.2.0-macos-amd64-426d58b6ab62ab7213475007305c0a403955c00f.dmg` | 8,808,743 | `f5f066634429e61312066c056b836482ca228315ea858cab3de28653e67120fe` | `2026-09-11T12:51:36Z` | ad-hoc / NOT_RUN | runner 挂载、bundle、x86_64、strict codesign PASS；Intel 真机 NOT_RUN |

`9cce3ee` 包曾完成三平台构建，但同 SHA 的 push core run `34592549314` 真实暴露 code-0 QUIC 终态竞态，因此没有进入 Release；不能选择性引用同 SHA 的绿色 PR run 恢复它。后续 `a885531` committed 候选资产完成修复验证，但 Release 最终只上传从合并/tag SHA `426d58b` 的 main run 重建资产。更早 dirty r2 快照同样不属于 Release。

所有资产必须从真实 commit 或对应 workflow head 构建，文件名包含 `v0.2.0`、平台、架构和 commit。`v0.2.0` 现已发布，后续兼容修复应提升到 `0.2.1`，兼容功能提升 minor，破坏性协议变化需同时评审产品 major 与协议版本；不得覆盖现有 tag 资产或重写来源元数据。
