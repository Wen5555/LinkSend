# LinkSend implementation progress

## 2026-09-12 六项桌面能力（分阶段执行中）

基线 `fbfc250`，工作分支 `codex/desktop-six-features`；用户明确授权逐目标推送、部署和发布。
依赖官方核实与安装完成：Go1.27.1、Node24.21.0、pnpm12.4.1、React19.3、TS6.0.3、Vite8.3、Vitest5、Query5.102.8，Wails保留beta.18。
已实现本机拒绝与trust schema1迁移、profile内核锁、保存退出及调度/迟到事件屏障；真实QUIC保存退出与针对性race通过。
完整命令/失败修复/各阶段状态见 [六项执行记录](evidence/DESKTOP-SIX-FEATURES-EXECUTION.md)。
Mac地址按用户最新信息更新为10.234.184.6，SSH已通过，隔离工具链安装已完成。

用户后续明确最低macOS13、取消12支持，并允许M0/M1合并发布；M2/M3平台层和M4接收协议在隔离分支并行推进。
M1已增加task schema3、设备偏好、持久草稿、幂等队列、离线旁路、源变化确认及200ms合并的epoch/revision事件；
前端已拆分且使用Query/真实绑定，29项测试通过。根完整普通/race及双模块检查通过，原生UI与包级验证进行中。
具体实现与真实QUIC负例见 [M1执行记录](evidence/DESKTOP-M1-EXECUTION.md)。

M0/M1 精确提交 `2f2656d` 的三平台 CI、四个包哈希/来源校验及 Windows CI payload 原生六项操作 PASS，
香港 M1 事务部署与独立复核 PASS；[部署和备份](evidence/DESKTOP-M1-DEPLOYMENT.md)。
M2 已接入单实例、有界持久入口日志、窗口拖放、SendTo/Finder provider、数量大小预览；
Windows 同时入口/相对路径/隐藏/双 profile/冷启动/重启原生检查 PASS，完整菜单/拖放验收继续。
M0/M1 已正式发布为 [v0.5.0-m1 测试预发布](https://github.com/Wen5555/LinkSend/releases/tag/v0.5.0-m1)，
包含精确 CI Windows/两种 Mac 包，非 Latest；Mac ARM 原生启动/退出也已通过。
发布后深测发现 M1 Windows 自定义退出按钮兼容问题，Release 已追加已知问题；当前 M3 源码改用原生 TaskDialog。
M3 已接托盘/后台/通知/防睡眠，Windows 原生隐藏唤回、实际4MiB接收中的继续和保存退出均 PASS。
还修正后台偏好误取消接收、shutdown 状态发布早于实际取消及 M2 CI 测试上下文问题。
具体失败、修复和哈希见 [M3 执行记录](evidence/DESKTOP-M3-EXECUTION.md)。
M4 接收应用层、M5 完整内容链路及 M6 收件箱 UI 继续在隔离分支并行，按依赖串行接入。

M4 传输与 App 后端已接入不可变清单、分页选收、冲突策略、目录/计划持久化、实际空间预检、NoContent、发送选择摘要恢复绑定以及M6完整索引登记。真实QUIC同时修复SQLite并发写锁和接受后error/body帧歧义，并验证活跃接收时改全局目录不会取消任务；完整根普通/race、双模块检查通过。UI/物理双机原生验收与里程碑发布另行记录，不因后端测试而标为完成；见 [M4 App证据](evidence/DESKTOP-M4-APP.md)。

M4/M5 wire整合保留三个可选能力；发送端选择和实际内容解释的持久化回调都成功后才发accepted，并同时验证manifest/selection/content摘要。联合真实QUIC及旧端兼容验证见 [整合证据](evidence/DESKTOP-M4-M5-ACCEPTANCE-INTEGRATION.md)。

Updated: 2026-09-12. Current source: `0.5.0`; published milestone: `v0.5.0-m1`; protocol: V1.
The following `v0.4.0` sections preserve earlier evidence. No prerelease is an accepted production release.

## 2026-09-12 v0.4.0 文档、主分支与测试预发布

- 产品版本从 `0.3.0` 提升到 `0.4.0`，协议仍为 V1。该 minor 版本汇总配对幂等与错误语义、安全 LAN 发现、WSS 读泵复用、trickle ICE、响应端 QUIC listener 预注册、显式双方完成确认、DHCP/EFS 修复和接收热路径优化。
- 公开 README 按使用者视角说明功能、数据路径、快速开始和边界；架构、协议、安全、性能、测试、验收、路线图、部署及平台测试文档同步到 `0.4.0`。旧版本发布记录和当时的失败/未运行结论继续作为历史事实保留。
- Release 只使用 GitHub Actions 从 tag 对应干净提交生成的 Windows amd64 ZIP、Windows NSIS 安装程序与 macOS arm64/amd64 DMG；本地 dirty snapshot 和实机临时包不作为 Release 资产。Windows 未代码签名，macOS 仅 ad-hoc 签名且未公证。
- 仍保留真实限制：无 Relay/TURN 文件中继；MASQUERADE-only 双 NAT 曾返回 `CHECK_TIMEOUT`；公共 IPv6、更多 NAT 类型、网络切换/睡眠唤醒、完整安装卸载与全量原生交互没有因本次发布而自动变为 PASS。
- 实现与第一版公开文档已直接合入并推送 `main`，提交 `7303f34260c09b8ad71c117ee2d6837fac4dfe46`；推送前 `origin/main` 与本地没有分叉。提交前 Windows 根普通/race/vet/`GOWORK=off`、桌面独立 verify/test/vet/build、前端 typecheck/lint/11 tests/build、真实 loopback ICE/TLS 1.3/QUIC 摘要校验和 Wails/NSIS `0.4.0` 版本资源检查全部 PASS。
- 香港测试主站从该提交的独立干净 clone 构建并事务部署：`vcs.modified=false`，服务端 SHA256 `227e2ec9fb029fcc8a568637376318e4eae4099f1cc2ce79bc7224557f07fa45`，最终 systemd MainPID `393922`，schema 2、数据库完整性、443、公网 health、产品/协议版本、QUIC、`relay=false` 与 recent fatal=0 均 PASS。公网配对再次确认 TTL=600 秒、同身份幂等与其他身份 `PAIRING_CODE_USED`。
- 部署审计同时修复了即将到期的 Origin 证书续期链路：移除缺失的 `jq` 依赖、规避 Cloudflare HTTP/2 `PROTOCOL_ERROR`、将 rendezvous 迁移为独立 systemd service，并让续期任务只执行证书事务和 service restart。真实续期、定时器和独立复核 PASS，失败单元为 0；备份与回滚见 `DEPLOY-HK.md`。
- 最终文档提交 `e17e0cb793a2f69162394d3d53e5cda8b13912fa` 的首轮 GitHub core run `34667737461` 真实 FAIL：Linux runner 分别出现 simultaneous session 首个 QUIC payload 为空和 persistent inbox 未创建确认任务；desktop/package job 继续独立运行，但该提交不允许打 tag。两项在 Windows 各重复 100/20 次未复现，失败共同发生于 trickle 收敛后的首个数据流附近。
- 根因是 Pion 可能在 `Connect` 返回后报告初始化阶段最后一次 selected-pair 优化；旧代码已立即武装运行期路径监控，会把这次初始化尾声误判成网络迁移并关闭刚建立的 QUIC。endpoint 现在使用 1 秒初始稳定窗口：窗口内只更新 pair 基准并重新计时，不增加建连等待；窗口后不同 pair 仍触发关闭与显式恢复。新增纯状态回归测试后 simultaneous QUIC 重复 100 次、persistent inbox 重复 50 次、完整普通/race/vet/`GOWORK=off` 再次 PASS。

## 2026-09-12 配对幂等、LAN 直接发现与接收热路径优化（v0.4.0）

- 配对服务将原先统一的“invalid/expired/used”拆分为稳定错误码，控制库迁移到 schema 2，增加 `used_by/used_at`；同一身份在成功响应丢失或前端重复点击后可幂等取得既有成员，其他身份仍被拒绝。邀请响应增加服务端相对 TTL，前端显示真实倒计时并用同步门闩阻止同一渲染帧重复提交；输入兼容大小写、空格和常见连字符。新码生成事务清理超过 24 小时的失效摘要并对极低概率摘要碰撞重试。
- 新增 `internal/discovery`：UDP/53318 签名组播、每接口定向广播、单播响应和受限手工 IP 探测；公告 2048-byte 上限、TTL=1、直连前缀校验、15 秒过期、nonce 重放窗、响应限频。发现后用临时 TLS 1.3 双向 Ed25519 控制通道交换现有签名 ICE envelope，文件仍只经 Pion ICE + QUIC。未配对 LAN 设备只有在发送方选择、接收方确认且双方完整完成后才 pin；失败/拒绝不留信任。成功地址随 pin 保存，重启后定向探测，不扫网段。
- Windows 物理网卡为 `10.234.232.205/16`、Private，Mac 为 `en0=10.234.171.192/16`，双方另有 Mihomo/utun。校园混合有线/Wi-Fi过滤组播与定向广播；受限单播实测 Windows 正确选择“以太网”、Mac 正确选择 `en0`，互相发现签名身份，Windows→Mac TLS 控制心跳 PASS。Mac→Windows 被测试探针现有两条 Windows Private/Public 入站 Block 规则拒绝，未修改防火墙，正式应用仍需首次运行允许专用网络。证据 `/tmp/codex-ssh/linksend-lanprobe-unicast-v3-20260911T235205Z`。
- 接收热路径把已验证字节统计从每 ACK 全量扫描改为 O(1) 单调计数；正文仍逐块 `file.Sync`，恢复位图最多每 8 块或 500ms 批量 checkpoint。checkpoint 落后不丢数据，因为重启仍逐块重算 staging 哈希。传输中保存设置不会取消 LAN 任务，发现配置在当前任务完成后再重启。
- 已通过 Windows 根普通/race/vet/`GOWORK=off`、桌面独立 verify/test/vet/build、前端 typecheck/lint/11 tests/build、Wails 3 production（1 service / 33 methods / 17 models）。香港主站已部署 `0.4.0` 与 schema 2 并完成公网配对复核；Mac 已安装包含 LAN envelope 签名修复的候选，受限单播发现、mTLS 控制心跳及停止后台接收后的并行文件准备/ICE/QUIC 精确路径 PASS。该精确诊断路径耗时约 0.36 秒，但新版原生确认弹窗的完整人工点击仍不提前标记 PASS。

## 2026-09-12 DHCP、trickle ICE 与双机连续发送修复

- 后续及时性优化把认证 WSS 改为单后台读泵，空闲接收与发送借还同一连接；业务 context 取消不再破坏 socket，心跳响应由泵消化。Mac 10 轮接收始终 `connection_count=1`，本地双向复用回归两端总 WSS 数保持 2。香港主站在双方 `end_of_candidates` 转发后立即释放协商，事务部署前备份 `/opt/linksend-lan-test/backups/20260911T223124Z-transport-wss-reuse` 校验可读且数据库 integrity=ok；新 PID `389489`、二进制 SHA256 `d48c2c9ec50a71eddfdcf8caf97307c7b45c70b18c629e63fc2127fd3ef83d13`，公网 health/协议 V1/QUIC/`relay=false`、schema 1 和 recent fatal=0 均 PASS。部署 job `/tmp/codex-ssh/deploy-linksend-wss-reuse-20260911T223111Z`，未触发回滚。
- 源文件 Prepare/hash 与信令、ICE、QUIC 并行；安全自动网卡结果在后台接收启动时预热，并在每次复用前按原接口和当前 IP 复核，DHCP 变化会重新枚举。任务保存最多 64 个本地 phase 时间点及连接阶段耗时；纯 UI 进度/phase 不再同步提交 SQLite，块发送/接收、恢复元数据和终态仍持久化，避免历史写入阻塞每个 QUIC ACK。
- 响应端在发出 `connect_response` 前注册 pinned TLS QUIC listener，避免 initiator 首包早到后等待 PTO；成功终态增加兼容 `confirmed_ack`，标准 QUIC close 的 draining/endpoint 回收移出用户完成路径。物理 Windows→Mac 新版连续 10/10 PASS：1,296–1,604 ms，平均 1,452 ms，相对同现场旧 3,521–3,749 ms 平均缩短约 59%；发送端 WSS 0–0.7 ms、endpoint 81–90 ms、QUIC 多数 11–15 ms。十个不同文件两端 SHA256 逐一一致，证据 `/tmp/codex-ssh/inbox-opt-v16-20260911T223358Z`、复核 `/tmp/codex-ssh/verify-linksend-files-v16-20260911T223613Z`。跨 NAT、原生弹窗人工点击、IPv6/网络切换/睡眠唤醒仍未据此宣称通过。
- 最终源码快照在 Windows 通过根普通/race/vet/`GOWORK=off`，桌面独立 verify/test/vet/build，前端 typecheck/lint/8 tests/build 与 Wails production。Windows 当前运行 `apps/desktop/bin/LinkSend.exe`，版本 0.3.0、SHA256 `9df2116bb23ebaaf6b4790fd7b01b21f488d99f761a9880ccf8129729473dacc`。Mac 同源码普通/race、桌面、前端及 arm64 DMG 构建通过，DMG SHA256 `7988bf1bf99f1155de4d4c2d3e382faf185ebeaf35ac0f8ecb4c896d8061ae25`；已事务安装到 `/Applications/LinkSend.app`，运行二进制 SHA256 `8fd1c04724dd97e5b49cf5917fd254d78f6f0406550ac0647f7882f98405eb2c`，替换前备份为 `/Users/wen/Library/Application Support/LinkSend/app-backups/20260911T224737Z-transport-v16/LinkSend.app.copy`。

- 使用 `codex-ssh-manager` 将 `mac-test-102342413` 更新为 `wen@10.234.171.192`，resolve/probe/audit-host 通过。Mac 当时运行 0.3.0，但偏好仍固定旧地址 `10.234.0.86:0`；已先备份再清空为自动选择并重启。备份为 `/Users/wen/Library/Application Support/LinkSend/desktop-preferences.json.pre-dhcp-bind-fix-20260911T193855Z.bak`。
- 桌面运行时现在检查已保存的字面量 IP 是否仍属于当前接口；DHCP 旧地址本次运行回退自动选择，环境变量显式绑定不被覆盖。endpoint 接受纯 IPv4/IPv6 并补 `:0`；普通 MTU 相同时优先非 point-to-point 接口，显式优先级仍可选择隧道。现场稳定选择 Mac `en0 10.234.171.192` 与 Windows `以太网 10.234.232.205`，未选 `198.18.0.1` TUN。
- 原实现串行等待两端完整 STUN gathering，小文件在 ICE checks 前约空等 8 秒，而真实 ICE 只需约 0.1–0.4 秒。现改为请求/响应立即发送、候选逐个签名 trickle、Pion checks 与 gathering 并行；验证到 `lan_direct` host path 后双方正常提前结束无关 STUN 等待。
- 初版并行实现曾在高重复/race 中复现 QUIC code 0 提前关闭：Pion 初始从 peer-reflexive 切换为 host pair，被旧逻辑误判为运行中迁移。endpoint 现分 provisional/final 两阶段，初始收敛不关闭 socket，候选交换结束后才固定 pair 并开启后续路径监控。修复后关键连接套件连续 20 轮及 race 通过。
- 响应端在已验证请求后若 endpoint、候选、ICE/QUIC 或期望 peer 检查失败，会返回绑定 sender/recipient/session/generation 且经 Ed25519 签名的受限 `status` 稳定码；发起端验证后立即结束，不再把确定的对端失败误报成 30 秒超时。消息不含系统 cause、路径、token 或 ICE credential。
- Windows profile 启用了 EFS，历史 `trust.json` 的加密状态不同，成员同步时 `os.Rename` 返回无法移动到不同磁盘，造成 session 前间歇性 `DIRECT_FAILED`。连接热路径现优先使用本地 pin，仅缺 pin 时刷新服务端设备；名称变化不再重写 pin。信任保存增加 Windows `ReplaceFileW` 与带 `trust.json.previous` 校验/崩溃恢复的原位兜底。真实 EFS 目录上 `devices` 已退出 0、JSON 可解析且恢复日志清理；修复前备份为 `.artifacts/diagnostics-20260912-bind-fix/trust.json.before-efs-replace-fix`。
- 物理 Windows→Mac 连续 10/10 PASS，每轮新建 WSS/session/UDP endpoint，耗时 3521–3749 ms，全部 host↔host、`lan_direct`、TLS 1.3、QUIC、`relay=false`。Mac 十份文件 SHA256 均为 `40548e1b1e13d54f38ef44be02a25fe6f1a82490f4d33f475043c1d965317955`，与 Windows 源一致。证据 `/tmp/codex-ssh/live-repeat-v5-20260911T205855Z`，复核 job `/tmp/codex-ssh/verify-mac-repeat-v5-20260911T210112Z`。自动接收覆盖连接到 `AwaitingAcceptance`，原生弹窗人工点击仍未冒充自动化 PASS。
- Mac v5 源包 SHA256 `57b17507d6d8feeef708e4c4ecad4e2018e640f1c3d3939156a9c7e9ee8d1272`，arm64 DMG SHA256 `e4c115dd49576459289c75b628f5e482aca3324e5a8bee7f3649a524ddd42c80`；Mac 普通/race/desktop/原生构建通过并安装到 `/Applications/LinkSend.app`，运行二进制 SHA256 `907ebaa6a7f25158c959effd4eb50aec90afb3ff4ef122c0fcad69cd928b7519`。替换前备份为 `/Users/wen/Library/Application Support/LinkSend/app-backups/20260911T205524Z-connectivity-v5/LinkSend.app.copy`。
- Windows 当前运行 `apps/desktop/bin/LinkSend.exe`，SHA256 `1650b6ad6ac690c9fd29e29d0b31a783b8a407c596d2c8845b3465e908f548b7`，版本 0.3.0；Windows 与 Mac 的保存 bind 均已改为自动，Windows 偏好备份为 `.artifacts/diagnostics-20260912-bind-fix/desktop-preferences.json.before-auto-bind-windows`。两端最终 GUI 均已恢复，Windows 2 条、Mac 2 条 TCP 连接 Established；Mac 最终复核 job `/tmp/codex-ssh/verify-mac-gui-v5-final-20260911T210933Z`。最终实际通过：根普通测试、完整 race、vet、`GOWORK=off`；桌面独立 test/vet/build；前端 typecheck/lint/8 tests/build；Wails Windows build。当前仍是未提交测试快照，未推送、未发布，跨 NAT/IPv6/睡眠唤醒未因本轮 LAN 结果改写。

## 2026-09-12 Windows 在线但 `DIRECT_FAILED` 排查与绑定地址修复

- 本机持久任务历史显示最近两次发送均在约 3 秒内结束于 `connecting` / `DIRECT_FAILED`，实际发送 0 B，且没有 `session_id`、ICE 候选、STUN 计数或路径证据；因此失败发生在本地 UDP/ICE endpoint 创建之前，不是文件内容、接收目录或已建立直连后的传输失败。
- 当前 `%APPDATA%\LinkSend\desktop-preferences.json` 保存的 `bind_address` 为纯 IP `10.234.232.205`。该地址仍是物理以太网的有效地址，但旧 endpoint 构造直接传给 `net.ResolveUDPAddr`，要求 `IP:port`，从而在 presence 仍显示在线时提前失败；桌面任务层将该非协议底层错误归并成了笼统的 `DIRECT_FAILED`。
- 当前配置已修正为 `10.234.232.205:0`，应用正常退出后重新启动；窗口创建成功，后台信令连接保持 Established。没有自动向对端发送测试文件，物理双机重试仍需用户在界面发起，不能用启动/信令检查冒充传输通过。
- `internal/connectivity.New` 现兼容纯 IPv4/IPv6 字面量并自动补临时 UDP 端口 `:0`；新增 `TestIPOnlyBindUsesEphemeralPort`，避免用户再次因省略端口进入同一失败路径。协议、ICE、QUIC、TLS 和文件数据路径未改变。
- 实际通过：`go test -count=1 ./internal/connectivity`、根模块 `go test -count=1 ./...`、`go vet ./...`、根 `GOWORK=off go test -count=1 ./...`；桌面 `GOWORK=off go test -count=1 ./...`、`go vet ./...`、`go build ./...`；Wails 3 Windows/amd64 production build。首次构建因独立 `task` 命令不存在退出 1，改用已固定的 `wails3 task` 后首次因 PATH 未包含 `wails3` 退出 1，显式加入 `.tools/bin` 后通过。
- 修复构建 `apps/desktop/bin/LinkSend.exe` 为产品 `0.3.0`，SHA256 `EDFA11F57EEE4C477AC95346212F48BD0AEF30D9DAD99F37DF3A2AC92C7658C5`。直接覆盖 `C:\Program Files\LinkSend contributors\LinkSend\LinkSend.exe` 被 Windows 权限拒绝，未绕过权限；旧安装文件已备份到 ignored 本地诊断目录。已安装版当前依靠修正后的 `IP:0` 配置工作，源码兼容修复待下一次安装包更新。

## 2026-09-11 v0.3.0 局域网连接、配对码与自动接收简化（当前交付源码）

- 按最新产品决策降低首次配对门槛：动态配对码改为 40 位随机量的 `ABCD-EFGH`，10 分钟有效、一次性、服务端仅存规范化摘要；输入忽略大小写/连字符/空格，旧 43 字符邀请继续兼容。配对成功后客户端自动固定认证成员列表中的 Ed25519 公钥，不再要求手工核对 64 位指纹；既有 pin 发生密钥变化仍拒绝。首次配对现在明确信任信令服务，安全边界已同步更新到 SPEC/SECURITY/PROTOCOL。
- `internal/app/inbox.go` 增加不产生空闲任务历史的常驻接收器：桌面打开且存在接收目录时自动连接 WSS、保持设备在线、收到连接后才创建接收任务；任务结束后自动恢复监听。发送会有界停止本机空闲监听，完成后恢复；对端刚恢复监听的 `PEER_OFFLINE` 有 4 次短退避重试，ICE/QUIC 失败不会盲目重试。
- 接收 manifest 后进入 `AwaitingAcceptance` 并由 React 弹出确认层，显示发送设备、内容、数量、总量和目标目录。用户可勾选“以后自动接收此设备”，偏好写入 `trust.json`；后续仍执行固定设备密钥、TLS 1.3、块哈希、完整摘要和安全落盘校验。设备页可恢复为每次确认。
- 当前 Windows 网卡审计确认旧自动顺序会先选 Mihomo TUN `198.18.0.1`（Go MTU 65535），而不是物理以太网 `10.234.232.205`。自动选择现在在没有显式优先级时先选 1280–9000 的常规链路 MTU；不按接口名或私网段硬编码，显式优先级仍可选 TUN。实际选中远端地址属于本接口直接配置网段时才报告 `lan_direct`。
- UI 移除“设备组、手工信任、开始等待接收”，收敛为生成码、输入码、选择设备发送和接收弹窗。`frontend-skill` 指导下保留既有克制桌面系统并强化发送主动作；Playwright 实际检查桌面传输页与 700px 配对页布局，截图位于 `output/playwright/`。浏览器预览缺 Wails runtime 的 404 仅作为预览限制，不计原生功能通过。
- 实际通过：根模块普通测试、完整 race、vet、`GOWORK=off go test`；配对码规范化/单次消费测试；真实 loopback Pion ICE + quic-go 的“无手工 trust + 无 StartReceive + 首次确认 + 第二次自动接收”回归；桌面模块 `GOWORK=off` mod verify/test/vet/build；前端 typecheck/lint/8 tests/build；Wails 3 production Windows/amd64 build（1 service / 31 methods / 15 models）。最终本地 `apps/desktop/bin/LinkSend.exe` SHA256 为 `814C4AF7587733F199F081DF1211C715C4830F50005AA88AFADF35FB8C2AF7A1`；隔离 profile 隐藏启动 4 秒仍存活后由本轮检查结束。
- 当前交付请求：产品版本提升为 `0.3.0`（协议仍为 V1），以同一源码提交更新香港测试主站并生成 Windows 与 macOS arm64 测试包；部署与产物证据追加在本节。新版物理 Windows↔macOS 局域网、真实原生接收弹窗、按设备免确认、跨 NAT、IPv6、网络切换和睡眠唤醒仍需单独实测，不能用 loopback 或浏览器预览冒充。
- 源码提交为 `df8732b72ea18727b455441c94debc0ebf4b5d49`，本轮未覆盖 `v0.2.0` tag、未创建 Release。Linux rendezvous 从该提交的独立干净 clone 构建，`vcs.modified=false`，SHA256 `431a6939ab074e12a27a6e321691cd0be1a35f6529f09ce8e2dc8cdf8d0bfa99`。
- `hk-main` 经 SSH manager resolve/probe/audit 后执行事务部署。首次只读脚本因错误假定数据库路径及源站系统 CA 信任退出 1，没有改动；修正后确认旧 PID `381064`、版本 `0.2.0`、数据库 `/opt/linksend-lan-test/data/server.db`、schema 1、integrity=ok。部署备份 `/opt/linksend-lan-test/backups/20260911T145102Z-v0.3.0-df8732b` 可读且备份数据库 integrity=ok；新 PID `384944`，公网 health 为 `0.3.0`/协议 V1/QUIC/`relay=false`。动态短码格式、单次加入和双方自动 pin 经隔离公网 profile 验证 PASS，测试设备已撤销、临时身份与候选已清理；独立复核 `RECENT_FATAL_COUNT=0`。部署 job `/tmp/codex-ssh/linksend-v030-deploy-20260911T145049Z`，独立复核 `/tmp/codex-ssh/linksend-v030-independent-verify-20260911T145144Z`；未触发回滚。
- Windows amd64 从同一干净提交使用 Go 1.26.5、Wails `v3.0.0-beta.18`、pnpm 11.19.0、NSIS 3.12 构建。EXE 与安装器 FileVersion/ProductVersion 均为 `0.3.0`；便携 ZIP 完整性及精确 EXE 隔离启动 4 秒 PASS，安装器 7-Zip 检查包含 `LinkSend.exe` 和 WebView2 bootstrapper。便携 ZIP SHA256 `8fe347c28043dcce3083f6c0c664192da9ff1d13789b9429b57feb99b6007dcb`，NSIS 安装器 SHA256 `ba7b5f45f2ae149bef49c4e3b2d78995b5e0b1f525713896a5822a42a9a42b12`；两者均为 `UNSIGNED_TEST_BUILD`，安装/卸载未运行。
- Mac 构建源包先由同一提交通过 `git archive` 生成并排除历史 `.artifacts`，SHA256 `d0c8224b39b424ef368bd3b57aa25d678652a1ba5f35412bb6a67fda9f59b2d6`。`mac-test-102342413` 两次 manager probe 均 TCP/22 timeout，audit 同样失败；用户随后明确授权推送、合并并改走 GitHub Actions，因此该物理 Mac 阻塞不再阻止生成，但物理启动仍为 `NOT_RUN`。
- [PR #7](https://github.com/Wen5555/LinkSend/pull/7) 从 `codex/v0.3.0-pairing-inbox` 合并到 `main`，merge commit 为 `0fc36f1ca79dc8af7e304b8c27a0b0607d3654c0`。分支的两组 core、两组 desktop 与三平台 packaging 全部 PASS；合并后的 main core run `34635195414`、desktop run `34635195393`、packaging run `34635195522` 也全部 PASS。仓库没有配置 required-check 规则；合并前仍显式核对了 7 个实际 check 全部成功，没有借此绕过失败。
- main run `34635195522` 的 Windows ZIP/NSIS 与 macOS arm64 DMG 已下载并逐项复核，三者 `source_commit=workflow_head_sha=0fc36f1...`、`source_state=COMMITTED`、`source_checkout_clean=true`。Windows ZIP SHA256 `c88109e2c9d011bb66fdebbefa571516e856f8ea95f98626bc567d3b4db7d742`，安装器 SHA256 `bc984b6499a2b48f346f8990f1fd77b861f1753917e751bc27dd73ed75ec32c6`；版本资源、ZIP 完整性、NSIS 内置 EXE/WebView2 和精确 CI EXE 隔离启动 4 秒 PASS，仍为 unsigned，安装/卸载 NOT_RUN。
- macOS arm64 DMG SHA256 `07604a4bd3efe69c12b34dbad502ad9aada4209d43aaab7e63bd961a7285a232`。runner 已执行真实 DMG 挂载、bundle id `com.linksend.desktop`、版本 `0.3.0`、arm64 与 `codesign --verify --deep --strict` 检查；本机又完成下载哈希、7-Zip HFS+ 完整性、plist 和 Mach-O `GOOS=darwin/GOARCH=arm64` 复核。Windows 展开 DMG 时仅因 `Applications` symlink 权限返回退出 2，其余 app 内容可读；这不替代物理 Mac 启动。DMG 仅 ad-hoc 签名、未公证。
- 最终文件集中在 `.artifacts/v0.3.0-0fc36f1/deliverables/`，包含简化命名的 Windows 安装器、Windows 便携 ZIP、macOS arm64 DMG、BUILD-INFO、README 和 SHA256SUMS。没有创建或覆盖 `v0.2.0` tag，也没有创建 `v0.3.0` Release。

## 2026-09-11 v0.2.0 已合并并发布测试预发布

- [PR #6](https://github.com/Wen5555/LinkSend/pull/6) 通过 fast-forward 合并到 `main`，merge/tag 目标均为 `426d58b6ab62ab7213475007305c0a403955c00f`。main 的 core run [`34600609146`](https://github.com/Wen5555/LinkSend/actions/runs/34600609146)、desktop run [`34600609047`](https://github.com/Wen5555/LinkSend/actions/runs/34600609047) 和三平台 packaging run [`34600609161`](https://github.com/Wen5555/LinkSend/actions/runs/34600609161) 全部 PASS。
- [GitHub Release v0.2.0](https://github.com/Wen5555/LinkSend/releases/tag/v0.2.0) 已公开为 Pre-release，并明确设为非 Latest。用户后续明确要求合并和发布，因此覆盖了发布前“暂不合并/发布”的决策；已知产品 FAIL 与 NOT_RUN 项没有被改写为 PASS。
- Release 上传 Windows ZIP、Windows installer、macOS arm64/amd64 DMG、合并 SHA256 清单和构建 manifest。GitHub 返回的四个二进制资产 size/digest 与本地重新计算、workflow artifact 内 `SHA256SUMS.txt` 全部一致；三份 BUILD-INFO 均记录 `source_commit=workflow_head_sha=426d58b6ab62ab7213475007305c0a403955c00f`、`workflow_run=34600609161`、`source_state=COMMITTED`、`source_checkout_clean=true`。
- Release Windows ZIP 内 EXE 为 20,635,648 bytes、SHA256 `afd7bcbf9bc58a2a5d590b8a470a2c74a6caf6d2a80cc4dc4e0729627c55a0b0`，FileVersion/ProductVersion `0.2.0`，Wails 3 beta.18。对该确切 EXE 的隔离可见原生冒烟为窗口句柄非零、idle `WM_CLOSE` 接受、10 秒内 exit 0；隐藏窗口方式不能取得句柄的测试方式 FAIL 也未冒充产品结果。
- 两份 Release DMG 的 runner 挂载、bundle/版本、架构和 strict ad-hoc codesign PASS，但本 release 资产未在物理 Mac 启动；无 Developer ID、未公证。Windows 安装/卸载也仍为 NOT_RUN。
- 香港测试主站继续运行服务端实际源码 `4b7ccf66...` 构建的产品 `0.2.0`；后续客户端/文档提交未改变 rendezvous/server/signaling/config/schema，因此未无意义重启。下一次产品版本或服务端依赖变化继续按 [DEPLOY-HK](DEPLOY-HK.md) 的事务流程同步。

| Release 资产 | 大小（bytes） | SHA256 | 构建 UTC | 签名 / 公证 |
|---|---:|---|---|---|
| Windows amd64 ZIP | 8,403,529 | `dcdd7127c527b1464f0ea025046ad871a35ed6eaba4a2c725884779071d81577` | `2026-09-11T12:49:31Z` | unsigned / N/A |
| Windows amd64 installer | 9,996,124 | `99a4d5d25dbc9761e4839ff435f362ec65aa75fcdafdf46f285f05defdde7370` | `2026-09-11T12:49:31Z` | unsigned / N/A |
| macOS arm64 DMG | 8,170,928 | `c637897290677c7deb8c50395b95568f55a93a14bdf51e2babbf7eaf974fdf4c` | `2026-09-11T12:48:16Z` | ad-hoc / NOT_RUN |
| macOS amd64 DMG | 8,808,743 | `f5f066634429e61312066c056b836482ca228315ea858cab3de28653e67120fe` | `2026-09-11T12:51:36Z` | ad-hoc / NOT_RUN |

准确边界保持不变：MASQUERADE-only 独立双 NAT 是产品 `CHECK_TIMEOUT` FAIL；Relay 未实现；完整原生交互、物理跨平台恢复、安装/卸载、签名和公证未完成。`v0.2.0` 只能称为测试预发布。

## 2026-09-11 v0.2.0 发布前候选记录（历史）

- 版本决策：从 `0.1.0` 提升到 `0.2.0`，因为本轮在 1.0 前加入任务 schema 2、暂停/重启恢复、验证后缺块续传、多网卡诊断与连接生命周期修复，属于明显功能扩展；不提升到 `1.0.0`，因为 MASQUERADE-only NAT、物理网络切换、完整原生交互、签名/公证等仍未通过。
- 产品版本与协议分离：`internal/protocol.ProductVersion=0.2.0`；协议 `Version=1`、TLS ALPN `linksend/1` 和 V1 文件帧保持不变。CLI/rendezvous `--version`、`/healthz.version`、capabilities、Wails DTO、Windows/NSIS、macOS plist 和前端 package metadata 统一使用 `0.2.0`。
- 仓库实际使用 Wails 3 `v3.0.0-beta.18`；旧 AGENTS 项目说明中的 Wails 2 不是有效实现状态，不允许据此回迁。
- `.artifacts`、本地 Wails 工具、Task cache、输出目录、WebView2 bootstrapper 和临时远程脚本已加入忽略规则，但没有删除任何既有文件。历史 `0.1.0` Stage/Release 文档保留原事实并标记为历史快照。
- 用户已明确香港入口是测试主站，并授权以后每次产品版本或服务端依赖更新时同步服务。每次仍执行 manager resolve→probe→audit-host 与 `inspect → backup → change → verify → rollback-ready`；只操作 `/opt/linksend-lan-test` 的测试服务，不修改防火墙、路由、DNS、代理或正式身份数据。本轮部署、验证和回滚方法见 [DEPLOY-HK](DEPLOY-HK.md)。
- 本节开始时是基线 HEAD `fef3e3f59797a6de25cb7f9b1f2a1850512808d5` 加未提交修改；旧 r2 资产继续标为 `UNCOMMITTED_TEST_SNAPSHOT`。后续已从真实干净提交重新构建 committed 候选；旧资产没有被放入新 ZIP/DMG，也没有改写提交元数据。
- `0.2.0` 本机预提交门槛：根 `diff --check`、mod verify、全量 test、race、vet、GOWORK=off test/build PASS；P0 立即重连 20 次和真实 QUIC 终态/恢复定向套件 3 次 PASS；Wails bindings 1 service / 27 methods / 14 models，生成后前端 frozen install/typecheck/lint/8 tests/build PASS；桌面独立 mod verify/test/vet/build、Wails production 与 NSIS 3.12 PASS。
- 版本资源复现并修复：Wails 3 beta.18 自动 build-assets 生成的 Windows version info 缺 `FileVersion` 显示字符串，使主 EXE 的 PowerShell VersionInfo 为空；将 fixed file/product version 规范为 `0.2.0.0`、使用 `0409` string table 并补 `FileVersion=0.2.0` 后，主 EXE 与 installer 的 FileVersion/ProductVersion 均实际返回 `0.2.0`。新增跨源码/Wails/安装元数据同步测试防止回退。
- 源码提交 `4b7ccf66f89c9720d9b5970e5d610b453048d69c` 已建立。直接在保留 untracked 的主工作树和共享 Git worktree 构建均被 Go 标为 `vcs.modified=true`，两次资产均拒绝部署；改用独立干净 clone 后 Linux amd64 rendezvous 的 `go version -m` 为该 revision、`vcs.modified=false`，大小 11,309,218 bytes，SHA256 `c549cdbf1bd461d6f583302f96700471000aacba1fed912c66d6e74912f30247`。
- 香港测试主站已按 manager 事务完成 `0.2.0` 同步：备份 `/opt/linksend-lan-test/backups/20260911T101714Z-v0.2.0-4b7ccf6`，旧 PID `352572` → 新 PID `381064`；配置 SHA 与数据库 schema/integrity 不变。公网 health 报告产品 `0.2.0` / 协议 V1；两次完成后立即重连、拒绝后立即完成各传 2 MiB 均 PASS。测试运行时已删除，两条测试设备记录均撤销，额外数据库清理备份为 `/opt/linksend-lan-test/backups/20260911T102329Z-v0.2.0-lifecycle-cleanup`。旧主站立即重试 FAIL 因实际部署复核而关闭。
- 候选提交 `9cce3eef20a6224f56c9da79d16f8966cabb61f9` 的三平台 packaging run `34592549254` 全部成功，但同 SHA 的 push core run `34592549314` 在真实 QUIC `TestDirectServiceSendAndReceive` 复现 `Application error 0x0 (remote): closed`；PR core 的成功 run 不抵消这个 FAIL。根因是接收端读到 `confirmed` 后正常关闭单次传输连接时，quic-go 可能先交付 application close code 0、后交付 stream FIN，发送端 terminal flush 将正常终态误判为失败。
- 最小修复只在“已验证 completed、已写出 confirmed”的 terminal flush 内接受远端 QUIC application code 0；非零 close、reset、timeout 和其他阶段错误仍失败，协议 V1 不变。新增真实 QUIC 正向及非零 close 负向测试；Windows 上完成用例 100 轮、含负例套件 50 轮、race 20 轮和 app 端到端 100 轮均 PASS。修复提交为 `a88553180bd1defac5b236d75fe4dff5046734dd`；`9cce3ee` 及其包已被取代，只保留为缺陷前对照证据。
- 发布前当时，`a885531` 的 GitHub required checks 已完成：core push run `34596127764` PASS、core PR run `34596131146` PASS、desktop push run `34596127757` PASS、desktop PR run `34596131090` PASS；三平台 packaging run [`34596127738`](https://github.com/Wen5555/LinkSend/actions/runs/34596127738) 的 Windows amd64、macOS arm64 和 macOS amd64 jobs 全部 PASS。该检查点的决定是暂不合并或创建 Release；之后用户明确覆盖该决定，实际发布状态见本页顶部。
- committed Windows ZIP 内 EXE 为 20,635,648 bytes、SHA256 `afd7bcbf9bc58a2a5d590b8a470a2c74a6caf6d2a80cc4dc4e0729627c55a0b0`，FileVersion/ProductVersion 都是 `0.2.0`，Wails 为 `v3.0.0-beta.18`。隔离 profile 下原生窗口句柄非零，`CloseMainWindow()`/`WM_CLOSE` 被接受并在 10 秒内 exit 0；这只验证窗口创建和空闲关闭。测试 profile 的两个文件因本地命令策略未能清理，仍留在 ignored `.artifacts` 检查目录且不得提交。
- committed macOS runner 已完成 DMG 挂载、bundle、架构和 `codesign --verify --deep --strict`，但仅有 ad-hoc 签名，无 Developer ID、未公证。物理 Mac `mac-test-102342413` 在收尾时 resolve 成功，probe/audit-host 均 TCP/22 timeout，因此 committed arm64 DMG 启动为 `BLOCKED_BY_EXTERNAL_ENV`；不能用 runner 或历史 dirty r2 窗口证据替代。

### `a885531` committed 候选资产

所有资产均来自 workflow/head `a88553180bd1defac5b236d75fe4dff5046734dd`、`source_state=COMMITTED`、`source_checkout_clean=true`。工具为 Go 1.26.5、Node 22.15.0、pnpm 11.19.0、Wails 3 `v3.0.0-beta.18`；Windows 使用 NSIS 3.10，macOS 使用 Apple clang 17.0.0。

| 资产 | 大小（bytes） | SHA256 | 构建时间 UTC | 签名 / 公证 | 原生验证 |
|---|---:|---|---|---|---|
| Windows amd64 ZIP | 8,403,530 | `316a11b74cf146762138384c0cd84ce76b8ec19a48943b1b0abf23f89de37690` | `2026-09-11T11:55:19Z` | unsigned / N/A | 窗口创建与 idle `WM_CLOSE` PASS；其余 NOT_RUN |
| Windows amd64 installer | 9,996,125 | `aa346acecd333ea8757bb0ed2f6466de4767af82fdc6640332cd2d0c83311092` | `2026-09-11T11:55:19Z` | unsigned / N/A | 安装/卸载 NOT_RUN |
| macOS arm64 DMG | 8,170,931 | `79857ce7ba336e6ceec516b19b737a1386100e9f6dfdf328a44deaeaee142e91` | `2026-09-11T11:54:30Z` | ad-hoc / NOT_RUN | runner 包级 PASS；物理启动 BLOCKED_BY_EXTERNAL_ENV |
| macOS amd64 DMG | 8,808,731 | `15c34a786f5aba9b72e7e2e09eaf6502aebf315cfee6161b9918104543de621f` | `2026-09-11T11:57:59Z` | ad-hoc / NOT_RUN | runner 包级 PASS；Intel 真机 NOT_RUN |

发布前按原门槛判断仍未满足：MASQUERADE-only 独立双 NAT 保持产品 `CHECK_TIMEOUT` FAIL；物理网络切换、committed Mac 原生启动、完整 Windows/macOS 原生交互、Windows 安装/卸载、Developer ID 签名和公证仍未完成。后续虽按用户明确授权发布为 Pre-release，这些缺口仍然有效，不能声称生产可用或跨平台完整验收。

## 2026-09-11 P0/P1/P2 分阶段可靠性改进（历史 dirty snapshot）

- 基线仍为 `main` / `fef3e3f59797a6de25cb7f9b1f2a1850512808d5`；本节全部结果来自保留既有修改的 dirty working tree。未 reset、clean、提交、推送、创建 Release 或复用旧资产冒充本轮构建。桌面实际为 Wails 3 `v3.0.0-beta.18`。
- P0：同设备信令连接替换时按连接所有权清理旧协商；旧 handler 和旧 generation 不能删除/污染新会话；同时发起以设备 ID 字典序收敛到一个 initiator/session；信令短断不关闭健康 QUIC；`confirmed`、拒绝和 error 使用真实 QUIC 半关闭/有界读取完成终态交付。真实 Pion ICE + quic-go 回归覆盖同时发起（10 次）、旧 handler（20 次）、无响应超时后立即重试、信令断开后继续 QUIC、拒绝/冲突/confirmed 终态；受影响包普通和 race 均 PASS。
- 香港测试主站当时仍运行旧 dirty revision `8baaa1cf1d67a48adbceb32ecb469999361a1e09`，二进制 SHA256 `53445be88943fbabdaa7aabfaf98e299ad9780b1084daa764cf8804e48e5a338`；短时间重试仍为产品 `FAIL`，不是外部环境阻塞。本轮新构建 `fef3e3f+dirty` 只在 `hk-main` 的 `127.0.0.1:39117` 隔离实例验证：两次完成后立即重连、拒绝后立即重连均 PASS，每轮 2,097,152 bytes 且接收内容 `cmp` 一致。主站二进制与 PID 前后相同，实验进程、配置、DB、身份和正文已回滚删除。成功 job：`/tmp/codex-ssh/linksend-p0-isolated-healthz-retry-20260911T062201Z`；首次误用 `/v1/health` 的脚本 FAIL 证据保留于 `/tmp/codex-ssh/linksend-p0-isolated-validation-20260911T061946Z`。
- P1：任务快照明确区分 `task_id`、`attempt_id`、`session_id`、ICE generation 与单调 `revision`；旧 attempt 回调不能覆盖新 attempt。状态覆盖 Preparing、AwaitingAcceptance、Transferring、Verifying、Paused、Recovering、Completed、Rejected、Cancelled、Failed，暂停/取消停止新块调度，校验/提交失败不能进入 Completed。
- SQLite 历史升级为 schema 2：snapshot 与私有 recovery metadata 按 revision 原子更新；v1→v2 前用 SQLite `VACUUM INTO` 生成权限为 `0600` 的可读 v1 备份；损坏行进入 `task_quarantine` 且不阻塞启动；写失败撤销 `history_persisted`。旧二进制回滚必须在应用关闭后恢复 `task-history.sqlite.schema-v1-*.bak`，不能直接打开 schema 2。
- 真暂停/重启恢复/缺块续传已接入现有 transfer 协议。恢复绑定 TransferID、manifest digest、chunk size、源路径及内容、接收目录、对端身份/指纹；`ResumeTask` 是正文恢复的显式用户确认，每次生成新 attempt/session/ICE credentials。接收端重哈希 staging，只请求缺失或损坏块；源内容变化和本地 pinned peer 变化在继续正文前失败。
- 真实 QUIC 续传证据：12 MiB 首块后暂停，未损坏时本轮/累计实际发送与接收 12,582,912 bytes、重传 0、唯一验证/提交 12,582,912；损坏首个 4 MiB staging block 时实际发送/接收 16,777,216、重传 4,194,304、唯一验证/提交仍为 12,582,912；完整重启两个 Service 后恢复 8,388,608 bytes，沿用 task/TransferID、使用新 attempt/session，内容一致且无重传。只有这些缺块协商与计数通过后，当前实现才报告 `byte_resume_supported=true`。
- `history_persisted`、`restart_recovery_supported`、`byte_resume_supported` 独立报告：模拟历史写失败时前两者为 false，而进程内 byte resume 实现仍为 true。部分文件提交记录会在每个成功 commit 后 checkpoint；后续文件失败时已提交文件和字节保持可验证，不显示整任务 Completed。
- CLI 新增只读 `status [--task]` 与显式 `resume --task ...`；resume 只拥有当前 CLI 进程启动的 attempt，不宣称能暂停/取消另一个桌面进程。Wails 增加 PauseTask/ResumeTask 并重新生成 bindings；React 按 `can_pause/can_resume` 门控，使用 revision 合并防止迟到轮询覆盖新快照，分别展示实际发送/接收、重传、验证、提交与双方确认；固定测试配对码不再常驻界面。
- P2 网络源码新增可用接口/地址族发现、显式接口优先级与排除、IPv6 link-local 排除、实际 base socket/interface/family、ICE 状态时间线、候选类型、TLS/ALPN、STUN 请求/响应计数和信令 JSON 字节计数。未按网卡名称、私网地址、presence 或 candidate type 推断 LAN/公网，连接方法仍为 `direct_unknown`。已选本地地址消失会废弃旧 endpoint、关闭当前 QUIC，并由任务恢复流程等待双方显式建立新 session。
- `scripts/dual-nat-netns.sh` 已在 `lab-server` 的 rootless 外层 user/mount/network/PID namespace 中真实运行：内部创建两个独立 NAT namespace、重叠 `10.77.0.0/24`、独立 WAN、coturn 4.5.2 STUN-only 与 HTTPS 信令，不使用 loopback 或单 Docker bridge。MASQUERADE-only/状态过滤运行真实返回 `CHECK_TIMEOUT`；这是当前无 relay 情况下的端口受限 NAT 能力 FAIL。显式固定 UDP SNAT/DNAT 的可穿透 NAT 运行完成 8,388,608 bytes QUIC，双方同一 session、内容 SHA256 一致，STUN request/response 为发送侧 16/6、接收侧 20/8，NAT_A/NAT_B 计数 9/7，`relay=false` 且仍为 `direct_unknown`。脱敏包 `.artifacts/final-20260911/linux-dual-nat-evidence-20260911.tar.gz` SHA256 `ba7447fe3eb603b669db3a76ab7ba2be049d7abeeefdb94b4927161f43f7eebf`；远端身份、私钥、正文、coturn、namespace 和测试根已回滚删除。宿主地址/路由前后相同；宿主 iptables 因无非交互 sudo 不能读取，但实验规则全部位于 rootless 外层 namespace。
- 仍未验收：真实网卡切换/睡眠唤醒、公共 IPv6、未配置固定映射的更多 NAT 类型、Windows/macOS 新版原生文件/目录选择、打开目录、红点/Cmd+Q 按键、退出保护和重启恢复入口。浏览器或窗口创建证据不能替代这些原生操作。
- 最终 Windows 门槛：根模块格式、`git diff --check`、`go mod verify`、普通测试、race、vet、`GOWORK=off` 测试全部 PASS；桌面独立模块 `GOWORK=off` verify/test/vet/build PASS；前端 frozen install/typecheck/lint/8 tests/build PASS；最后一次 Wails production build 确认 EXE 中包含当前 `index-CQOAaBGW.css` 与 `index-Dz-ecqi4.js`。
- 发布脚本复现出 NSIS 默认项目名仍为 `LinkSendTemplate`，会生成错误命名的安装器并把内置 EXE 改名。`apps/desktop/build/windows/nsis/project.nsi` 现显式固定 LinkSend 项目、公司、产品、版本、可执行文件和卸载注册键；NSIS 3.12 重建后输出 `LinkSend-amd64-installer.exe`，7-Zip 确认内含 `LinkSend.exe`。本机已有 `%APPDATA%\\LinkSend.exe` WebView 数据，因此未实际安装/卸载，避免触碰现有用户状态。
- Windows 原生冒烟使用隔离 profile 启动本轮 EXE：进程存活、主窗口句柄非零、Windows close message 返回 true 且进程在 10 秒内退出；随后精确删除测试 identity/SQLite。该结果不替代文件/目录对话框、打开目录或活跃任务退出保护点击验收。
- Mac 修复后包使用 `mac-test-102342413`，按 manager 的 resolve→probe→audit-host 和隔离目录流程执行。158 个明确源文件打包为 `linksend-source-fef3e3f-dirty-r2.tar.gz`，SHA256 `8612b015bdbdacae26d9d3915b95f9d9744d3576fbd280bfbef751faa940e3f8`，排除全部旧 `.artifacts`、bin、node_modules 和临时脚本；根普通/race/vet、桌面 test/vet/build、arm64/amd64 顺序构建均 PASS。r2 构建 job `/tmp/codex-ssh/linksend-mac-rebuild-r2-20260911T085119Z` 的构建步骤完成，但验证脚本把请求架构 `amd64` 与 Mach-O 名称 `x86_64` 直接比较而最终退出 1；修正后的只读验证 `/tmp/codex-ssh/linksend-mac-rebuild-r2-verify-final-20260911T085952Z` 退出 0。该脚本缺陷不改写为构建 PASS 的退出码，也不影响两份已独立核验的 DMG。
- Wails 3 原生退出回调语义已修复：`ShouldQuit` 在无活跃任务时返回 true；`Rejected` 纳入终态；有活跃任务时先阻止原退出事件，由异步原生对话框回调取消任务，等待终态后再调用 `App.Quit`。桌面单元测试 PASS；Windows r2 EXE 的 `WM_CLOSE` 与 10 秒内退出 PASS；Mac arm64 r2 包创建 1 个约 1008×684 的原生窗口，idle quit Apple Event 与进程退出 PASS。由于 macOS Accessibility=false，红点实际点击、Cmd+Q 按键、原生对话框、键盘导航和活跃任务退出保护仍为 `NOT_RUN`，不能由 Apple Event 代替。
- r2 两个 DMG 的只读挂载、bundle id `com.linksend.desktop`、架构和严格 codesign 验证 PASS，签名均为 ad-hoc、无 TeamIdentifier、未公证。Mac 远端 r2 源码、build root、身份和两个原生测试 profile 已由 `/tmp/codex-ssh/linksend-mac-rebuild-r2-cleanup-20260911T090331Z` 清理；用户原有 `/Volumes/LinkSend` 挂载不属于本轮测试，未操作。
- 四个历史 r2 资产位于 `.artifacts/final-20260911/`，均标记 `UNCOMMITTED_TEST_SNAPSHOT` / 基线 `fef3e3f59797a6de25cb7f9b1f2a1850512808d5` / `vcs_modified=true`。其大小和 SHA256 保留于该目录 `ASSET-MANIFEST.json`；旧资产不能改名或复用为 `0.2.0` 提交构建。后续真实提交、香港部署和新资产在本页最顶部另行记录。
- 2026-09-11T09:24Z 对最终工作树重验：gofmt、`git diff --check`、根 verify/test/race/vet/`GOWORK=off test`、桌面独立 verify/test/vet/build、前端 frozen install/typecheck/lint/8 tests/build 全部退出 0；P0 server 立即重连重复 20 次、真实 QUIC 生命周期/终态重复 3 次及 P1 缺块/重启恢复定向回归再次 PASS。`scripts/dual-nat-netns.sh` 的清理补丁 `bash -n` PASS，运行态证据仍来自上述隔离实测，不冒充重跑。强制 bindings 首次因 Taskfile 子进程 PATH 不含 `wails3` 退出 1，临时加入仓库 `.wails-bin` 后生成 PASS（1 service / 27 methods / 14 models），生成后 TypeScript/lint 再验通过。最终 Windows Wails build 退出 0，EXE SHA256 `c808e31a54c8d4819373902f460ef2d5ffbe28b6603f9ab5353465e908f548b7` 与 r2 staging EXE 相同，并嵌入当前 `index-Dz-ecqi4.js` / `index-CQOAaBGW.css`。

## 2026-09-11 连续验收执行记录

- 环境基线记录于 `.artifacts/acceptance-20260911/baseline.json`；当前工作树原有未跟踪构建/测试产物保持不变。Windows amd64、PowerShell 7.6.5、Go 1.26.5、Node 22.15.0、pnpm 11.19.0、Docker CLI 29.2.1；Wails 3 `v3.0.0-beta.18` 已按锁定版本安装到本地工具目录。
- 根模块 `gofmt`、`git diff --check`、`go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`、`GOWORK=off go test -count=1 ./...` 均 PASS；桌面模块 GOWORK=off test/vet/build、前端 frozen install/typecheck/lint/8 tests/build 均 PASS。
- Wails `task build ARCH=amd64` 首次因 PATH 未包含 CLI 而失败（环境配置缺陷，原始错误保留在会话记录）；补充 PATH 后重新运行 PASS，并重新生成 TypeScript bindings。生成器发现 `internal/protocol.Capabilities` 的三个历史/恢复字段未同步到 `apps/desktop/frontend/bindings/.../internal/protocol/models.ts`，已更新该绑定文件；未改变协议或数据路径。
- `demo-local` PASS 仅限 loopback-direct（真实 TLS 1.3/QUIC、1 MiB 摘要一致）；`test-nat` 退出码 1，标记 BLOCKED_BY_EXTERNAL_ENV（Windows 无受控 Linux namespace/NAT fixture）。Docker Compose 静态解析 PASS（使用非生产 dummy token），Docker daemon 不可用，运行态 NOT_RUN。
- `hk-main` 与 `nl-highdefense` 使用 codex-ssh-manager 完成 resolve/probe/audit 及扩展只读审计，均 PASS；未执行 sudo、配置变更、防火墙/路由/DNS/代理操作。扩展审计证据中的服务状态、监听端口和配置路径已脱敏记录于 `.artifacts/acceptance-20260911/results.txt`。
- 该连续验收检查点当时尚缺 NSIS 和新 Mac 构建；两项已在本文顶部记录的后续发布准备阶段补齐。双 NAT/IPv6/网络切换/睡眠唤醒及完整原生交互仍为 NOT_RUN 或 BLOCKED_BY_EXTERNAL_ENV；模拟与 loopback 结果没有替代真实双机验收。

## 2026-09-09 Wails 3 migration and Hong Kong production verification

- Wails 3 CLI and Go module fixed at `v3.0.0-beta.18`; desktop entry/service/window/dialog lifecycle and generated TypeScript bindings now use Wails 3. The old v2 module, `wails.json` and `frontend/wailsjs` outputs were removed. Source snapshot and working-tree patch are retained under `.artifacts/pre-wails3-20260909-*`.
- Wails 3 Windows production build completed from the dirty working tree: `apps/desktop/bin/LinkSend.exe`, SHA256 `0DD18B54495B1320518B0E61903B2E9A155DDC7B902ACE320B564CA81BF5B27E`; `go version -m` reports `github.com/wailsapp/wails/v3 v3.0.0-beta.18`. The process remained alive for 4 seconds in a native WebView2 smoke check; no manual native click acceptance was performed.
- Hong Kong `hk-main` was inspected, backed up and updated using the local SSH credential. Candidate rendezvous SHA256 `53445BE88943FBABDAA7AABFAF98E299AD9780B1084DAA764CF8804E48E5A338` is running at `/opt/linksend-lan-test/rendezvous`; backup `/opt/linksend-lan-test/backups/20260909T133335Z-wails3` was verified with SQLite `PRAGMA integrity_check=ok`. `test_pairing_code="orion123"` and an explicit existing group are enabled; generic configs remain off.
- Real Hong Kong HTTPS/WSS control-plane evidence: fixed-code joins for `hk-win-wails3` and `hk-peer-wails3` returned `admin=true` in group `73cde936…`; a dynamic invitation joined `hk-dynamic-wails3` with `admin=false`; reusing that invitation was rejected. After a controlled server restart, health remained `status=ok`, `relay=false`, `transport=quic`, and the group persisted with 8 members/5 admins/0 revoked.
- Real two-way transfers through Hong Kong signaling and direct QUIC on the physical Windows interface `10.234.232.205`: 1 MiB Windows→peer transfer ID `4773432abda776c76882261f55c0b491`, SHA256 `30E14955…`; 2 MiB peer→Windows transfer ID `cf38b13233b0214c7d309e28bf823d0f`, SHA256 `5647F05E…`. Both reported TLS 1.3 (`772`), ALPN `linksend/1`, `relay=false`, host↔host candidates, and matching received hashes.

## Environment and limits

- Windows amd64, PowerShell 7, Go 1.26.5, Node 22.15.0, pnpm 11.19.0.
- The default PATH exposed MinGW-w64 GCC 8.1, which made Windows race binaries exit with `0xc0000139`. Scoop MinGW 16.2.0 is now installed and selected explicitly for race verification.
- Docker 29.2.1 client is installed, but the Linux engine is unavailable.
- Wails 3.0.0-beta.18 is available at `.tools/bin/wails3.exe`; WebView2 is installed, but macOS runtime validation is not available here.
- This round changed only the LinkSend service binary/config on `hk-main`; no unrelated firewall, DNS, proxy or certificate records were modified. At the time of this deployment snapshot no Git commit/push/reset/clean/stash had been performed; the later candidate commits and push are recorded in the 2026-09-10 section below.

## 2026-09-08 Hong Kong host connectivity preparation

- The reusable `hk-main` SSH alias was resolved and probed successfully (`root@109.66.88.204`, SSH port `60851`); a read-only host audit reported no failed systemd units and 22% root filesystem usage.
- The remote LinkSend rendezvous process is running from `/opt/linksend-lan-test/rendezvous` with `listen = "0.0.0.0:443"`, the configured origin certificate for `linksend.oooai.de`, and `relay=false`/QUIC capabilities. `GET /healthz` returned `status=ok` through both loopback and the public listener.
- coturn is running in STUN-only mode on UDP `3478`; no TURN allocation or relay port range is configured. nftables permits TCP `443` and UDP `3478` and retains a default-drop input policy.
- Windows reachability checks to `109.66.88.204:443` and `109.66.88.204:3478` succeeded. The origin certificate is self-signed but has SANs `linksend.oooai.de` and `*.oooai.de`; normal clients must use the Cloudflare edge hostname and must not disable certificate verification.
- Earlier check (superseded by the domain recheck below): DNS returned NXDOMAIN for both `linksend.oooai.de` and `stun.oooai.de`. Formal Windows/macOS WSS pairing and transfer testing is therefore pending the user's Cloudflare records: proxied `A linksend.oooai.de -> 109.66.88.204` and DNS-only `A stun.oooai.de -> 109.66.88.204`.

## Version control

The repository is managed with Git on `main`, tracks `https://github.com/Wen5555/LinkSend.git`, and has been pushed without force updates. Local identities, databases, build binaries, toolchains and temporary profiles are excluded by `.gitignore`.

## Milestones

| Milestone | Source implemented | Current evidence | Remaining |
|---|---|---|---|
| M0 | Two Go modules, workspace, Wails template, configs, docs, CI and commands | Root tests/vet/race, CLI/devtool, and desktop GOWORK=off checks pass; desktop CI path corrected in source | CI runner confirmation |
| M1 | Pion ICE + quic-go UDP demux, TLS 1.3 pinning, timeout/close paths | Windows loopback host ICE, encrypted bidirectional QUIC, wrong-pin negative tests, benchmark and `demo-local` pass | Real two-host LAN, public IPv6 and controlled dual NAT not run |
| M2 | Ed25519 identity, invite pairing, SQLite group, WSS auth, presence/session routing, revoke and local trust | signaling package tests and demo control plane pass | Reconnect/backoff and multi-session coordinator |
| M3 | Manifest/BLAKE3/secure receiver plus shared direct session API and CLI send/receive | App service test completes paired profiles through WSS, ICE, TLS 1.3, QUIC and transfer; demo transfers 1 MiB with matching content hashes and no file-sized signaling forwarding | Real two-machine LAN, public IPv6 and cross-NAT |
| M4 | Chunk checkpoint/recovery, safe staging, pause/cancel primitives | transfer unit tests and real quic-go loopback cancellation lifecycle tests pass | Process-kill restart acceptance through app/session coordinator |
| M5 | Thin Wails binding to shared identity/devices/diagnostics | Desktop Go source, frontend checks and Windows Wails production build pass | Windows interactive, macOS runtime and task UI |
| M6 | Deployment examples, explicit STUN-only config, diagnostics and benchmark entry points | Compose config parses; STUN-only runtime and HTTPS identity are not run | Docker daemon, HTTPS certificates, STUN Binding/TURN Allocate negative test, NAT lab and package signing |

## Commands run in this milestone

Passed:

```text
gofmt -w internal/signaling/client_test.go internal/app/service.go internal/app/demo.go cmd/linksend/main.go cmd/rendezvous/main.go cmd/devtool/main.go apps/desktop/app.go
go test ./...
go test ./internal/signaling
go run ./cmd/devtool demo-local
```

`demo-local` reported authenticated TLS 1.3, `relay=false`, a direct loopback candidate pair, 1,048,576 file bytes and about 3,171 signaling-forwarded bytes. ICE library close logs mention closed sockets during cleanup; they do not change the result.

Not run or blocked:

- The old GCC 8.1 race attempt exited with `0xc0000139` before tests; it is retained as a toolchain failure, not a race result.
- Real two-machine LAN, cross-NAT srflx, public IPv6, Windows to macOS and Linux namespace NAT: no target topology/privilege available.
- Docker compose and production HTTPS: Docker Linux engine/certificates unavailable.
- Independent desktop module and frontend checks were run below; macOS runtime and interactive desktop acceptance remain pending.

Additional verification completed after the initial update:

- `go vet ./...`: passed.
- `GOWORK=off go test ./...` at the root: passed.
- With Scoop MinGW 16.2.0 selected via `CC`, `CXX` and `PATH`, `go test -race ./...` passed across all root packages and integration tests.
- `apps/desktop` with `GOWORK=off`: `go test ./...` and `go build ./...` passed.
- `apps/desktop/frontend`: `pnpm run typecheck`, `pnpm run lint`, `pnpm run test` (3 tests) and `pnpm run build` passed.
- Historical Wails 2.15.0 production build passed before migration; the current Wails 3 build is recorded in the 2026-09-09 section above.
- After shared direct-session changes, Wails production build was rerun and passed again.
- `go run ./cmd/devtool test-nat` correctly returned nonzero with an explicit Windows/Linux-privilege explanation.
- `go run ./cmd/devtool bench-transport` passed on this Windows host: native 102.18 MB/s, ICE integration 98.87 MB/s for one exploratory iteration; this is loopback evidence only.
- `docker compose -f deploy/compose.yml config` passed with an explicit bootstrap token; the Docker engine itself remains unavailable, so image build/start was not run.
- `internal/app.TestDirectServiceSendAndReceive` passed: two profiles used the shared `ConnectDirect`/`AcceptDirect` API and `SendFiles`/`ReceiveOnce` API for an authenticated transfer.
- `go run ./cmd/devtool check-core` now runs ordinary tests, vet and race tests; it passed with the installed Scoop MinGW 16.2.0 toolchain.

## Stage 1A current scope

- Source changes: coturn 4.6.2 now receives the explicit `stun-only` option; the compose healthcheck is documented as liveness only; desktop CI enters `apps/desktop` and prints `GOMOD/GOWORK`; reserved CLI task commands return `NOT_IMPLEMENTED` without dispatching to unrelated state-changing methods.
- This Stage 1A run must separately record root tests, desktop-module tests, frontend checks and compose parsing. Wails production build, HTTPS certificate validation, coturn runtime and real Windows/macOS LAN/NAT traversal are not implied by those checks. Real quic-go loopback cancellation lifecycle is separately covered by Stage 1B.
- Stage 1A is complete in source and local automated checks; Stage 1B below records the current lifecycle implementation and evidence. Real network acceptance remains separate.

## Stage 1B current scope

- `internal/transport.QUICStream` now keeps quic-go `CancelRead`/`CancelWrite` behind a small transport boundary. Transfer cancellation and protocol failure abort both stream directions; normal completion still uses `Close`.
- Candidate exchange now uses a phase context. The first terminal sender/receiver error cancels its sibling, parent cancellation is classified as `CANCELLED`, and a missing `end_of_candidates` is bounded by a candidate phase budget.
- Signaling establishment, candidate exchange, ICE checks and QUIC handshake retain separate cancellation/timeout boundaries. The existing `CHECK_TIMEOUT` code remains the ICE phase code; signaling and QUIC timeout causes are no longer intentionally collapsed into it.
- Real quic-go loopback tests cover blocked acceptance read, blocked receive read, blocked ACK wait, repeated abort, candidate sibling failure, missing end message, malformed message and parent cancellation. These are lifecycle tests, not LAN/NAT evidence.

## Locked versions

- Go 1.26.5, module minimum 1.26.0.
- Pion ICE v4.4.2, quic-go v0.62.0.
- modernc.org/sqlite v1.58.0, coder/websocket v1.8.15, go-toml/v2 v2.4.3, zeebo/blake3 v0.2.4.
- Wails 3.0.0-beta.18, React 19.2.8, TypeScript 5.9.3, Vite 7.3.6, pnpm 11.19.0.

## Next concrete actions

1. After the two Cloudflare records resolve, run Windows to macOS real two-machine LAN acceptance with the Hong Kong rendezvous service and STUN. Preserve DirectEvidence, transfer hashes and failure-phase output.
2. Stage 2B: run different-network cross-NAT acceptance without relay.
3. Verify IPv6 and Linux interoperability.
4. Add persistent task orchestration, then Wails task UI, after real-network evidence is recorded.

## 2026-09-08 engineering optimization evidence

- Diagnostics now use a separate redacted identity DTO; health failures expose only a stable code/summary. Direct transfers can export read-only `DirectEvidence` from the selected ICE pair, endpoint counters and negotiated TLS/ALPN through the CLI `--evidence` flag.
- Transfer controls use typed operation constants and semantic validation. Accept/ack verified values are bounded, peer error detail is limited to 4 KiB and sanitized, frame limits reject invalid bounds, and progress rates are finite. Resume state parsing is bounded and rejects trailing JSON/unknown file IDs; fresh staging and checkpoint temporary files have best-effort cleanup. Cancellation persists `Cancelled`.
- ICE failures preserve cancellation, timeout, no-candidate and generic ICE failure categories. Signaling/QUIC wrappers retain underlying causes. Trust files are bounded; SQLite rejects unknown schema versions; server expiry cleanup and network closes avoid holding the mutex during websocket close.
- The root module path and desktop module path are now `github.com/Wen5555/LinkSend` and `/apps/desktop`; CI verifies modules with `GOWORK=off` and `go mod verify`. `devtool network-info` reports interface state/address data without credentials.

Verification on Windows amd64: root `gofmt`, `git diff --check`, `go mod verify`, `go test ./...`, `go vet ./...`, `go run ./cmd/devtool check-core` (including race), root and desktop `GOWORK=off` test/build, frontend typecheck/lint/test/build, Wails 3 production build, `demo-local`, and `network-info` passed. A 20x transfer repeat was first interrupted after an erroneous test harness blocked; the corrected transfer package passed its focused run. `test-nat` remains an explicit nonzero not-run result because this Windows host lacks the privileged Linux namespace fixture.

## 2026-09-08 domain recheck

- `linksend.oooai.de` resolves to Cloudflare addresses `172.67.208.195` and `104.21.45.36`; `stun.oooai.de` resolves directly to `109.66.88.204`.
- Windows STUN Binding request to UDP 3478 received an 88-byte success response with matching cookie and transaction ID. This is STUN reachability, not ICE/NAT acceptance.
- Public HTTPS `/healthz` returns Cloudflare 525. The source certificate is self-signed Ed25519; corresponding Cloudflare source connections fail with `tls: peer doesn't support any of the certificate's signature algorithms`. Origin-local HTTPS with the origin certificate explicitly trusted returns healthy capabilities and `relay=false`.
- `--server` must use `https://linksend.oooai.de`; the CLI converts HTTPS to WSS internally. Stage 2A commands have been corrected accordingly.
- No server configuration, certificate, firewall, or service was changed. Public pairing/transfer remains `BLOCKED_BY_EXTERNAL_ENV` until TLS is corrected and public HTTPS/WSS are verified.
- Read-only evidence: hk-main `/tmp/codex-ssh/linksend-domain-inspect-20260908T150418Z`.
## 2026-09-08 Cloudflare origin TLS repair

- Using the user-provided Cloudflare credential, an Origin RSA certificate for `linksend.oooai.de` was issued and installed on `hk-main`.
- Existing `/opt/linksend-lan-test/origin.crt`, `origin.key` and `server-public.toml` were backed up to `/root/linksend-lan-test-backups/20260908T151853Z/` before replacement.
- The rendezvous process was restarted with the same configuration. Public `https://linksend.oooai.de/healthz` now returns `status=ok`, `protocol_version=1`, `transport=quic`, `relay=false`.
- STUN remains available at `stun:stun.oooai.de:3478` (DNS-only). The certificate is valid for 7 days and must be renewed before `2026-09-15 15:12 UTC`.
- Verification job logs are retained at `/tmp/codex-ssh/linksend-origin-cert-rotate-20260908T151839Z`; rollback is restoring the recorded backup files and restarting the same rendezvous command.

## 2026-09-09 Stage 2A result

- Real Windows↔macOS LAN transfer passed. A 16 MiB random fixture completed through authenticated QUIC with TLS 1.3, `relay=false`, host candidates `10.234.49.250:62995` and `10.234.232.205:62109`, and 1,680 bytes of observed STUN traffic.
- Receiver and sender SHA256 values matched. Full evidence is recorded in `docs/STAGE2A-20260909.md`; sender output is under `.stage2a/20260909-000044/` and is ignored by Git.
- This closes the Stage 2A same-LAN evidence item. Cross-NAT, IPv6, different-network and desktop UI acceptance remain separate.

## 2026-09-09 Stage 2B readiness

- 加固两端 Stage 2A 入口：SkipBuild 复用并校验显式二进制及 SHA256；脚本固定仓库根目录并限制 role。
- CLI 在 --evidence 模式下对失败也输出结构化结果，stderr 与 result.json 分离；接收端新增独立 --wait-timeout。
- 2B-01 跨 NAT、Linux 双 NAT fixture、IPv6、网络切换和桌面 UI 均未运行，保持待验收。详见 docs/STAGE2B.md。

## 2026-09-09 LAN reliability pass

- 当前分支 main，HEAD e2129b5，工作树含未提交 Stage 2B readiness 加固及本轮文档。
- 本轮实际运行：linksend help/receive help、internal/transfer、internal/app、tests/integration、devtool network-info；Go 测试与网络信息检查通过。用户此前报告的 gofmt/go test/go vet/git diff --check 单独标注为既有证据。
- Windows 物理 LAN 候选为以太网 10.234.232.205/16；198.18.0.1/30 Mihomo 虚拟接口排除。macOS 双向实机重复待现场执行。详见 docs/LAN-VALIDATION.md。

## 2026-09-09 任务接口与 Wails UI

- `internal/app/tasks.go` 新增进程内任务编排：异步发送、接收等待、任务快照、单活动任务忙碌保护、取消请求与确认终态、接收确认/拒绝、失败发送重新发送。任务只存当前进程，未宣称重启恢复；原始失败任务保留，重试生成新本地 ID。
- `internal/app/direct.go` 增加本地阶段观察回调，继续复用既有 `SendFilesDetailed`/`ReceiveOnceDetailed` 真实 ICE、TLS、QUIC 和安全落盘路径。
- `apps/desktop/app.go` 增加薄 Wails 绑定、原生文件/目录选择器和环境变量驱动的 bind/STUN 配置；`frontend/src/App.tsx` 提供传输、设备、设置/诊断三个区域，任务操作连接真实后端快照。
- 自动化：`go test ./...`、`go vet ./...`、前端 `typecheck`/`lint`/`test`/`build` 通过；本轮 Wails 交互运行和 Windows/macOS 新版本双机验收尚未运行。
- 暂缓：跨 NAT、网络迁移、进程重启恢复、持久化历史、并发队列、复杂桌面交互和安装签名。

## 2026-09-09 Windows 初版产品化

- `apps/desktop/app.go` 增加本地偏好原子保存（服务地址、绑定地址、STUN、设备名、接收目录）、真实网卡枚举、加入设备组、完整指纹信任和安全打开接收目录的 Wails 桥接；环境变量仍优先覆盖本地设置，既有身份目录不变。
- `frontend/src/App.tsx` 重做为传输、设备、设置与诊断三页应用壳：发送端只显示等待确认，接收端才显示接受/拒绝；任务轮询与远端状态刷新分离；浏览器桥未注入时显示明确不可用提示。
- `frontend/src/App.css` 使用浅色语义 tokens、侧栏导航、状态条、分组表面、可见焦点和减少动效规则，完成 Playwright 传输/设备/设置三页截图检查。
- 自动化通过：桌面 `GOWORK=off go test ./...`、`go build ./...`、`go vet ./...`；前端 `typecheck`、`lint`、`test`、`build`；Wails 3 production build；`git diff --check`。
- 产物位于 `.artifacts/windows-preview/20260909-0405/`，含 `LinkSend.exe`、`BUILD-INFO.json`、`SHA256SUMS.txt`、中文启动说明、验证记录和浏览器渲染截图；ZIP 位于 `.artifacts/windows-preview/LinkSend-windows-preview-20260909-0405.zip`。
- Wails 原生窗口点击与截图、跨 NAT/IPv6、网络迁移和重启恢复仍未运行，均保持 `NOT_RUN` / `BLOCKED_BY_EXTERNAL_ENV` 边界。
- Windows 启动冒烟：运行 `apps/desktop/build/bin/LinkSend.exe` 后进程保持运行超过 3 秒并由本次检查结束；未进行人工点击和双机传输。

## 2026-09-09 Windows 初版复核修补

- 接收任务快照现在保留已认证 `peer_id`、manifest 内容摘要和条目数量；接收确认信息可在任务区核对发送者、内容和总量。
- `OpenTaskDirectory` 改为按已完成接收任务 ID 校验目录，避免前端传入任意本地路径；桌面接收等待时限固定为 10 分钟，ICE 检查时限为 30 秒。
- 前端任务/远端轮询加入 in-flight 防重叠；操作调用增加 pending 门闩；完成接收任务提供打开目录操作。
- 复核构建：根模块测试/桌面测试与 vet、前端 typecheck/lint/test/build、Wails 3 production build 均通过；交付目录已更新为最后一次构建的二进制和 SHA256。

## 2026-09-09 401 配对错误修复

- 通过当前桌面 profile 的真实身份与香港主站数据库只读比对确认：身份未加入设备组，服务器返回 401 是成员校验拒绝，不是签名算法或 TLS 故障。
- 前端新增后端错误人性化映射：区分“尚未加入设备组”“已加入但无管理员权限”“邀请无效/过期/已使用”，避免把安全拒绝显示成原始 HTTP 错误。
- 新增对应前端回归测试；隔离 profile、密钥和远程检查脚本均已清理。当前身份仍需由现有管理员设备生成一次性邀请后才能加入，客户端不会自动 bootstrap 或绕过管理员权限。

## 2026-09-09 隔离端到端与远端健康复核

- 在独立临时 rendezvous（`127.0.0.1:18887`、独立 SQLite/profile）上完成两个随机身份的 bootstrap、邀请、加入、双向完整指纹信任和 16 MiB 随机文件传输；发送端与接收端独立 SHA256 一致，TLS 1.3、QUIC、`relay=false`。临时 profile、密钥、数据库和大文件已在测试后清理，仅保留本记录摘要。
- 香港主站 `hk-main` 只读探测通过；`https://linksend.oooai.de/healthz` 返回 `status=ok`、`protocol_version=1`、`relay=false`、`transport=quic`；远端 443 由 rendezvous 监听，3478 由 coturn STUN-only 监听。
- 该隔离测试证明 CLI/核心真实链路，不等价于物理 LAN、跨 NAT 或 Wails 原生窗口验收。
- 生产 EXE 使用隔离 `LINKSEND_DATA_DIR` 启动冒烟保持运行超过 3 秒；WebView2 Runtime 152.0.4191.66 已安装。当前会话未进行人工窗口点击，桌面交互仍待现场验收。

## 2026-09-09 Windows Preview 首次配对与退出保护补齐

- `apps/desktop/app.go` 现在区分偏好不存在、有效、损坏、不支持版本和不可读状态；损坏/不支持文件原地保留，远端操作在用户保存修复前被阻止。新增 `PreferencesStatus`、`EffectiveConfig`，明确环境变量 > 已保存偏好 > 默认值及需重启生效边界。
- `internal/app/service.go` 新增基于已认证设备列表的成员/角色状态：`member/admin`、`not_member`、`auth_failed`、`unavailable` 分开表达；合并 401 不再被猜测为确定未入组。前端管理员邀请按钮按服务端角色禁用并解释原因，普通新设备主入口为粘贴邀请。
- `apps/desktop/main.go` 注册 Wails 3 `ShouldQuit`。存在准备、等待确认、连接、传输、校验或取消中的任务时，原生窗口提供继续任务/取消任务并退出；取消清理有界等待，超时保持窗口打开，重复关闭幂等。
- 新增桌面偏好恢复回归测试；根模块、桌面模块（`GOWORK=off`）、前端和 Wails Windows/amd64 production build 均通过，`git diff --check` 通过。
- 最终本地产物：`.artifacts/windows-preview/20260909-1312/` 及对应 ZIP。原生窗口人工点击、原生截图、真实主站管理员邀请和物理双机本轮仍为 `NOT_RUN`/`BLOCKED`，未以浏览器或隔离组结果替代。

## 2026-09-09 配对权限模型调整

- 根据产品决策，邀请生成不再要求管理员角色；任一已配对且未撤销设备都可以生成 32 字节高熵、10 分钟有效、一次性配对码。保留服务端签名认证、设备组隔离和逐对端完整指纹信任。
- 首次建立空设备组仍需使用服务端 bootstrap 入口；本轮没有把服务改成匿名开放注册。
- 更新服务端邀请授权、客户端错误文案、桌面按钮和 signaling 回归测试。管理员字段继续作为服务端事实展示，但不再作为生成配对码的前置条件。

## 2026-09-09 可用性与本地配对改进

- 新增 `docs/IMPROVEMENT-PROMPT.md`，记录全项目检查、配对边界、UI 视觉和验证要求，可作为后续迭代的固定改进提示词。
- `internal/server.Config.TestPairingCode` 默认关闭；本轮香港授权配置可在显式目标组上启用固定码 `orion123`，开发服务仍提供同一固定码。服务端仍要求 Ed25519 注册签名，生产高熵一次性邀请路径不变。
- `internal/store.JoinTestCode` 使用独立兼容路径加入明确设备组并授予管理员，并添加回归测试；公共配置缺少 `test_pairing_group` 时拒绝固定码配置。
- 桌面设备页展示“仅测试使用、未来可能关闭”的固定配对码并支持复制；输入框提示可直接输入 `orion123`。
- `apps/desktop/frontend/src/App.css` 增加 Apple 风格视觉层：系统字体、克制蓝色强调、半透明分组表面、统一圆角与焦点状态、窄屏导航折叠和减少动效规则。
- 本轮验证：根模块 `go test ./...` 通过；桌面模块 `GOWORK=off go test ./...`、`go build ./...`、`go vet ./...` 通过；前端 `pnpm run typecheck`、`pnpm run lint`、`pnpm run test -- --run`（4 tests）、`pnpm run build` 通过；`git diff --check` 通过。
- 未运行：Wails 原生窗口人工点击、Windows↔macOS 新版 UI 验收、跨 NAT/IPv6/网络切换。上述项目仍保持 `NOT_RUN` 边界。

## 2026-09-09 全面产品改进复核

- 增量实现：任务层把接收拒绝、源文件变化、完整性失败、危险路径、磁盘空间/权限和取消映射为稳定协议错误码；Wails 任务 DTO 与 CLI 失败出口只展示中文操作建议，底层 HTTP、文件系统和栈信息保留为内部 cause，不直接进入用户界面。
- 成员资格把信令不可达、版本不兼容和身份拒绝分开；任一已加入且未撤销设备可生成一次性邀请的前端文案与服务端权限保持一致。
- 固定配对码对同一 profile 重试为幂等并授予管理员；生产 `Join` 的高熵、10 分钟、一次性语义未改变，静态入口通过显式目标组控制。
- 直连完成后任务阶段显式进入 `connected`，界面把成员资格、对端 presence、指纹信任和已建立直连分别呈现。新增 favicon 清除浏览器预览的无意义 404。
- 当前 Windows/PowerShell 环境实际通过：`gofmt -l .`、`git diff --check`、根 `go test ./...`、`go vet ./...`、根 `GOWORK=off go test ./...`、根普通及 `GOWORK=off go test -race ./...`、桌面 `GOWORK=off go test/go vet/go build ./...`、前端 typecheck/lint/4 tests/build、Wails 3 production build、`demo-local` 和 Compose 配置解析。Wails 3 EXE 隔离启动后保持运行超过 4 秒，再由本轮检查结束。
- 浏览器行为/视觉检查：实际打开传输、设备、设置三页，并在 700px 窄屏检查导航折叠、单列布局、表单宽度、禁用态和复制入口；这是 WebView 内容渲染证据，不等于 Wails 原生窗口点击验收。
- `NOT_RUN` / `BLOCKED_BY_EXTERNAL_ENV`：本轮未运行新的 Windows↔macOS 双机传输、跨 NAT、IPv6、网络切换、睡眠唤醒、macOS Wails、Windows 原生窗口人工点击、Docker 引擎运行或安装签名。`devtool test-nat` 在 Windows 上按设计非零退出并说明需要 Linux 特权 namespace fixture。
- 下一步：在两端现场执行 Stage 2B 跨 NAT 与新版原生 UI 工作流；之后实现持久任务历史、真正的应用重启恢复和暂停/恢复编排。

## 2026-09-10 Wails 3 候选打包与交付核验

- 任务分支为 `codex/wails3-hk-dmg-20260909`，当前候选 commit 为
  `555c3190c4c5f13a52eebe9b9a8f6208abb7ee7d`；本轮提交了 DMG 资源、Windows/Mac 测试说明，
  未修改 `main`、未合并、未发布正式 Release。源码快照仍保留在 `.artifacts/pre-wails3-20260909-*`。
- GitHub Actions run [34387753173](https://github.com/Wen5555/LinkSend/actions/runs/34387753173) 的
  `windows-amd64`、`macos-arm64`、`macos-amd64` 均 `success`。Windows ZIP 和两个 DMG 已下载到
  `.artifacts/ci-34387753173/` 并通过 ZIP 完整性、包内 `SHA256SUMS.txt`、DMG HFS+ 目录、
  `Info.plist`、`CFBundleIdentifier`、Go `GOARCH/GOOS` 与 Wails 依赖核验。
- 最终包 SHA256：Windows ZIP
  `1c43b7576118c5480d139bd691a1c84b8c3d09f67d96f8815c1877c4055f8f80`；macOS arm64 DMG
  `5ff0217fab3b257999eae710f29bc76f08979b37100b2f1b4347546a6d3645a2`；macOS amd64 DMG
  `653c2ceb098c5d81bc4c6f0a351cb20b446772c206888463aec7b34efff73819`。DMG 为内部测试包，
  仅 ad-hoc 签名，不含 Developer ID 或公证。
- Windows ZIP 现在包含真实 `LinkSend.exe` 与 `README-WINDOWS-TEST.txt`；DMG 包含完整
  `LinkSend.app`、`README-MACOS-TEST.txt` 和 Applications 拖拽入口。离线 `go version -m` 确认
  Go 1.26.5、`GOOS=darwin`、`GOARCH=arm64/amd64` 及 Wails `v3.0.0-beta.18`。
- 当前主机实际完成了 Windows 与香港服务的既有真实双向 QUIC 证据；没有新增 Mac 原生窗口、DMG
  挂载、红点/Cmd+Q、跨 NAT、IPv6 或网络切换验收，均标记 `NOT_RUN`/`BLOCKED_BY_EXTERNAL_ENV`。

## 2026-09-11 任务诊断文案与阶段可读性补丁

- 前端任务列表将内部阶段枚举映射为中文可操作文案（发现网络地址、交换连接信息、检查直连路径、建立安全直连、等待接收确认、恢复传输等）；未知未来阶段仍原样保留，便于诊断。
- `humanizeBackendError` 补齐稳定连接、传输、任务操作和桌面边界错误码（包括 `NO_CANDIDATES`、`CHECK_TIMEOUT`、`DISK_FULL`、`UNSAFE_PATH`、`RELAY_NOT_IMPLEMENTED`、`TASK_NOT_RETRYABLE` 等），避免将原始内部错误直接展示给用户。
- 修复测试配对面板 CSS 伪元素重复显示“仅测试使用”文案，并补充阶段与错误码前端回归测试。
- 实际验证：前端 `pnpm run typecheck`、`pnpm run lint`、`pnpm run test -- --run`（6 tests）、`pnpm run build`；根模块 `gofmt -l .`、`git diff --check`、`go vet ./...`、`GOWORK=off go test ./...`；桌面模块 `GOWORK=off go test ./...`、`go vet ./...`、`go build ./...`；Wails `v3.0.0-beta.18` production Windows build 均通过。
- `go run ./cmd/devtool demo-local` 实际完成 loopback TLS 1.3/QUIC 文件传输与摘要一致性；该结果仅证明本机链路。Windows↔macOS、跨 NAT、IPv6、网络切换、睡眠唤醒和原生窗口人工点击仍为 `NOT_RUN` / `BLOCKED_BY_EXTERNAL_ENV`。
- Wails 最新构建产物 `apps/desktop/bin/LinkSend.exe` 隔离数据目录启动冒烟保持运行超过 4 秒后结束；未进行原生窗口人工点击。

## 2026-09-11 直连路径证据贯通任务界面

- `DirectConfig` 新增本地 evidence 回调，连接成功后把实际 `connection_method`、`transport_protocol` 和 `relay` 结果写入任务快照；未改变协议、ICE、QUIC 或文件数据路径。
- Wails/React 任务列表显示已验证的“局域网直连 / 互联网 P2P / 路径待确认”，避免把在线状态或 host candidate 当作局域网证据。
- 实际验证：根与桌面模块测试、桌面 vet/build、前端 typecheck/lint/6 tests/build、绑定重新生成和 `git diff --check` 均通过。跨设备、跨 NAT、IPv6、网络切换和原生窗口仍未运行。

## 2026-09-11 任务历史与候选构建复核

- `internal/app` 增加 `task-history.sqlite`（SQLite `user_version=1`，含 `schema_version` 等价版本元数据）持久化；启动时损坏记录被隔离，不阻塞应用；未完成任务在重启后标记为 `failed/TASK_INTERRUPTED`，清除旧会话证据并要求用户核对后重新发起；块检查点仍仅由 transfer 层使用，桌面级 byte resume 尚未实现。快照新增单调 `revision`、`history_persisted`、`restart_recovery_supported`、`byte_resume_supported` 与暂停/恢复能力字段。
- 根模块 `go test ./...`、`go test -race ./...`、桌面模块 `GOWORK=off go test/go vet/go build ./...`、前端 frozen install/typecheck/lint/Vitest 6 项/production build 均通过；新增历史持久化和损坏记录回归测试。
- 提交 `9b72334d0fbe5e18649424924e96cf88756c6845` 已推送分支 `codex/wails3-hk-dmg-20260909`。Actions run [34515881198](https://github.com/Wen5555/LinkSend/actions/runs/34513870167) 的 Windows amd64、macOS arm64、macOS amd64 三平台均 success，产物与该提交对应。
- 本机可交付 Windows 便携 ZIP：`apps/desktop/bin/LinkSend-windows-amd64-0.1.0-preview.zip`，包含 EXE、测试说明、BUILD-INFO 和包内 SHA256。NSIS/Wails CLI 未安装，安装器未生成；macOS DMG 仅通过 CI 产出，未在本机挂载或进行原生窗口点击验收。

## 2026-09-11 全面网络提示词执行记录

- 香港主站 `hk-main` 经 codex-ssh-manager 完成只读 `resolve/probe/audit-host`：root、Debian 6.1.0-50-cloud-amd64、x86_64，根分区余量约 78%，`rebootRequired=false`；未修改生产配置。
- 荷兰 VPS `nl-highdefense` 的 SSH 管理器探测在本轮超时，未取得 STUN/TURN 或 NAT 实验运行证据，标记 `BLOCKED_BY_EXTERNAL_ENV`，未执行 sudo 或网络配置变更。
- `go run ./cmd/devtool demo-local` 退出码 0：真实 ICE host candidate、QUIC TLS 1.3、摘要一致（1 MiB），仅作为 loopback 证据，不能替代双机 LAN。
- `go run ./cmd/devtool test-nat` 退出码 1，输出明确为 Windows 缺少受控 Linux namespace/NAT fixture；标记 `BLOCKED_BY_EXTERNAL_ENV`，没有伪造 NAT 成功。
- GitHub Actions run [34523034011](https://github.com/Wen5555/LinkSend/actions/runs/34523034011) 对主分支提交 `a9646f21c3d088036ade6d7f0a44cd0cb4a75c9a` 的 Windows amd64、macOS arm64、macOS amd64 jobs 均 `success`；对应构建附件可从该 run 下载。尚未替换旧候选 release `v0.1.0-preview-e27`（其目标提交不是当前主分支），避免上传不匹配资产。

## 2026-09-11 Windows ↔ Mac 实机联调与修复（阶段性历史记录）

- 本节保留暂停/恢复实现之前的现场事实；当前实现和构建状态以本文顶部“P0/P1/P2 分阶段可靠性改进”为准，不应把本节当作最终能力声明。环境：Windows 物理以太网 `10.234.232.205/16` ↔ Mac 物理 Wi-Fi `10.234.241.3/16`；两端原有 TUN 保持启用。SSH 全部使用 codex-ssh-manager 的 `mac-test-102342413`。完整状态、命令和限制见 [本轮验收与复测步骤](MAC-WINDOWS-VALIDATION-20260911.md)。
- `IMPLEMENTED / PASS`：真实双向 QUIC/TLS 1.3 传输空文件、中文文件、多文件、12 MiB（三 chunk）文件及中文空目录，接收文件 SHA256 匹配；Mac 真实拒绝、权限拒绝、冲突、独立 64 MiB 卷磁盘不足均保留稳定错误分类，权限恢复和实验卷卸载已核验。
- `IMPLEMENTED / PASS`：修复候选诊断泄露 ICE ufrag；修复 QUIC 终态响应因立即 reset 而丢失；冲突/权限/磁盘错误在接收端、发送端、CLI/任务 DTO 和前端一致；移除仅凭 srflx 推断互联网路径的逻辑。前端分别显示历史持久化、重启恢复和字节级续传，移除常驻固定测试配对码。
- 信令协商清理为 `IMPLEMENTED / PASS`（本地/原生 Go 回归），香港现有旧服务短时间重试仍为 `FAIL`。本轮没有部署生产服务；不能以源码修复替代线上验收。
- `IMPLEMENTED / PASS`：Windows 根模块普通/race/vet/独立模块测试，桌面独立模块 test/vet/build，前端 frozen install/typecheck/lint/7 tests/build；Mac Go 1.26.5 根模块 test/race、桌面 test/vet；两端 Wails beta.18 bindings 及主要前端 JS SHA256 一致。
- 原生构建：Windows production EXE、Mac arm64/amd64 DMG 均实际构建；Mac `hdiutil`、`plutil`、`file`、`lipo`、`codesign` 校验通过。隔离 arm64 应用由 CoreGraphics 观察到 1 个真实窗口，未进行原生控件操作。修复 Windows 源码拷贝到 Mac 后的 CRLF shell/HTML 问题，加入 `.gitattributes`。
- 本轮包标记 `UNCOMMITTED_TEST_SNAPSHOT`，基线 `fef3e3f59797a6de25cb7f9b1f2a1850512808d5`，不冒充该提交的干净构建。Windows 为 `UNSIGNED_TEST_BUILD`；Mac `code_signature=ADHOC / distribution_identity=NONE / notarization=NOT_RUN`。本轮无新 PR/合并/Release。
- 该现场阶段当时 `NOT_IMPLEMENTED`：桌面重启恢复、字节级续传、桌面暂停/恢复；这些能力随后已在本文顶部所述源码快照中实现并通过本地真实 QUIC 回归，但尚未做新的物理双机恢复验收。中继仍为 `NOT_IMPLEMENTED`；真实双 NAT、Linux 双机、完整网卡矩阵、原生 UI 缺口仍见验收报告。
- 历史交付勘误：较早的 Windows 便携包曾缺包内 SHA256/准确合并提交来源信息，不能视为满足本轮发布要求；本轮 ZIP 单独记录未提交来源并包含包内校验和。默认 Windows 身份数据目录通常为 `%APPDATA%\LinkSend`（`os.UserConfigDir()`），不是 `%LOCALAPPDATA%`。
