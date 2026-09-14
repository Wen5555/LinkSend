# E0-01 准确 M5 包与复现基线

2026-09-13。状态：PARTIAL，待总控审查；尚未完成准确安装包的跨平台完整矩阵。
当前源码基线 3bcb73019d1ac6de1d9f341bb7d22e2813875081；M5 发行源码 d0c4a4b13ddc5bd7cd42c8f977d7aa6f7897ae06。
两者仅文档差异，不能把源码测试与安装包验收互换。

## 包来源重验

`gh api repos/Wen5555/LinkSend/releases/tags/v0.5.0-m5` 退出0。对原 m5-ci/RELEASE-MANIFEST.json 指向的四资产实际 Get-FileHash，均与 GitHub 当前 digest/大小匹配。
原始收据在 ignored `.artifacts/desktop-experience/e0/m5-{release-current.json,package-rehash.json}`。

| 包 | 大小 | SHA256 |
|---|---:|---|
| Windows ZIP | 9628352 | dfc10e70d924b4cac302c844d862413e31e2826aef41e1543ad47f4559500cbe |
| Windows NSIS | 11194837 | cc1d5aad9adbd93131f385c41d326b794aca525f719eab1afb95df0b3f0d63f5 |
| Mac ARM DMG | 9171576 | 4138435bf52cb73718acafdba68924e98c23dac794265402e2a399549d8e962c |
| Mac Intel DMG | 9888056 | 0786c3a1cc4741e040170cbbd6d8bab1936123950a373730483d3dd20f58b942 |

Windows 真窗口使用准确 Release ZIP 解出的 EXE，SHA256 `e08d8dce9cda57edb3fb8b55743d3b8e3928919841ff9a2bd14582c5087b67ff`；不是当前源码临时构建。
本轮尚未在 NSIS 安装身份中复测，ZIP payload 窗口不算 NSIS 安装通过。
当前 Mac 已安装 /Applications/LinkSend.app 的 EXE 实测 `bb709954e4f2a4c558ea99f0201038e0d63fa47de17dddf00f80452875951992`，与准确 CI ARM EXE `e5760753472bbc9f3ebb9b835f32f79458c22f205f1bb7b9ba13d5550a896628` 不同，不能称当前已安装准确 M5 包。

## 七项问题复现矩阵

实际 Windows 11 Pro x64 10.0.26200，PowerShell7.6.5；独立 profile、loopback 信令/真实身份与 Pion ICE+QUIC fixture，不接触用户剪贴板。
原生 session `scripts/desktop-native/m1-native-tests/run-20260912T181423895Z/`；GUI PID260116/ HWND43979262，启动命令退出0；中断恢复后该 PID已不存在，未据此声称正常退出0。

| U | 源码/历史观察 | 本轮准确 Windows payload | Mac/安装/网络边界 |
|---|---|---|---|
| U1 | ReceivePlanDialog 手动预览后确认 | 真实4项/97B offer；确认按钮 enabled=false，offscreen=true，坐标1331,1130,158,48；界面要求预览；真实拒绝后 peer exit1 RECEIVE_REJECTED | Mac原生与NSIS NOT RUN |
| U2 | Admin撤销/旧revoked不能重配、Nearby OR Online | 设备详情只有“屏蔽并取消信任”，没有删除；配对码表单存在 | 服务重配/跨组负例尚未本轮运行 |
| U3 | 单地址与通道耦合风险，见方案/ADR | isolated fixture只能证明本机路径 | 物理发现→配对→控制→双向hash NOT RUN |
| U4 | 旧手动内容仍实现，无自动同步 | 真实传输页包含文字/链接/图片编辑器与保存草稿 | 不触碰日常剪贴板；自动同步 NOT IMPLEMENTED |
| U5 | 多组件整表保存 | 设置页顺序含内容快照、后台、偏好、系统入口、网络、接收、诊断，仍有“保存设置” | 新revision并发契约 NOT IMPLEMENTED |
| U6 | SendTo/Services合并主草稿 | 真实页面仅安装Windows发送到等兼容入口 | 真共享原型另表，不以此判成功 |
| U7 | 长页堆叠 | 传输页依次含准备发送、接收、旧内容、队列、任务；接收确认按钮原生offscreen | 完整1100×720/960×640/125%/150%/深色/键盘矩阵 NOT RUN |

原始 UIA 文本/树在 `.artifacts/desktop-experience/e0/m5-*-native.txt`、`m5-incoming-before-preview.{json,txt}`。
`go build -o scripts/desktop-native/m1-native-tests/fixture.exe ./scripts/desktop-native/m1-native-tests/fixture` 与 m4-native-peer 构建退出0；这些对端来自新源码基线，GUI来自精确M5包，来源不混淆。

## 自动检查

Go1.27.1、Node24.21.0、pnpm12.4.1、Wails3 beta.18（实际 version 命令退出0）。
执行任务 desktop `GOWORK=off go test ./...` 退出0：desktop1.228s、nativeclipboard0.493s；独立vet/build随后也退出0；日志 e0/desktop-independent-{test,vet,build}.log。
总控 E0-core-baseline-review-v1：原目录固定3bcb730下 `go test ./...`、`GOWORK=off go test ./...`、`GOWORK=off go vet ./...` 全部退出0；未加-count=1，缓存情况以原始日志为准。
总控日志在 `C:/Users/Wen/.codex/supervision/linksend-desktop-experience/e0-core-{workspace-test,independent-test,independent-vet}.log`。这不替代未来候选或物理链路。
新增control-inspect的独立go test（no test files，编译检查）/vet与真实HTTPS运行退出0，不宣称新增了自动用例。
全量race/前端/原生安装矩阵本轮 NOT RUN，产品源码尚未改动。
