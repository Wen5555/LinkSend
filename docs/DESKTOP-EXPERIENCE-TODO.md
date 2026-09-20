# LinkSend 桌面体验全轮 TODO

日期：2026-09-13。唯一产品写入方：执行任务 `/root/desktop_executor_terra`（`gpt-5.6-terra` / `xhigh`；旧执行者已停止且不得并发恢复）。
总控：`01a096c4-2e59-7db3-a7a8-ec2a125409d6`（local）。工作分支：`codex/desktop-experience-upgrade`。
基线：`3bcb73019d1ac6de1d9f341bb7d22e2813875081`。E4 自动剪贴板源码范围已由总控在 `3363fd5+387b57c` 验收，当前执行 E5 准确包、物理双机、恢复、部署与网络联合验收。

依据：[用户执行提示词](prompts/DESKTOP-EXPERIENCE-GOAL.md)、[完整方案](DESKTOP-EXPERIENCE-IMPROVEMENT-PLAN.md)。
七项均为必交付：U1 一次接收；U2 删除与配对；U3 发现可靠性；U4 移除手动内容并自动剪贴板；U5 设置；U6 真正系统共享；U7 有界四视图。

## 生命周期与证据规则

生命周期为：待办 → 执行中 → 待审查 → 验收通过 → 阶段已同步；外部阻塞单列原因与可继续工作。
只有总控可以判定验收通过。提交、测试通过、原生通过、网络通过、GitHub 同步互不替代。
各验证列使用 NOT RUN / PASS / FAIL / PARTIAL，非适用项须解释 N/A；未运行一律 NOT RUN。
每份 evidence 记录精确提交/dirty 状态、包 SHA256、OS/架构、真实拓扑、步骤、实际命令/退出码、日志、失败/限制和下一步。
包级结果不能继承源码测试快照；loopback、指定地址协议探针、真实桌面与双 NAT 分开记录。

## 稳定验收项

| ID | U 要求 | 阶段 | 依赖 | 改动范围 | 验收条件 | Evidence 路径（相对 docs） |
|---|---|---|---|---|---|---|
| E0-01 | U1–U7 | E0 | 无 | 准确包、源码、工具链与复现矩阵 | 固定 M5 包与新源码提交及 SHA256；七项逐条复现，严格区分源码审阅与准确安装包结果 | evidence/DESKTOP-E0-BASELINE.md |
| E0-02 | U2/U3 | E0 | E0-01 来源核对 | Windows/Mac 只读网络证据、香港与荷兰审计 | 当前接口/地址/路由/监听；发现→配对→控制→双向发送→hash；香港 health/认证/在线列表分列；荷兰真实拓扑及隔离双 NAT 可行性，指定地址探针不等于桌面通过 | evidence/DESKTOP-E0-NETWORK.md |
| E0-03 | U6 | E0 | E0-01 来源核对 | Win11 Share Target 包级最小原型 | 真实 ShareOperation/StorageItems、身份/签名、冷启动/交接、源权限及安装卸载回执；开发自签仅证明原型 | evidence/DESKTOP-E0-WINDOWS-SHARE-PROTOTYPE.md |
| E0-04 | U6 | E0 | E0-01 来源核对 | Mac Share Extension .appex 包级最小原型 | 真实 App Group/entitlements、文件表示期限、后台冷启动及扩展退出后可读；明确实际凭据/系统授权缺口 | evidence/DESKTOP-E0-MAC-SHARE-PROTOTYPE.md |
| E0-05 | U2/U3/U4/支撑 | E0 | E0-01；结合 E0-02/03/04 证据 | ADR、PROTOCOL、SECURITY、SPEC、产品范围 | 冻结配对/撤销/同步作用范围、成员版本及旧端拒绝、LAN 同意事务、剪贴板权限/冲突/TTL 与会话调度；总控审查 | evidence/DESKTOP-E0-DECISIONS.md |
| E1-01 | U2 | E1 | E0-05 | 配对服务、控制库、身份与桌面流程 | 有效码直接配对、首台初始化、幂等与明确跨组切换；普通成员仅组内权限；重放/跨组负测 | evidence/DESKTOP-E1-PAIRING.md |
| E1-02 | U2 | E1 | E1-01 | 撤销/设备删除、连接、重试、同步 | 先持久撤销全部既有组/LAN grant，再关闭权限/连接/重试；幂等组删除、离线待同步；重启/旧快照/WSS/LAN/旧控制会话不复活；新配对不恢复旧LAN/免确认/剪贴板；保留文件历史 | evidence/DESKTOP-E1-REVOCATION.md |
| E1-03 | U2 | E1 | E0-05 | LAN 独立添加/同意事务 | 被连接端确认；拒绝/超时无残留，nonce/身份绑定、双向幂等；服务失败状态真实 | evidence/DESKTOP-E1-LAN-PAIRING.md |
| E1-04 | U3 | E1 | E0-02 | 发现 providers、地址/接口/回程与缓存 | 多接口多地址、独立降级、明确源/回程、记忆地址退避、容量/过期/socket 重建；核实 LocalSend 一手来源并记录双栈/mDNS 选择 | evidence/DESKTOP-E1-DISCOVERY.md |
| E1-05 | U2/U3 | E1 | E1-01/02/03/04 | 统一目录、状态、网络/睡眠恢复 | 事实状态；健康 QUIC 不随 WSS/发现重启取消；准确包真实双向矩阵 | evidence/DESKTOP-E1-DIRECTORY-RECOVERY.md |
| E2-01 | U1 | E2 | E1-01/02/03/04/05 | Go 默认接收命令、接收计划与 UI | 原子默认接收、默认保留两份、一次点击；代际/撤销/空间/安全落盘；恢复原计划；免确认保存失败独立反馈 | evidence/DESKTOP-E2-DEFAULT-RECEIVE.md |
| E2-02 | U5 | E2 | E0-05；E1 授权模型 | 设置 API、设备覆盖与分类 UI | 分类设置、每设备继承、revision 局部更新、统一保存反馈；并发不丢字段、不打断健康任务 | evidence/DESKTOP-E2-SETTINGS.md |
| E2-03 | U4 | E2 | E0-05 | 旧内容入口、迁移、历史管理 | 移除手动文字/链接/图片创作；旧草稿/队列显式处理且不自动续发；历史/收到文件/快照可管理 | evidence/DESKTOP-E2-LEGACY-CONTENT.md |
| E2-04 | U7 | E2 | E2-01/02/03 | 传输/记录/设备/设置四视图、有界列表 | 首屏选文件/目标/发送；1100×720、960×640、125%/150%、中文长名、深色、键盘验证 | evidence/DESKTOP-E2-LAYOUT.md |
| E3-01 | U6 | E3 | E0-03；E1-01/02/03/04/05 | Windows 正式共享适配器与设备面板 | 真实可用设备；当前用户 IPC、持久去重、文件授权、独立共享请求 | evidence/DESKTOP-E3-WINDOWS-SHARE.md |
| E3-02 | U6 | E3 | E0-04；E1-01/02/03/04/05 | Mac 正式扩展与设备面板 | App Group/进程身份、临时表示接管/bookmark、后台唯一 owner、真实设备 | evidence/DESKTOP-E3-MAC-SHARE.md |
| E3-03 | U6 | E3 | E3-01/02 | 双平台准确包与安装生命周期 | 准确身份与签名/权限；冷启动/后台/主窗打开/连续共享/扩展退出继续发送；安装升级卸载 | evidence/DESKTOP-E3-NATIVE-PACKAGES.md |
| E4-01 | U4/支撑 | E4 | E0-05 | profile 存储所有权、会话与调度 | 先测量再收敛；提交屏障、认证会话复用、流级取消、有界文件和剪贴板调度 | evidence/DESKTOP-E4-STORAGE-SESSIONS.md |
| E4-02 | U4 | E4 | E4-01 | 权限、原生监听/写入、能力协商 | 默认关闭；按设备/方向/类型双端授权；Windows AddClipboardFormatListener、Mac changeCount；正文不经 JS/WSS | evidence/DESKTOP-E4-CLIPBOARD.md |
| E4-03 | U4 | E4 | E4-02 | 自动同步状态机、资源与托盘 | 文本/链接/图片、防回环；origin sequence与Lamport分离、串行落板revision门闩；预签发lease拒绝首帧/整图/重传晚到，续期不续旧事件、重连旧lease无效；快复制/离线不补发/锁屏/撤销/限额/暂存回收/托盘暂停 | evidence/DESKTOP-E4-CLIPBOARD-BEHAVIOR.md |
| E5-01 | U1–U7 | E5 | E1–E4 全部待总控验收完成 | 同提交候选联合验收 | 七项联合、文件/恢复/异常、旧版升级/混合版本、网络与原生实机矩阵；准确包 hash/来源/OS | evidence/DESKTOP-E5-ACCEPTANCE.md |
| E5-02 | U1–U7 | E5 | E5-01 | 工程文档、PR/CI、产物与部署回执 | 最终工作分支 PR/CI、包来源、部署/备份/回滚证据齐全；只有总控判定全部达标后结束 Goal | evidence/DESKTOP-E5-DELIVERY.md |

## 独立状态账本

E0 准备记录：[DESKTOP-E0-PREPARATION](evidence/DESKTOP-E0-PREPARATION.md)。正式 E0 已派发；各验证结果按实际完成更新。

| ID | 生命周期 | 代码 | 自动测试 | 原生 | 网络 | GitHub | 总控判定/阻塞 |
|---|---|---|---|---|---|---|---|
| E0-01 | 阶段已同步 | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PASS | draft PR #8 / 1518e31，core/desktop/三平台包CI全PASS；见分项记录 |
| E0-02 | 执行中 | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PASS | 已有网络证据复用，真实双向与双 NAT 缺项不改写 |
| E0-03 | 外部阻塞 | PARTIAL | PARTIAL | PARTIAL | NOT RUN | NOT RUN | 构建/签名PASS、系统安装FAIL、激活NOT RUN；不降格U6 |
| E0-04 | 待审查 | PARTIAL | PARTIAL | PARTIAL | NOT RUN | 执行中 | 总控接受2342009原型IO修补；服务4097与文件交接仍未通过，见MAC-SAFEIO证据 |
| E0-05 | 验收通过 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 仅ADR语义：总控E0-safeio-close-v1接受11c44df；具体API/schema/实现及兼容测试仍待E1/E4 |

| E1-01 | 源码验收通过 | PASS | PASS | NOT RUN | NOT RUN | PASS | 总控已接受 `666cbaa` E1 后端范围；准确包配对 UI/物理矩阵留 E5 |
| E1-02 | 源码验收通过 | PASS | PASS | NOT RUN | NOT RUN | PASS | 总控已接受 `666cbaa`；准确包离线 outbox/重启同步仍未实机 |
| E1-03 | 源码验收通过 | PASS | PASS | NOT RUN | PARTIAL | PASS | LAN 同意/恢复源码已接受；旧物理 LAN 仅 PARTIAL，准确 package 系统确认留 E5 |
| E1-04 | 部分验收通过 | PARTIAL | PASS | NOT RUN | PARTIAL | PASS | IPv4 主路径源码已接受；E5-D test-only 双栈/mDNS 原型已通过但生产 provider 未启用，物理多网卡/准确包IPv6/睡眠矩阵未闭合 |
| E1-05 | 源码验收通过 | PASS | PASS | 源码构建PASS/准确包NOT RUN | NOT RUN | PASS | 事实目录、原生事件与5秒快照已接线；准确包事件延迟、网络/睡眠矩阵未闭合 |
| E2-01 | 验收通过 | PASS | PASS | 浏览器PASS/准确包NOT RUN | 复用E1 | PASS | 默认计划读取设备覆盖/全局同名策略；普通/免确认一致，恢复计划不变；准确包联合验收留E5 |
| E2-02 | 验收通过 | PASS | PASS | 浏览器PASS/准确包NOT RUN | NOT RUN | PASS | schema7设备策略继承、全局策略、字段级dirty合并、deferred-save和CAS重试已验收 |
| E2-03 | 源码验收通过 | PASS | PASS | 浏览器PASS/准确包NOT RUN | NOT RUN | PASS | 旧草稿可见/取消、队列直达且不自动续发；E4 自动剪贴板源码已接受，物理同步留 E5 |
| E2-04 | 验收通过 | PASS | PASS | 浏览器PASS/准确包NOT RUN | NOT RUN | PASS | 完整外壳960×640主按钮可见可点；准确包原生DPI/键盘与双机UI留E5 |
| E3-01 | 源码验收通过 | PASS | PASS | 源码构建PASS/系统激活NOT RUN | NOT RUN | PASS | `c205b12` 源码范围已接受；签名/包身份系统激活按用户暂缓 |
| E3-02 | 源码验收通过 | PASS | PASS | arm64源码构建PASS/签名激活NOT RUN | NOT RUN | PASS | `c205b12` 源码范围已接受；签名/App Group 系统激活按用户暂缓 |
| E3-03 | 源码验收通过 | PARTIAL | PASS | 构建材料PASS/准确包系统生命周期NOT RUN | NOT RUN | PASS | 安装布局/manifest 源码接受；签名、系统安装与激活按用户暂缓 |
| E4-01 | 源码验收通过 | PASS | 定向/race PASS | N/A | loopback QUIC PASS | PASS | 总控已接受单一 SQLite owner、QUIC 复用、流取消与生命周期门闩；物理网络留 E5 |
| E4-02 | 源码验收通过 | PASS | 定向/race PASS | Win监听PASS、Mac命名pasteboard PASS | N/A | 387b57c五组checks/三平台包PASS | 总控已接受schema9权限、原生watcher、生命周期FIFO与单owner；准确包用户剪贴板留E5 |
| E4-03 | 源码验收通过 | PASS | 定向/race/前端PASS | Win真实HWND隔离写读PASS、Mac命名pasteboard PASS | loopback QUIC PASS | 387b57c五组checks/三平台包PASS | 总控已接受固定worker/latest pending、deadline/cancel/reset、singleflight、非对称generation与写入错误状态；物理Win↔Mac留E5 |
| E5-01 | 执行中/待复审 | PASS | PASS | PARTIAL | PARTIAL | PASS | `0c59ae2` 的 E5-G 已通过；4d36132 的 E5-H 已完成准确包重新配对、策略覆盖后恢复继承、六项默认关闭、精确删除与 HK revoked 收尾；设备目录系统 picker 已按 package PID/控件树核验，受管目录覆盖已保存（revision 8）并经“继承全局目录”恢复（revision 9）。E5-B 首条 Windows→Mac 系统文本仍 FAIL；链接、图片、反向、防回环和文件并存未运行，窗口已关闭且两端 grant/master 均已关闭。网切/睡眠、DPI/键盘仍待 |
| E5-02 | 阶段已同步/待最终验收 | PASS | PASS | PARTIAL | PARTIAL | PASS | 已核对 `560f6f2` 五项 CI success；E5-A/B/C 证据已推送。仅最终物理/系统矩阵未闭合；未合并 main、未移动 tag |


## 当前交付对账（2026-09-19）

| U | 已接受范围 | 尚缺的完成证据与依赖 |
|---|---|---|
| U1 一次接收 | E2-01 源码、计划/策略/恢复约束；E5-G 的准确 package 默认“接收文件”弹窗、`keep_both`、双端 1 MiB hash PASS | 断线/应用重启中的接收恢复、更多目录/异常矩阵仍待；本次不依赖剪贴板授权。 |
| U2 配对/删除 | E1-01/02/03 主路径源码；E5-G 准确 package UI 配对、精确删除、HK `revoked=1` 与 profile 重启不复活 PASS；E5-H 用同一已撤销 identity 显式重新配对后由 4d 准确 UI 再次删除/服务端清理 PASS | LAN 反向现场路径、离线 outbox 和更多系统确认矩阵仍 PARTIAL。 |
| U3 发现/恢复 | IPv4 主路径、原生事件与快照源码；E5-D test-only IPv6 signed discovery/LAN TLS 与 mDNS 原型 | 生产 IPv6 discovery/mDNS provider 未启用；物理多网卡、准确包IPv6、DHCP、VPN/TUN、睡眠矩阵也未执行，均须在相应真实环境和操作下完成。荷兰隔离 namespace 仅验证原型，不能替代物理双机矩阵。 |
| U4 自动剪贴板 | E2 旧入口迁移、E4 源码 | 准确包物理同步首条 Windows→Mac 文本 FAIL；用户已允许本轮临时覆盖，但启动聚合在执行层被 `blocked by policy` 拒绝、窗口未启动；其余双向文本/链接/图片、防回环和文件并存待可实际执行的同范围有界窗口。 |
| U5 设置 | E2-02 源码/浏览器范围；0c59 准确 Windows package 的两次“本机名称”后台 UIA 保存/持久化 PASS；E5-H 4d 准确包确认设备目录继承、策略覆盖后恢复继承、全局 `keep_both`、六项 clipboard 权限默认关闭及受管系统目录 picker 的标准控件可达 | 文件接收、更多设备覆盖及物理双机设置矩阵仍待；Mac AX 未获用户授权，不扩展自动化。 |
| U6 系统共享 | E3 源码和非签名构建 | Windows package identity/系统激活、Mac App Group/Share Extension 注册与安装生命周期；用户已暂缓签名/信任条件。 |
| U7 四视图 | E2-04 浏览器范围、部分准确包 125%/小窗证据 | Windows Tab 顺序需用户点击准确 package 置前台；150%/深色需要用户提供相应系统显示/主题条件，Mac 原生控件矩阵未运行。 |

U3 独立原型已在荷兰隔离 namespace 完成，范围为 IPv6 签名发现、TLS 控制、QUIC 文件分项与 DNS-SD 候选 hint；没有重做双 NAT、改宿主网络或扩产品 UI。后续只在明确产品收益与签名 hint 验证入口成立时评估生产 provider。

## 执行边界与恢复入口

- 当前批次为 E5 联合验收；E4 自动剪贴板源码范围已由总控验收通过。签名相关由用户暂缓，准确包 DPI、双向网络、双机 UI 与物理用户剪贴板按实际证据推进。
- 授权含阶段工作分支/PR、两平台项目依赖及隔离安装、香港 LinkSend 支持组件事务部署、荷兰构建与专属 namespace/container NAT。
- 不合并 main、不正式 Release、不购买签名服务、不改宿主防火墙/路由/代理、不影响无关业务。
- 远程全部使用 codex-ssh-manager resolve/probe/audit；2026-09-19 当前 Mac 地址为 `10.234.35.5`，保留登记 alias、密钥与 host key 校验。复杂任务使用 durable job，断线先 resume/tail；历史证据中的旧地址不改写。
- 生产先验证可读备份，再最小修改、独立验证和明确回滚；只使用已核实提交，旧数据库不能覆盖新业务写入。
- 保护用户 profile/文件/剪贴板；保留无关未跟踪目录并逐路径暂存，不使用 git add .。
- 根/desktop 两模块分别验证（含 GOWORK=off），前端与原生/网络结果独立。协议/schema 变更先文档与兼容安全测试。
- 本批 E5-A 已在 `10.234.35.5` 完成 resolve/probe/audit 和受管 remote jobs；HK 健康生产服务继续保持 `387b57c`/schema 4，未重新部署。系统剪贴板保持不读不覆盖；用户已允许同范围双端临时使用，但本次执行层拒绝了窗口启动，未产生 package 或 profile 副作用。

### E5-J 4d 准确包中断/重启恢复（2026-09-20）

- [x] 固定 4d361320bce1c0bb354a590fe0cc3c73be0242aa 的 Windows/Mac 准确包与载荷摘要；1a46564 只记录为证据工作树。
- [x] Windows 到 Mac 正常 256 MiB 文件及 SHA256 验证，路径为 lan_direct / QUIC / TLS 1.3 / ALPN linksend/1。
- [x] Windows sender 在至少一块已验证且整体未完成时中断；同一 sender task/TransferID 经重启后的恢复传输 UI 进入新 attempt，最终 1 GiB SHA256 一致、双方确认完成、0 retransmit。
- [x] Mac 的中断 checkpoint、恢复后缺失 1,065,353,216 bytes、同一 TransferID/manifest/selection/receive plan/target directory 和最终 SHA256 已核验。Mac 连接恢复会创建新的本地 receive task/attempt，文档不把它表述为双端 task ID 不变。
- [!] 单次 Mac 到 Windows V2 activation 为 0-byte QUIC_HANDSHAKE_FAILED；保留负例，不重试，不影响 Windows sender 恢复结论。
- [!] 测试 peer 接收目录清理被执行层 blocked by policy：SaveDeviceProfile helper 在执行前未运行，受管目录仍为 revision 12；用户 profile 未触碰。不得标注为已恢复继承。
- [ ] 物理网络切换/睡眠恢复、Mac 到 Windows 反向握手根因和系统 Share Extension/App Group 激活仍需独立验收。

### E5-K（NOT PASS）
- [x] 4889cbe handoff/diagnostic 与五项 CI；[!] 新反向 V2 在 ice_checking/CHECK_TIMEOUT、0 bytes 失败，未重试。


### E5-L：4889cbe 反向 QUIC 写路径诊断（NOT PASS）
- [x] 复用固定 4889cbe Windows/Mac 包和现有隔离 profile，完成 baseline V2 request：ICE 两端 selected/Connected，但 Mac sender 在 quic_handshake/QUIC_HANDSHAKE_FAILED、0 bytes 终止；持久受限分类为 udp_socket/0x41/EHOSTUNREACH。
- [x] 仅 Mac 设置 QUIC_GO_DISABLE_ECN=true 进行第二轮；仍得到 QUIC_EVENT_OPERATION=write_EHOSTUNREACH 和 0 bytes。该对照说明禁用 ECN 后仍复现，不能归因于 ECN 单因素；它不排除 WriteMsgUDP/sendmsg 或内核/链路原因，也不是把 ECN 永久关闭作为产品修复的依据。
- [x] 两轮均保存无凭据 ICE/QUIC 白名单日志、任务安全快照、source hash 和 PID 收尾回执；Windows 没有 incoming task，因此未 Invoke 接收。两端 clipboard master=false，测试 peer 六项 grant 全 disabled。
- [!] 无 sudo 改动的权限检查显示：tcpdump 可执行但 sudo -n 要求密码；当前 Mac 用户对 /dev/bpf0..3 无读写权限。因此头部观察 BLOCKED_BY_EXTERNAL_ENV，没有抓包、pcap、全机过滤或权限变更。
- [ ] 供总控/用户审阅的最小头部观察草案，未执行：在 Mac 的现有管理员权限终端中，仅在一次新的受控双机 4889 attempt 期间运行下列 90 秒/80 包有界命令。它只把 UDP/ICMP 的时间、IP、端口、长度和 ICMP 摘要写到终端；没有 -X、-A、-w，不保存正文或 pcap：
~~~sh
sudo sh -c '/usr/sbin/tcpdump -i en0 -nn -q -l -s 96 -c 80 "(host 10.234.35.5 and host 10.234.16.254) and (udp or icmp)" & p=$!; (sleep 90; kill -INT "$p" 2>/dev/null) & g=$!; wait "$p"; r=$?; kill "$g" 2>/dev/null || true; exit "$r"'
~~~
  观察仅用于区分 QUIC datagram 是否离开 Mac、是否收到相关 ICMP 拒绝；不得把没有样本当作成功或失败原因。双 IP outer filter 不会显示第三方网关发出的 ICMP 引述错误；没有匹配样本不能排除网关拒绝，暂不扩展观察方案。执行前仍需总控确认该用户侧权限窗口。

### E5-M：4889cbe Windows→Mac 接收端应用重启恢复（PASS）
- [x] 固定 workflow 35475890292 的 4889cbe Windows/Mac 准确包，并复用受管 1 GiB 源 e5j-win-to-mac-restart-1g.bin（SHA256 5fd17c10b75c7052a339acb81ff9ee8884e9b08d4d7d8763bd52b5cd39f4a0ae）；未新建或复制 GiB 文件。
- [x] 正确 fixture 使用 e5a-0c59/windows/profile（sender identity 83c68b7afd4698cc8affca4f0aa67c2460521c7875f9ce326b7807b9d670561f），Mac 既有该 identity 的 auto_accept。此前错误 profile 达 awaiting_acceptance、0 bytes，已停止且不计入恢复证据。
- [x] request a6faacd12d62457b86acb09bb217acfd：Mac 仅在新 TransferID 的 verified 4 MiB、received 8 MiB、committed 0 checkpoint 后由受管监控 SIGTERM；sender 随后为 recovering/connection_interrupted。UIA 仅 Invoke 恢复传输，不 Invoke 重新发送。
- [x] sender task 和 TransferID 保持一致并进入新 attempt；最终双方 verified/committed 均为 1 GiB，sender retransmitted 4 MiB、receiver retransmitted 0、bilateral_confirmed=true，lan_direct/QUIC/TLS 1.3/ALPN linksend/1，目标 SHA256 与源一致。
- [x] Mac 恢复任务只接收缺失 1,069,547,520 bytes；初始/恢复 receive task 的 TransferID、manifest、selection、receive plan 和目标目录一致，binding_consistent=true，receive plan digest 为 191827a88f13e4d09a2ea34afa9b8eec484437d77a3d83eb3a20fd99c4dd0415，最终无意外第二副本。
- [!] 两次旧同名 TransferID 监控漏列/时序错误和首次 UIA 参数名冲突均保留为无效基线；前者没有中断，后者没有 UI action，均不计入通过。已终态的旧受管 activations 可逆归档，不删除源或用户文件。Mac→Windows write_EHOSTUNREACH 仍是独立反向故障。
- [x] 两端 package 按 owned PID/path 停止；clipboard master=false、enabled grants=0。最终回执为 C:\Users\Wen\.codex\supervision\linksend-desktop-experience\e5m-4889cbe\e5m-final-machine-receipt.json。

### E5-N：4889cbe 小文件与空闲性能基线（PASS，样本范围有限）
- [x] 新增受管 fixture 为 3 个连续 1 MiB 文件和 12 个 512 KiB 批量文件，共 15 个、9,437,184 bytes，低于 32 MiB 上限；未重传已有 1 GiB 样本。Windows→Mac 4 次 run 均为 lan_direct/QUIC/TLS 1.3/ALPN linksend/1、双方确认、0 retransmit，Mac 只读核验 4 个 completed task、15 个目标文件和全部 SHA256，未见 E5N 重名副本。
- [x] 实际端到端 elapsed：连续 1 MiB 为 3,588.242、1,312.396、1,856.090 ms（n=3）；12×512 KiB 批量共 6 MiB 为 2,509.479 ms（n=1）。样本不足，不报告 p95。
- [x] 空闲主进程各采 12 点、2 秒间隔：Windows 22.221 秒窗口的 CPU 秒差为 0.328125，即单逻辑核平均 1.476627%，working set 51,552,256–52,989,952 bytes、private 65,372,160–66,588,672 bytes；Mac 22.195 秒窗口的 RSS 恒为 130,514,944 bytes，ps 平滑 pcpu 为 0.1–0.7%、均值 0.316667%。两端均不含 WebView/helper 子进程，两个 CPU 口径不可直接等同。
- [!] 准确 package 没有端到端 discovery timestamp，也不暴露单独 DB commit 定时边界，均记为 NOT_OBSERVABLE；当前 n=3/n=1 不称 p95。初始 Mac 采样成功后 SSH-manager 的 tail-job 读回 helper 触发 zsh status 变量问题，未为绕过它新增样本。

### E5-P：4889cbe 海量小文件基线（PASS，1000 文件边界）
- [x] 一次单批目录传输：新增 1000 个 1 KiB 专用文件，合计 1,024,000 bytes，低于 4 MiB 上限；不重跑 E5-N 的 1 MiB/12 文件样本。产品 file_count=1001，因为它把所选根目录与 1000 个普通文件均计为条目。
- [x] sender/receiver 均 completed、verified/committed=1,024,000、bilateral_confirmed=true、retransmitted=0，lan_direct/QUIC/TLS 1.3/ALPN linksend/1。Mac read-only 逐文件 SHA256 复核 1000 个目标文件，mismatch=0；完整 1000 项 source hash manifest 保留在 E5P 最终回执。
- [x] sender task persisted start 至 bilateral completed 为 35,000 ms。activation 入队至完成的墙钟未被初始 monitor 持久化，entry 被消费后不可回补，明确记为 NOT_OBSERVABLE，不能用 journal 文件时间猜造。
- [x] 空闲主进程观测均小于 180 秒：Windows 15 点/28.309 秒，CPU 秒差 1.671875、单逻辑核平均 5.905756%，working set 51,404,800–53,501,952 bytes、private 64,425,984–66,347,008 bytes；Mac 12 点/22.181 秒，RSS 130,482,176–130,514,944 bytes、ps 平滑 pcpu 0.1–0.9%、均值 0.316667%。仅主进程，不称长时稳定性或全应用资源。
- [!] 首个 sender monitor 把 product file_count 误按为 leaf file count，完成后才恢复回执；首个 Mac read-only 查询误按 manifest_summary 精确等于目录名。两者都是收集假设，未重传、未改变传输或补造时钟。两端 package 已按 owned PID/path 停止，clipboard=false、enabled grants=0。

### E5-N 收束后的剩余必交付项

仍可自主完成的具体下一步：
- [x] 4d19adce4bcc759afedeca87f02670e5e235b250 的 core×2、desktop-wails3-checks×2、wails3-packages 五项 CI 已全部 success；restart-recovery race 用例的本地有界重复验证通过。
- [ ] 性能后续观察（非本里程碑新增必交付）：长时空闲/GC、发现延迟和独立 DB commit。E5-P 只覆盖一个 1000 文件目录批，不外推百万文件；当前准确包没有 discovery/独立 commit 时钟，NOT_OBSERVABLE 只表示本轮无测点，不表示完成或外部权限阻塞；不为补测点单独修改包。存储并非完全无证据：`history_test.go:327` 的 `BenchmarkTaskHistoryConnectionLifecycle` 测量 `persistRecord → SQLite autocommit` 全路径，E4 的 100×3 结果 2.140/2.176/2.190 ms/op 含序列化、锁和 SQL，不能写成单独 `tx.Commit`。

### E5-Q：U2/U7 有界源码收敛（源码 PASS，准确包待后续候选）
- [x] paired / nearby / removed 三类目录按名称、别名与完整 identity 查找并独立分页，每页最多 10 行；详情稳定绑定 DeviceID，目标消失后收起、页码钳制并回焦搜索。
- [x] 抽屉 header/footer 固定、正文独立滚动；关闭、保存、删除和二次确认可达。Chromium source harness 已核对 960×640 与 1100×720 的长中文名、无水平溢出、Tab/Shift+Tab 闭环。该结果不替代准确 Windows DPI、macOS AX 或系统主题验收。
- [x] 草稿 paths、队列、活动任务每页最多 10 项；enqueue 继续发全量 paths，跨页 ReorderQueue 继续传完整 pending IDs，动作使用稳定 ID，页码在后台删除/完成后钳制。收件状态只在顶栏并导航至既有“文件接收”设置；`no_content` 不列入进行中。
- [x] 前端验证：`pnpm test` 8 files / 76 tests、typecheck、lint、build 全部 PASS；本地源浏览器检查不是准确 package 结果。

确需用户现场、权限或签名条件：
- [ ] U2 离线删除的当前 4889 准确包尝试没有执行删除：仅本次进程覆盖 LINKSEND_SERVER_URL=https://127.0.0.1:1 后，UIA 可 Invoke 设备导航，但精确测试 Mac identity 不在当前 WebView 可访问树。没有 ScrollPattern，向已确认 package HWND 的设备列表区域发送 12 次定向滚动仍不可达；未按第 N 个设置按钮盲删。只缺用户现场手动滚动并选择该精确测试 Mac 行后的一次删除/确认，随后再由受管脚本核验 pending-sync、重启、原服务同步和重新配对。
- [ ] E5-L 反向 Mac→Windows 写路径根因：最小两 IP UDP/ICMP 头部观察仍需 Mac 现有管理员权限终端。双 IP outer filter 不显示第三方网关的 ICMP 引述错误；无匹配样本不能排除网关拒绝，不扩大观察方案。
- [ ] Windows 原生 Tab 顺序：只缺用户把准确包窗口置前且 input desktop 为 Default 后的一次真实 Tab 检查；不强制前台或改用 CDP。
- [ ] 150% DPI/深色：只缺真实 150% 显示与系统深色环境的准确包布局观察；当前用户全局显示/主题保持不改。
- [ ] 系统剪贴板双机矩阵及与文件并存：用户已有临时覆盖授权，但执行策略仍拒绝窗口；只缺可执行的隔离/可丢弃 clipboard 窗口，不需再次授权或读取日常 clipboard。
- [ ] 物理网络切换、睡眠/唤醒：只缺双机同时在线且允许自然事件时的接口、session、revision 与 hash 时间线；不修改路由、代理或防火墙。
- [ ] 受信任系统安装、Share Extension/App Group、Developer ID/公证：用户已暂缓，需要相应签名身份、安装决定和凭据；现有受保护安装与快捷方式不覆盖。
