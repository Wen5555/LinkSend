# 2026-10-18 准确包原生检查点与取消选择器修复

分项证据，状态仅在[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)维护。物理双机、系统托盘、双NAT与原生loopback分别记录。

## f23383d 来源和 CI

提交 `f23383dc12945df42bc095cd9a2adc8dc011e7c0`，分支 `codex/lan-rebuild-ae`，[Draft PR #9](https://github.com/Wen5555/LinkSend/pull/9)。通过 `gh run list --commit <SHA>` 与 `gh pr view 9` 回读：core push/PR `39081829479/39081832198`、desktop push/PR `39081829495/39081832379`、[packages39081829568](https://github.com/Wen5555/LinkSend/actions/runs/39081829568) 全部success，PR head一致。

实际运行 `scripts/fetch-milestone-artifacts.ps1` 和 `scripts/verify-milestone-packages.ps1`，外层artifact digest、内层资产SHA256、BUILD-INFO/source SHA、COMMITTED/clean、平台和0.5.0版本通过。报告 `.artifacts/ae-20261018/pkg-f23383d/PACKAGE-VERIFICATION.json`。

| 资产 | SHA256 |
|---|---|
| Windows portable | `cb41a771fb97eb105ecc8e13ff6e689dbd846fa1fbaf8aed3e4d3e09189f5e04` |
| Windows installer | `e7a99ed606cba30239cafbca11a2f656c8c3d79bc06f2cabe230832d887bb2ab` |
| Mac arm64 DMG | `43ff8f108d1ee0b25126fd4418ecc7f8bff5407a2c93f8d15c1ee6131b3e34b3` |
| Mac amd64 DMG | `0c67b97ec11edc1cf9c0ff67d0207c6399f129ba88120eaba1dad8b1a40a9f8b` |

Windows运行EXE SHA256 `3a4ca5dd756e244cd08ccd6550f6a2a5c77074cc28c65ef4c06828fdd094d9cf`；Mac arm64运行EXE `11eef533d2ce084376e104ec2c04457aaf4c306004e24127339f04c43a5626e8`。Mac bundle `com.linksend.desktop`、arm64、`codesign --verify --deep --strict`、Local Network usage description通过；未签Developer ID/未公证。

## Windows 原生路径与实际接收

专用profile/接收目录、正常loopback配对fixture，非loopback接口排除。真实 `--send-files -- <专用目录>` 入口填主草稿，原生界面选源码预设/CLI目标并发送。不是物理Windows↔Mac或双NAT。

- N3原生设置保存256KiB/s，偏好revision1→2，实际262144B/s；clipboard=false。34KiB样本不用于速率结论。
- N1预览含1文件/2目录/34.0KB、排除1文件/1目录/27B。task `20261018T003244.908140700Z-00000001`、transfer `8b49751ecdfef9bc22c4c161dc6f420f`，sender completed、bilateral_confirmed=true、verified/committed34816B、retransmitted0、method=direct_unknown。
- 实际 `native-received/source-project/main.txt` 与源SHA256均 `0ffb78a7eb6cb5347c5b0046e776213ebf610b918f17942c4fe1a4bcf505f808`；中文空目录存在、node_modules不存在。receiver同transfer、Completed、selected_files1/entries3。收据 `native-n1-file-receipt.json`。
- 原生目录选择成功；取消后主草稿revision5/路径/peer/规则保持，却显示红色 `RuntimeError: cancelled by user`。取消UX **FAIL**、整项 **PARTIAL**。初始 `native-picker-receipt.json` 的PASS只验证草稿保持；保留原件，以本补充说明整体边界。

Windows UI只用computer-use的 `@oai/sky`。仅文本状态不足以给picker建立坐标几何，激活并取截图/文本后选择成功；HTML select用父窗口Down/Return。早期工具目标保护/黑截图不判产品持续故障。本轮未出现公用网络弹窗、未改防火墙规则。

## 源码修复与验证

核对锁定Wails beta.18 `internal/go-common-file-dialog/cfd/errors.go` 与Windows dialog实现：取消原样返回内部 `errors.New("cancelled by user")`，没有可导入的公共typed sentinel。三个picker只在Windows、精确该消息时返回空选择/nil并丢弃陈旧路径；真实错误/context cancellation/相似及包装消息保持。无wire/schema/前端变更，新准确包UX单列待验。

首轮核心 `TestPairingCodePersistentInboxAndAlwaysAccept` 因运行中的Mac包被正常发现而失败：旧 `len(devices)==2` 与Devices完整列表语义冲突。改为逐identity核对两个成员唯一、trusted/group_paired，额外目标不能trusted；生产逻辑不变。在同一Mac包仍运行时 `go test -race -count=3 -timeout 90s ./internal/app -run '^TestPairingCodePersistentInboxAndAlwaysAccept$'` PASS，20.091s，`picker-live-discovery-race.log`。

桌面workspace test、off-race/vet/build PASS（8.534/9.026s），`picker-verified-desktop.log`。核心 `GOWORK=off go test -count=1 -timeout 6m ./...`、vet/build终态exit0（app93.846s），`picker-verified-core-off.log`。提交后CI/准确包另追加；旧失败 `picker-core-cached.log` 保留。

## owned 资源

Windows按PID156888/准确EXE核对后停止，fixture155472按路径核对并写stop-fixture正常退出；CLI146712此前完成退出。随后回读三个PID均无进程，10473无监听。源、接收文件、profile、证据保留。

Mac按owned-pid/准确路径停止36699，job `/tmp/codex-ssh/job-linksend-ae-stop-native-f23383d-20261018-01` exit0，`OWNED_APP_STOPPED=true CLIPBOARD_MASTER=false RECEIVED_EMPTY=true`。辅助功能关闭、检查返回 `ACCESSIBILITY_DISABLED_NO_INPUT`，未操作Mac UI/启用权限；菜单栏、配对、文件、OS剪贴板仍NOT RUN。
