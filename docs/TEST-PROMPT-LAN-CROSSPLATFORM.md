# Codex 全面网络与交付验收提示词

以下提示词用于后续 Codex 任务。执行者必须先读取根目录 `AGENTS.md`、`docs/SPEC.md`、`docs/ARCHITECTURE.md`、`docs/PROTOCOL.md`、`docs/SECURITY.md`、`docs/TESTING.md`、`docs/ACCEPTANCE.md` 和 `docs/PROGRESS.md`，再开始测试。它们是可审计的执行要求，不是测试结果。

## 总提示词

```text
你是 LinkSend/Osend 的网络、桌面和发布验收负责人。请在现有仓库中连续执行“环境盘点 → 隔离实验 → 故障注入 → 架构修复 → 回归 → 打包 → 报告”，不要另建演示项目，不要伪造网络、签名、公证或窗口结果。

目标：证明或否定以下事实，并为每一项给出 PASS、FAIL、NOT_RUN 或 BLOCKED_BY_EXTERNAL_ENV：
1. 两个 Windows amd64 客户端在同一物理局域网可通过真实 ICE + QUIC 直连并完成双向传输。
2. Windows 在存在干扰网卡时仍能选择正确候选，不能把 VPN、TUN、Hyper-V、Docker、WSL、loopback 当作物理 LAN 证据。
3. Windows 与 Linux 在同一局域网可完成相同的身份认证、接收确认、完整性校验和安全提交。
4. Windows/macOS/Linux 的失败阶段、错误码、取消、重试、冲突和退出保护一致。
5. 香港主站只承担信令/在线状态，荷兰 VPS 只提供经审计的 STUN/实验服务；文件正文绝不能经过信令、JavaScript IPC、HTTP 上传或第三方中继。
6. 应用重启后能正确识别中断任务；只有在真正实现重新认证、manifest/staging 校验和缺块协商后，才允许声称恢复或字节级续传。

硬性边界：保留 Pion ICE 和 quic-go，不写自定义 ICE/TCP/HTTP fallback；固定测试配对码只能在显式隔离配置出现；不修改主机生产防火墙、路由、DNS 或代理。允许 SSH 到已授权的香港主站和荷兰 VPS，但所有 sudo 命令必须先打印、限定资源、记录回滚命令，并只修改实验专用服务配置。

每次运行先记录：git commit、工作树 dirty 状态、OS/arch、Go/Node/pnpm/Wails、网卡及地址、路由、DNS、WebView2、服务 URL、STUN URL、监听端口、数据目录和权限。为每个客户端建立独立 profile、identity、receive root、端口和日志目录；测试结束清理自己创建的资源，保留证据文件。

每个网络断言必须同时保存：本地/远端 candidate pair、candidate type、generation、实际 UDP base socket、接口名和地址族、ICE 状态变化、QUIC TLS/ALPN、对端指纹、relay 配置与实际使用、信令/STUN 字节计数、传输摘要、接收端提交摘要和完成确认。loopback、同网段、host candidate、presence 在线都不能单独证明 LAN。

发现 FAIL 后先最小化复现，再修复源码/测试/文档；禁止删除断言、放宽超时、跳过 race、把代码错误归为环境阻塞。修复后重新运行受影响场景、全部根模块测试、桌面 GOWORK=off 测试、前端测试和 race。只有新测试和旧测试都通过，才进入打包。
```

## Windows 同 LAN 与干扰网卡

```text
准备两台真实 Windows 10/11 amd64，分别使用独立 LinkSend profile。优先用真实 Wi-Fi 或以太网交换机；不要用同一进程、localhost 或同一虚拟网桥代替双机。按以下矩阵逐项执行：

- 仅物理 Wi-Fi；仅物理以太网；一端 Wi-Fi、一端以太网。
- 同时启用 Docker Desktop、WSL vEthernet、Hyper-V、Tailscale/WireGuard 或其他 VPN/TUN；记录每块网卡的 ifIndex、地址、metric、是否 loopback、是否虚拟。
- 物理网卡地址与 VPN 地址使用重叠私有前缀；验证选择结果来自真实 ICE nomination，而不是前缀猜测。
- 临时断开/恢复干扰网卡、改变网卡 metric、续租 DHCP；验证旧 generation 失效、重新 gather/check，旧回调不会覆盖新 revision。
- IPv4；若两台机器具备可路由 IPv6，再单独验证 IPv6；明确拒绝不支持的 IPv6 link-local scope。

每个场景发送：空文件、中文长文件名、多文件、普通目录、空目录、冲突目标和至少一个大于两个 chunk 的文件。接收端必须先看到摘要并明确接受，拒绝时不得写入正文。比较发送端“已读/已发送”与接收端“已校验/已提交/已确认”计数，确认完成摘要一致。
```

## Windows 与 Linux 同 LAN

```text
使用一台真实 Windows 和一台真实 Linux（物理机或已授权虚拟机，但必须记录虚拟化边界），两端使用不同 identity/profile。信令可经香港主站，文件必须走认证 QUIC。

执行：双向发送、中文路径、空目录、多文件、目标已存在、目标目录不可写、源文件传输中修改、接收端拒绝、发送端取消、重复取消、暂停后取消、连接建立后信令断开。验证健康 QUIC 不因信令短暂不可达而被错误判定断开。

在 Linux 端用受控故障注入模拟磁盘不足/权限拒绝；标记为故障注入，不冒充真实现场。保存双方日志脱敏副本，确认不含私钥、ICE 凭据、邀请令牌、文件正文和敏感绝对路径。
```

## 香港主站、荷兰 VPS 与双 NAT

```text
通过 codex-ssh-manager 连接已授权主机。先执行只读审计：OS、架构、服务状态、监听端口、Docker、Caddy、coturn/STUN 配置、磁盘和时间同步。sudo 只用于实验服务的最小变更；先备份配置并写回滚命令。

验证：
- 香港主站 WSS/HTTPS 信令健康、设备组隔离、邀请一次性和撤销。
- 荷兰 VPS STUN-only 响应、无 TURN allocation、无文件 HTTP API；记录 STUN 请求/响应计数。
- 两个隔离网络命名空间/容器各自位于独立 NAT 后，经公共网络连接；不得把同一 Docker bridge 当成双 NAT。
- 可达映射、UDP 阻断、STUN 不可达、映射变化、重连、重叠私网和超时失败逐项验证。
- 失败必须显示候选/ICE/QUIC 阶段和稳定错误码 `NO_CANDIDATES`、`CHECK_TIMEOUT`、`NO_VIABLE_CANDIDATE`、`RELAY_NOT_IMPLEMENTED` 等；不能猜测“对称 NAT”或“防火墙”。

抓包或服务计数证明大文件正文没有经过 WSS、香港主站、荷兰 STUN 或其他中继。实验完成后恢复配置并确认生产服务仍处于原状态。
```

## 进程退出、恢复与安全落盘

```text
对接收端在 AwaitingAcceptance、Transferring、Verifying、提交前后分别执行正常退出和强制终止。重启后验证：历史仍可读；旧 session/ICE 凭据不复用；中断任务被识别并要求用户明确操作；staging、checkpoint、源文件 manifest、目标冲突、对端指纹变化和摘要变化均有独立结果。

只有实际实现并测试了重新认证、manifest 重新协商、staging 块重校验和缺块请求，才把 byte_resume_supported 置为 true。记录实际重传字节数，完整重发必须显示为完整重发。

验证路径穿越、绝对路径、UNC/ADS、保留名称、大小写冲突、符号链接/重解析点、特殊文件、超长路径、中文路径、同名冲突和提交 TOCTOU。目标冲突默认不覆盖；校验失败不得产生 Completed。
```

## 原生桌面与发布门禁

```text
在真实 Wails Windows 窗口验证高 DPI、720px 以下窄屏、深色/高对比度、键盘 Tab/Enter/Esc、窗口关闭、活动任务退出保护和原生文件选择器。浏览器截图只能作为辅助证据，不能代替原生窗口。

在 macOS arm64 和 amd64 runner 分别执行 plutil、file/lipo、codesign、hdiutil，检查 Info.plist、Bundle ID、最低系统版本、嵌套二进制架构、可执行权限、绝对路径引用和 ADHOC 签名。没有 Developer ID/公证时必须标记 ADHOC、NONE、NOT_RUN。

同一 commit 生成 Windows ZIP、Windows 安装器（若 NSIS/WebView2 条件满足）和两种 macOS DMG。每个包重新计算 SHA256，BUILD-INFO 写明 commit、dirty、OS/arch、工具版本、签名和验证状态。安装器要在隔离用户目录测试安装、运行、卸载和用户数据保留；缺少 NSIS/WebView2 时记录 BLOCKED_BY_EXTERNAL_ENV，不制造占位安装器。
```

## 最终报告格式

```text
一、环境与拓扑：机器、网卡、NAT、主站/VPS、工具版本、权限边界。
二、测试矩阵：每个案例的命令、退出码、PASS/FAIL/NOT_RUN/BLOCKED、证据路径。
三、缺陷与修复：触发条件、根因、最小改动、回滚方式、回归测试。
四、协议/安全影响：候选、身份、QUIC、文件数据路径、错误码和持久化 schema。
五、交付：commit、workflow run、产物文件名/大小/SHA256、签名/公证、下载链接。
六、剩余限制：明确哪些能力仍未实现，不能使用“全面完成”“生产可用”“跨平台已验收”。
```
