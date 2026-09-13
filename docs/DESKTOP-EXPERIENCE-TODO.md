# LinkSend 桌面体验全轮 TODO

日期：2026-09-13。唯一产品写入方：执行任务 `/root/desktop_executor`（由已停止的 `01a096c7-6cfb-71b0-87aa-aa7ed781bb2f` 一次交接）。
总控：`01a096c4-2e59-7db3-a7a8-ec2a125409d6`（local）。工作分支：`codex/desktop-experience-upgrade`。
基线：`3bcb73019d1ac6de1d9f341bb7d22e2813875081`。最新授权已接受 `2342009` 的原型 IO 修补，要求同步 E0 候选后连续执行 E1-01 至 E1-05 的独立后端闭环。

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
| E4-02 | U4 | E4 | E4-01 | 权限、原生监听/写入、能力协商 | 默认关闭；按设备/方向/类型双端授权；Windows AddClipboardFormatListener、Mac changeCount；正文不经 JS/WSS | evidence/DESKTOP-E4-CLIPBOARD-NATIVE.md |
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

| E1-01 | 待审查 | PASS | PASS | NOT RUN | NOT RUN | NOT RUN | schema3/incarnation/revision/旧端拒绝候选；空组初始化与显式跨组切换未闭合 |
| E1-02 | 待审查 | PASS | PASS | NOT RUN | NOT RUN | NOT RUN | 本机屏障/outbox/幂等撤销及负例候选；真实离线重启补同步未实机 |
| E1-03 | 待审查 | PARTIAL | PASS | NOT RUN | NOT RUN | NOT RUN | TLS签名同意/拒绝零pin候选；服务凭证、ack查询与物理双机未闭合 |
| E1-04 | 待审查 | PARTIAL | PASS | NOT RUN | NOT RUN | NOT RUN | 多地址/独立降级/Windows源绑定候选；IPv6/mDNS/物理矩阵未闭合 |
| E1-05 | 待审查 | PARTIAL | PASS | NOT RUN | NOT RUN | NOT RUN | 事实目录与不取消健康task候选；准确包双向/网络切换/睡眠/双NAT未闭合 |
| E2-01 | 待办 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 未审查 |
| E2-02 | 待办 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 未审查 |
| E2-03 | 待办 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 未审查 |
| E2-04 | 待办 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 未审查 |
| E3-01 | 待办 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 未审查 |
| E3-02 | 待办 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 未审查 |
| E3-03 | 待办 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 未审查 |
| E4-01 | 待办 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 未审查 |
| E4-02 | 待办 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 未审查 |
| E4-03 | 待办 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 未审查 |
| E5-01 | 待办 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 未审查 |
| E5-02 | 待办 | NOT RUN | NOT RUN | NOT RUN | NOT RUN | NOT RUN | 未审查 |

## 执行边界与恢复入口

- 当前批次为 E0 候选同步及 E1-01 至 E1-05 后端闭环；按最新用户授权完成阶段提交、工作分支推送、draft PR 与 CI，无需等待逐次回执。
- 授权含阶段工作分支/PR、两平台项目依赖及隔离安装、香港 LinkSend 支持组件事务部署、荷兰构建与专属 namespace/container NAT。
- 不合并 main、不正式 Release、不购买签名服务、不改宿主防火墙/路由/代理、不影响无关业务。
- 远程全部 codex-ssh-manager resolve/probe/audit；Mac 最新地址为 10.234.186.184，保留登记 alias 与 host key 校验。复杂任务使用 durable job，断线先 resume/tail。
- 生产先验证可读备份，再最小修改、独立验证和明确回滚；只使用已核实提交，旧数据库不能覆盖新业务写入。
- 保护用户 profile/文件/剪贴板；保留无关未跟踪目录并逐路径暂存，不使用 git add .。
- 根/desktop 两模块分别验证（含 GOWORK=off），前端与原生/网络结果独立。协议/schema 变更先文档与兼容安全测试。
- 活跃远程作业：无；历史jobPath见E0分项证据。E0-A为ec55db317b0b5b0dfa60c1a61219f28da9b401da；ADR语义修订11c44df已接受；2342009原型IO修补已被总控接受但不代表U6/文件交接通过。当前先同步E0候选，再连续交付E1后端；Windows安装信任、Mac 4097/文件交接、准确包物理双向与双 NAT 仍是明确缺项。
