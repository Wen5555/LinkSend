# M5 最终 CI Mac 包核验

日期：2026-09-12。源码提交 `d0c4a4b13ddc5bd7cd42c8f977d7aa6f7897ae06`，GitHub packaging run `34686655743`。
检查主机为 `wen@10.234.14.15`（alias `mac-test-102342413`），实际系统 macOS 26.5 / ARM64。

两份 CI DMG 均完成来源收据核验、SHA256、`hdiutil verify`、只读挂载、bundle ID / 产品版本 / 架构 / 最低系统和 strict codesign 检查，全部 PASS。
最低版本均为 plist `13.0.0` 和 Mach-O `minos 13.0`；仅 ad-hoc 签名，没有 Developer ID 或公证声明。

| 资产 | DMG SHA256 | 包内 EXE SHA256 | 实际运行范围 |
|---|---|---|---|
| ARM64 | `4138435bf52cb73718acafdba68924e98c23dac794265402e2a399549d8e962c` | `e5760753472bbc9f3ebb9b835f32f79458c22f205f1bb7b9ba13d5550a896628` | 精确 CI payload 真实窗口与正常退出 PASS |
| Intel | `0786c3a1cc4741e040170cbbd6d8bab1936123950a373730483d3dd20f58b942` | `363700d7f17623404e2a8e5226e4e5ed79ea42d12f783797fe87685584d78e1a` | 仅 x86_64 包元数据检查；未运行、未安装或执行 Rosetta |

ARM 原生测试 PID `67390`，窗口 `8028`，大小 `1119 × 759`；观察到 desktop runtime ready，原生 terminate 被接受，退出码 0。
测试使用独立 profile，用户旧应用 PID `47950` 保持运行。测试进程已停止，两份 DMG 均已卸载。

远端作业 `/tmp/codex-ssh/desktop-m5-d0-ci-mac-20260912T100232Z` 退出 0。
本地证据包 `ci-mac-evidence.tar.gz` 与远端 SHA256 相同：`e676021fb33e55ce85d7bfbbbac9996353a5f09db75e1ccfd1489aeb6dd4a65a`。
原始收据、各命令输出及机器可读结果在 `evidence/`，分别见 `CI-MAC-ARM-ACCEPTANCE.json` 和 `CI-MAC-INTEL-METADATA.json`。

此报告补齐精确 CI payload 的包级核验和 ARM 原生启动 / 正常退出证据。macOS 13 实机、Intel 实机及这份 CI 包的完整 M5 控件 / 跨设备操作没有在本子任务执行，不能据此标为 PASS。
该原生检查未修改产品源码；本记录为发布后补录。未改 TCC、防火墙、路由、代理、用户剪贴板或旧应用。
