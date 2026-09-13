# E0-02 当前网络与服务基线

2026-09-13。状态 PARTIAL。只读主机审计与香港控制面已执行，物理桌面双向文件矩阵尚未完成。
全部远程连接经 codex-ssh-manager；没有改主机防火墙、路由、代理或无关服务。

| 目标 | 当前事实 | 结果 |
|---|---|---|
| Windows | 以太网17：10.234.232.205/16；Mihomo16：198.18.0.1/30；其余多块169.254地址，IPv6主要link-local | Get-NetIPAddress/Get-NetRoute/Get-NetTCPConnection退出0 |
| Mac alias mac-test-102342413 | wen@10.234.39.151，macOS26.5 build25F71 ARM64，en0校园网、多个utun | resolve/probe/audit及ifconfig/netstat/lsof退出0 |
| hk-main | root@109.66.88.204:60851，Debian，LinkSend PID399706 | probe/audit、服务hash、443/3478监听和公网health退出0 |
| nl-highdefense | root@157.254.234.182:22，Ubuntu26.04/KVM；eth0实际100.102.1.116/16，默认网关100.102.102.102；有现有Docker bridge | probe/audit与拓扑检查退出0，不按alias断言地理位置 |
| nl隔离可行性 | unshare --user --map-root-user --net，内部仅lo DOWN、无路由 | 退出0；有ip/iptables/nft/docker/tcpdump；尚无Go/coturn；不是双NAT传输结果 |

Mac地址更新的manager add错误及inventory最小修正见准备记录；apply后真正连接新地址成功。
nl audit的1个failed unit/rebootRequired=true只记录，不处理无关业务。

## 香港三层状态分列

现役二进制SHA256 `0923d9dfca11028859449eee7bc00026ca3e2616d5ed32411a3b42f216bd318c`，与M5记录一致。未重启/部署。
HTTPS health返回产品0.5.0、协议1、QUIC、relay=false。
2026-09-13T02:51附近，确认Windows没有LinkSend GUI进程后，在隔离目录临时复制受DPAPI保护的既有身份文件，运行control-inspect；退出后删掉临时identity.key，原profile不变，没有新建/撤销成员或发送文件。

| 阶段 | 结果 |
|---|---|
| HTTPS health | PASS |
| WSS challenge/response真实认证 | PASS，自身ID前缀c6a001eba9f0 |
| 认证后Devices快照 | PASS，11条当前有效成员，2条online（含当前诊断连接）；不是所有设备在线 |
| 双向P2P文件 | NOT RUN |

实际命令：`go run ./scripts/desktop-experience/control-inspect --profile <isolated-control-profile> --server https://linksend.oooai.de`，退出0。
日志 `.artifacts/desktop-experience/e0/control-inspect.jsonl`。工具WSS会替换同身份连接，因此不得与同身份GUI同时运行。
presence在线不证明对端LAN控制或QUIC可达。

## durable job 与断连恢复

| 作业 | 退出码/证据 |
|---|---|
| /tmp/codex-ssh/linksend-e0-inspect-mac-20260912T181200Z | 0；manager resume wrapper因zsh只读status报错，download原exit-code/stdout/stderr确认；未重跑原任务 |
| /tmp/codex-ssh/linksend-e0-inspect-hk-20260912T181200Z | 0 |
| /tmp/codex-ssh/linksend-e0-inspect-nl-20260912T181200Z | 0 |

本机原始记录位于 `.artifacts/desktop-experience/e0/`。导出只保留与LinkSend有关的状态，完整只读监听包含无关业务，保持ignored，不公开上传。

## 仍需执行

准确Windows/Mac包在隔离profile的发现→配对→LAN控制→Windows发送→Mac发送→各自hash；NIC/路由/监听必须与该次测试时间对应。
同Wi-Fi、混合以太网/Wi-Fi、热点、DHCP、睡眠、VPN/TUN切换等逐项NOT RUN。
历史Mac→Windows no route to host、MASQUERADE-only CHECK_TIMEOUT是历史证据，未作为当前根因或本轮结果。
隔离双NAT需独立NAT A/B、无私网旁路、真实STUN/ICE/QUIC/counters/hash，namespace能力检查不等于该验收。
