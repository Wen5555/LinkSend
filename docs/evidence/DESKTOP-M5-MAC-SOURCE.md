# M5 精确 d0c4a4b Mac 源码检查

日期：2026-09-12。状态：本报告所列检查全部 PASS，未修改或提交新的产品源码。

- 源码：`d0c4a4b13ddc5bd7cd42c8f977d7aa6f7897ae06` 的 `git archive`。
- 归档 SHA256：`25f870c63d1d0bae9a15c10508b7628e8da9cfe2bb0cadfc98bea621096cb081`。
- 主机：`wen@10.234.14.15`，SSH manager alias `mac-test-102342413`。
- 实际系统：macOS 26.5 / ARM64；Go 1.27.1、Node 24.21.0、pnpm 12.4.1、Wails 3 beta.18。
- 隔离目录：`/Users/wen/linksend-desktop-six/m5-d0c4a4b`。

| 检查 | 结果 |
|---|---|
| 根模块 `GOWORK=off go mod verify`、`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`go build ./...` | 全部退出 0 |
| desktop 独立模块同组 verify / test / race / vet / build | 全部退出 0 |
| Wails bindings 重新生成 | 退出 0 |
| 独立离线 frozen install、前端类型 / lint / 测试 / build | 全部退出 0，58 项测试通过 |
| `wails3 task darwin:package ARCH=arm64` | 退出 0，真实 ARM64 `.app` |
| codesign strict 验证 | PASS，仅 ad-hoc 签名 |
| 最低系统元数据 | plist `13.0.0`，Mach-O `minos 13.0` |
| 真实窗口与 runtime ready | PID 67210，窗口 8020，1119 × 759 |
| 原生正常退出 | terminate 被接受，应用退出码 0 |
| 旧应用保护 | PID 47950 原路径保持运行，测试进程已停止 |

源码检查作业：`/tmp/codex-ssh/desktop-m5-d0-source-check-20260912T094930Z`，退出 0。
原生启动作业：`/tmp/codex-ssh/desktop-m5-d0-native-smoke-20260912T095235Z`，退出 0。

源码构建 EXE SHA256：`e69c658e67c8d9f8e5f66fc7f7a214268658a6574dc756f8db717fb4ebedb7c9`。
下载证据包 SHA256：`938908a678077bb8507a727a18a15c916068bf343dcbacf4759ce85af7a1dbb5`，与远端相同。

`m5-source-evidence/acceptance-summary.json` 是机器可读结果，命令和完整日志位于同目录。
源码忠实性比对只发现 Wails 重新生成 `icons.icns` 和 `icon.ico`；Go 源码、模块文件、React 源码、绑定和 dist 与归档一致。
desktop 链接保留上游重复 `-lobjc` 警告，未造成测试或构建失败。

边界：这是精确源码归档的原生构建及真实窗口 / 正常退出证据，不替代最终 CI 包验证。
macOS 13 实机、Intel 实机、这份源码包的完整 M5 原生内容动作及最终 CI payload 均未在本子任务测试。
没有更改 TCC、主机防火墙、路由、代理、用户剪贴板或旧应用；测试 profile 不包含在导出的证据包中。
