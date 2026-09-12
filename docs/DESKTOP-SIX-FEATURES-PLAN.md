# LinkSend 六项桌面能力实施规划

日期：2026-09-12。状态：2026-09-12 已开始按 M0–M6 实施；当前阶段与实际结果见 [执行记录](evidence/DESKTOP-SIX-FEATURES-EXECUTION.md)。
适用仓库：D:/apps/Osend；规划时 HEAD：fbfc250159ea65dbb5840d6dadb03fe4423cf868。
用户场景：个人多电脑互传，兼顾日常临时交接。本轮仅覆盖上一轮回复的第 1–6 项。

2026-09-12 执行决策更新：用户明确取消 macOS12 支持，最低版本为 macOS13，采用 Go1.27.1；
M0 与 M1 合并发布。该授权取代下文规划时保留旧最低系统版本的限制，具体证据见
[平台兼容性纠正](evidence/DESKTOP-M0-COMPATIBILITY-CORRECTION.md)。

## 1. 范围与编号

本文件的 S01–S06 对应用户所选六项，不是 PRODUCT-PLAN.md 中 F01–F06 的顺序。

| 本轮编号 | 用户所选能力 | 原规划关联 |
| --- | --- | --- |
| S01 | 我的设备：别名、固定、排序、最近使用、专属接收目录及权限 | F01、F05 的设备策略 |
| S02 | 系统发送入口：拖放、Windows 发送到、macOS 原生入口、单实例与发送草稿 | F02、F04 |
| S03 | 托盘常驻与原生通知：窗口关闭策略、后台启动、通知定位、主动退出 | F03 |
| S04 | 显式发送文字、链接、剪贴板截图 | F06 |
| S05 | 接收决策：清单、目录、空间、保留两份、跳过、取消、选择接收 | F05、F15 |
| S06 | 本地队列与收件箱：排队、等待上线、恢复、搜索、定位、重发和清理 | F07、F08 |

一次性分享码/ShareGrant、多人分发、持续剪贴板同步、文件夹同步、手机/浏览器客户端、自动更新和中继均不在本轮范围。

## 2. 技术路线

采用“当前稳定工具链 + Wails 3 原生能力 + Go 单一任务内核”。保留 Go 双模块、Pion ICE、quic-go、SQLite、Ed25519/TLS 1.3、BLAKE3；不为追新重写已验证的数据面。

先进性的落点是：类型化边界、事件驱动快照、持久队列、原生桌面入口、可访问控件、长列表按需虚拟化以及可复现的迁移和故障测试。

### 2.1 已核实的版本基线与升级目标

下表的“升级目标”仅表示 2026-09-12 官方来源已发布的候选，不表示已在本项目验证。实际执行若日期变化，应重新核对官方发布、engines、peerDependencies 和安全公告，然后锁定精确版本。

| 层 | 当前仓库 | 规划选择 |
| --- | --- | --- |
| Go | toolchain 1.26.5 | 官方稳定 1.27.1 为首选升级目标；必须通过 Pion/quic-go/Wails、Windows race、macOS 构建及 JSON/签名/恢复兼容测试；同系列 1.26.8 可作为有证据的不兼容回退 |
| Wails | CLI/Go/runtime beta.18 | beta.18 已具单实例、托盘、文件拖放和通知；较新 beta.20 为单独迁移候选，仍是预发布，不称稳定版。仅在完成原生能力探针和回归后同步升级 CLI/Go/runtime/build assets |
| React / React DOM | 19.2.8 | 同版本锁定当前稳定 19.3.0；客户端渲染，继续使用生成的 Wails TypeScript bindings |
| TypeScript | 5.9.3 | 采用兼容稳定线 6.0.3 + typescript-eslint 8.70.0；最新 7.0.2 不兼容当前 lint 的编译器 API/peer 范围，暂缓主线采用，不强装 |
| Vite / Vitest | 7.3.6 / 4.1.11 | 8.3.0 + @vitejs/plugin-react 6.1.1 / Vitest 5.0.0 为候选；单独验证构建/测试迁移，不启用仍属实验的 Rust React Compiler |
| Node / pnpm | Node 22.15.0 / pnpm 11.19.0 | Node 24.21.0 LTS + pnpm 12.4.1 为目标；Node 26.8.2 Current 不作为默认发布构建环境 |
| UI 数据缓存 | React hooks + 轮询 | TanStack Query 5.102.8 用于查询/分页/缓存，Wails 事件推进有 revision 的快照；后端是唯一任务真相 |
| 可访问性 / 长列表 | 现有 CSS 与原生控件 | 保留 CSS tokens；按需 Radix（例如 Dialog 1.1.23）；历史规模需要时引入 TanStack Virtual 3.14.12 |
| 存储 | modernc SQLite + schema 2 任务历史；trust.json | 复用现有驱动与迁移方式，增加版本化元数据表；身份授权仍有唯一所有者，不复制进第二套“信任数据库” |
| 平台适配 | Wails 3 | Go 薄适配层 + 必要的 Windows/Apple 原生 API；系统依赖留在桌面模块，根模块不导入 Wails/Cocoa/WebView |

版本核对的来源及兼容限制见文末；前端精确升级结论在执行前再次确认。一次只迁移一个有独立回滚边界的工具链组合。由于 Go 1.27 调整标准 JSON 实现，除编译外要特别检查历史 JSON 可读性、未知字段、错误分类、规范签名输入与 manifest 摘要一致性。

TypeScript 的选择有明确依据：官方 TS 7 不提供旧编译器 API，typescript-eslint 8.70.0 的 peer 范围为 >=4.8.4 <6.1.0。官方虽提供 TS 6/7 并行方案，本轮不为追求版本号默认引入双编译器。TS 6.0.3 是兼容性选择，不声称它是 npm 最新大版本。

### 2.2 依赖使用原则

- 不引入 Next.js、SSR/RSC、Electron/Tauri 第二桌面壳、远端任务平台、Redis、云对象存储。
- React 表单和短期选择状态优先 useState/useReducer；不同时引入 Zustand、Redux、XState 来管理同一份任务。
- TanStack Query 不负责执行传输。收到较旧 revision 时丢弃，事件丢失、窗口恢复或服务重连后重新取快照；进度合并/节流，不能每个块 ACK 触发列表全量查询。
- 明确 Query 的重试/失效策略：撤销、接受、取消和入队不能被通用命令重试重复执行；staleTime 可配合显式失效，不使用会阻止失效重取的 static 配置。后端保证幂等，前端不乐观宣布文件已提交。
- Vite 8 的默认浏览器目标与构建器已变化。按项目最低支持的 WebView2/WKWebView 明确 build.target 和所用 Web API；不得因升级默认值静默抬高最低 macOS。降级语法不能代替缺失 Web API 的平台处理。
- 动效优先已有 CSS transition 和 prefers-reduced-motion；Motion 只在确有复杂交互且收益可测时引入。
- 单个深色/浅色 token 系统、语义 HTML、可见焦点、键盘操作和系统字体；不重建一套无关视觉品牌，不把工作区改成宣传首页。
- 前端变化逐步拆分现有 App.tsx 的页面、hooks、DTO adapter 和组件；新增代码不用 any 掩盖模型不完整，不扩大现有全局 lint 豁免。
- 中国大陆环境：先检查已配置代理、缓存和源；必要时使用可达镜像，但版本与完整性要对照官方元数据。不要永久改用户全局 npm/Go 配置。

## 3. 顶层结构与状态所有权

| 对象 | 所有者 | 主要职责 |
| --- | --- | --- |
| DeviceProfile | Go app/store | 本地别名、固定顺序、关系标签、最近实际交互 |
| ReceivePolicy | Go app + identity 授权层 | 按 peer 生效的确认、目录和冲突策略，撤销立即失效 |
| SendDraft / LocalContent | Go app；React 编辑副本 | 文件路径、内容快照句柄、目标与准备状态；用户点击排队后持久化 |
| ReceivePlan | Go transfer/app | 原 manifest、选择集合、目标相对路径映射及恢复绑定 |
| QueueItem | Go app/store | 排序、等待条件、到期时间、调度与幂等提交 |
| DeliveryTask | 既有 Go app/transfer | task/attempt/session/revision、实际字节、恢复与终态 |
| DesktopIntegration | apps/desktop 平台层 | 单实例、原生入口、剪贴板、托盘、通知、启动与退出 |
| TaskView / InboxView | React + 查询缓存 | 展示、筛选、分页、用户命令；不直接读写任务库和文件正文 |

建议的组织方式是扩展现有 app/store/transfer/identity，再在桌面模块建立实际需要的平台 adapter。不要只为了符合目录图创建空包，不要求按本表强行新建同名类型。

持久化升级需要：从执行时实际 schema 迁移；迁移前可读备份；事务；revision/幂等保护；写失败后准确报告能力；旧程序不打开新 schema 的明确回滚步骤。设备别名和队列不应影响已验证块的 fsync/checkpoint 顺序。

## 4. S01 我的设备

### 用户行为

- 把已验证设备归入“我的设备”，设置只在本机生效的别名和固定顺序。
- 远端改名不能覆盖本地别名；名称、IP、系统图标均不用于替代身份。
- 可设置专属接收目录，默认继承全局目录；切换目录不改变进行中任务的保存计划。
- 在线状态来自当前证据；最近使用记录真实交互，不用 presence 时间充当传输时间。
- 收藏、已信任、免确认接收分别控制；“我的设备”标签本身不授予权限。
- 取消信任/屏蔽会取消免确认，并使新 LAN/WSS 请求、排队任务和恢复请求按策略停止；仅清除 pin 不足以阻止下次自动重建信任，需要明确的本地拒绝记录。

### 验收

重启后别名/顺序/目录保留；重新发现同一身份不生成重复设备；远端同名不同密钥不继承权限；撤销后不能从 LAN 绕过；正在传输的任务不会被随后修改目录重定向。

## 5. S02 系统入口与发送草稿

### 用户行为与平台边界

- 拖放文件和目录到窗口，预览数量/大小、去重和移除；换目标保留草稿。
- Windows 首期交付当前用户级“发送到 LinkSend”；不覆盖默认文件关联，不以后台常驻管理员服务作为前提。
- Windows 原生 Share Target、macOS Finder Services/Share Extension 是不同集成，不把一个菜单项当作全部支持。
- macOS 首期使用窗口拖放与 Finder Services，必要时通过 NSServices/NSApplication.servicesProvider 薄适配；Share Extension 需要独立 .appex target 和数据交接。beta.18 未提供托盘拖入接口，菜单栏图标收文件需额外原生适配，不能与窗口拖放混为一谈。
- Windows Share Target 需要 package identity。NSIS/Win32 可评估签名 sparse MSIX 加 WinRT 激活桥接，但本轮基础交付采用 SendTo；不为了分享面板重写成 WinUI 或要求全局管理员权限。
- 六项完成的基础入口范围为窗口拖放、SendTo、Finder Services；系统分享面板和图标拖入是独立可选扩展。界面与交付报告必须写“服务菜单”或“发送到”，不能将其描述为 Share Extension 已完成。
- 系统入口可能交付临时文件 URL、受限授权或 item provider。需要持久排队时先可靠取得访问权或复制用户明确选择的内容到受控本地快照；不能把马上失效的临时路径当永久源文件。SendTo 的命令行多选有系统长度上限，超限明确提示，必要时再评估原生句柄/COM入口。
- 使用已核实的 Wails 单实例功能，把第二次启动的参数及 WorkingDir交给首实例。相对路径按发起进程目录解析；处理 Unicode、空格、长路径、多文件和重复事件。
- 按 profile 隔离单实例键；不同测试 profile 可以同时运行。同 profile 的 CLI/GUI 不允许各自抢写数据库。
- 系统入口只创建/合并可见草稿，默认不因收到启动参数直接向上次目标发送。

### 验收

应用关闭、打开、隐藏、忙于传输四种状态均可接收新内容；只有一个同 profile 写入者；取消文件选择不清空草稿；拖放目录不经 JS 读取 File.arrayBuffer；第二实例退出但内容不丢失；系统集成可卸载且不删除用户数据。

## 6. S03 托盘、后台与通知

### 默认行为

- 不默认开启系统自启动。首次明确提供“关闭窗口后继续接收”的选择；用户未选择时保持当前退出行为。
- 托盘/菜单栏提供显示窗口、打开收件箱、暂停调度/恢复调度、退出。
- “暂停调度”只阻止下一项开始；“暂停当前传输”调用已有传输控制，文案和语义不能混用。
- 真正退出先关闭新请求入口、停止调度、按用户选择处理活跃传输、持久化，再关闭连接与 goroutine。
- 退出保护必须支持有序保存后退出，不只提供破坏性取消；旧任务恢复权限和块检查点保持可用。
- 通知只在真实接收请求或确认后的终态触发；点击携带不透明 task ID，由后端定位内容，不把绝对路径塞入可伪造命令。
- 同一 task/终态 revision 去重；重启不反复通知旧完成任务；通知权限被拒绝时仍在应用内显示，不把传输判失败。
- 活跃正文传输期间可用系统防自动睡眠机制，进入暂停/完成/退出必须释放；不阻止用户显式睡眠。
- 后台运行与开机启动的选择可撤销；不同安装/便携形态分别验证。

### 验收

窗口隐藏仍能接收并提示；真正退出后进程/监听退出；二次启动能唤回隐藏窗口；通知点击正确定位任务；通知关闭/无托盘环境有明确可用入口；睡眠唤醒不会重复 listener 或把旧事件覆盖新状态。

## 7. S04 文字、链接与剪贴板截图

### 产品范围

- 只读取用户主动提交的文字/链接或用户主动请求读取的当前剪贴板。
- beta.18 内置 clipboard 只覆盖文本；截图/图片和剪贴板文件列表使用可测试的平台 adapter，不杜撰通用 Wails 图片 API。
- 剪贴板图片由 Go/原生层立即生成不可变、限额的临时 PNG 等内容快照。之后剪贴板变化不改变已排队内容。
- 文本和 URL 建议上限 64 KiB UTF-8；剪贴板图像建议编码后 32 MiB、解码 40 MP 上限。执行时依据内存测试调整并记录，不能只限制压缩字节而无解码像素限制。
- 用户在 React 输入的短文本是显式表单输入，可经受限 DTO 交给 Go；从文件/图片读取的正文不得通过 JS IPC。网络传输始终经过认证 QUIC。
- 第一版可以明确提供“作为文本文件发送”，随后完成原生内容类型；最终六项验收需要复制文字/手动打开链接/保存图片的完整体验，不能把临时退路当最终完成。
- 原生内容类型必须有能力协商和内容摘要绑定；旧端不支持时明确以普通文件发送或要求升级，由用户决定。
- 接收文字默认不覆盖剪贴板；URL 仅允许明确支持的 http/https 打开动作，其他 scheme 显示为文本；不自动打开、执行或解析 HTML。
- 历史默认保存类型、大小、设备和任务摘要；正文持久化要明确开关及清理策略。
- 首期可显示类型/尺寸/受限文字预览；图片缩略图只能通过经评审的受限本地展示通道或原生预览，不能以 Base64 原图绕过 IPC 边界。

### 验收

Unicode/空文本/多行/非法 UTF-8/过大内容；剪贴板打开失败与通知失败分别处理；排队后剪贴板变化不影响内容；恶意 URL 不自动执行；取消和清理只删除本次拥有的快照；新旧端均有明确结果。

## 8. S05 接收计划与安全保存

### 用户流程

接收请求显示认证发送者、内容类型、清单/目录结构、总量、可选子集、保存目录、空间和冲突；确认前不请求正文。选择目录或保留两份后显示最终拟保存名称。

默认不覆盖已有文件；遇冲突可选择保留两份、跳过或取消。跳过的是用户选择，不是假定已有文件内容相同。若今后提供“相同内容跳过”，必须比较完整摘要，不能只看名称/大小/mtime。

### 语义要求

- 保留不可变 original manifest；ReceivePlan 绑定其摘要、选中文件 ID 集合及目标相对路径映射。绝对接收路径不发送给对端。
- 子集是端到端协商结果。完整完成指所接受子集全部验证/提交/双方确认；原 offer 的未接收项另计 skipped，不增加 verified/sent/committed。
- 全部跳过使用明确的“未接收内容”结果，不产生虚假的成功字节或错误进入正文循环。
- 旧对端只能全量接收时禁用/解释部分接收能力，不通过不请求某些块骗过全 manifest 的完成校验。
- 目标名称使用 root-constrained、原子 no-replace 方式保留；检查到提交期间出现的新冲突有界重选/重新确认，不能先 Exists 后覆盖。
- 空目录、大小写冲突、Unicode规范化、长路径、Windows ADS/保留名、symlink/reparse 均沿用明确平台策略。
- 空间检查计算此次仍需写入的块、复用 staging、必要提交开销和同时运行任务的预算；检查是提示/预检，不替代运行时 ENOSPC。
- ReceivePlan 与 commit records 持久化。任务恢复后沿用原计划；不能在重启后给同一文件换个编号再重复提交。
- 目的目录失效时进入 needs_attention；不能静默改存到其他目录。

### 验收

部分接收、全部跳过、保留两份、提交时突然出现同名文件、空间耗尽、计划持久化失败、部分提交后强杀和接收完成但发送方未获终态确认。核对原文件哈希保持、选中子集哈希、保存路径、唯一字节和跳过数。

## 9. S06 队列与收件箱

### 队列

- Go 持久化队列，第一阶段全局活跃正文传输并发为 1。多个待处理入站请求有明确限额/到期/忙碌反馈，不能无限占用资源。
- 队列状态与 transfer 状态分开：queued、waiting_peer、needs_attention、running 等是调度状态；既有 Paused/Recovering/Completed 等仍由传输层管理。
- 入队使用幂等请求 ID，数据库提交成功后才向 UI 报告“已加入”；调序/取消在事务中完成。
- 用户发送时对离线目标明确选择等待；进程运行期间可自动启动已授权的队列项，接收端仍执行自己的确认策略。
- 单个离线目标不能堵住其他可运行任务；可运行项之间保持用户排序并防止长期饥饿。
- 等待采用 presence/发现事件与抖动退避，不为每个队列项创建一个永久 WSS 或轮询器；等待不占 ICE/QUIC endpoint。
- 稳定身份、配置和原始路径持久化。文件在排队后改变时先重新准备并提示变更；不默默传另一份内容。
- 应用重启后恢复队列可见性；未完成传输及待发队列先等待用户确认继续，遵循现有显式恢复原则，不因开机启动立即发出积压内容。
- 退出前与派发之间不能双执行；同 profile 保证一个 scheduler owner。恢复/new attempt/session 的边界继续沿用现有设计。

### 收件箱

- 按设备/方向/状态/日期筛选，搜索文件名；后端分页/索引，不能每次把所有历史记录和私有恢复路径发往 React。
- 已有文件可在文件夹中显示；文件已移走/删除则如实提示。
- “恢复”复用原逻辑任务及恢复身份；“重新发送”重验源文件并创建新逻辑任务，两者不能混用。
- “清理记录”“清理暂存”“删除收到的文件”分开；本轮默认只交付前两者，清理历史不删除用户文件。
- 清理暂存需检查所有活跃/暂停/可恢复任务引用，再只删除本应用拥有的孤立暂存；不把隐藏目录一概当垃圾。
- 大列表使用后端分页；测出渲染瓶颈后才引入 Virtual，不用虚拟化替代数据库查询设计。

### 验收

双击入队、同时两个启动入口、调序期间派发、离线目标旁路、取消与进度竞态、磁盘写失败、进程强杀、源文件变化、历史万条分页、已移动文件、清理后恢复请求。确认没有重复发送和状态倒退。

## 10. 交付顺序

| 阶段 | 内容 | 可演示成果 |
| --- | --- | --- |
| M0 | 现状审计、安全前置修复、工具链兼容探针、schema/协议/单实例设计 | 依赖版本与官方证据清单，现有行为回归通过；不把未验收网络矩阵改为 PASS |
| M1 | 类型化事件/快照、S01、S06 的持久草稿与队列骨架 | 自己设备可固定，忙时能可靠加入下一项，重启记录可读 |
| M2 | S02 单实例与系统入口 | Windows 文件管理器和 Mac 已实现原生入口可把内容交给同一个草稿/队列 |
| M3 | S03 后台生命周期与通知 | 关闭窗口继续接收、通知定位、真退出均有原生证据 |
| M4 | S05 接收计划、冲突、选择与恢复 | 一批文件选收/保留两份，强杀后恢复不重复提交 |
| M5 | S04 文字链接图片完整路径 | 显式粘贴→跨设备收到→复制/打开/保存，不经过信令正文 |
| M6 | S06 完整收件箱与跨功能联调、打包 | 从系统入口发送，后台接收，离线排队，冲突处理，重启恢复，历史可追踪 |

M1 队列骨架先实现必要性来自当前 StartSend 的 BUSY 全局互斥；不在没有 scheduler ownership 的情况下直接允许多个任务并发。阶段可以小批次交错，但协议、schema、bindings 与 build assets 的更改要串行整合。

## 11. 统一完成标准

- S01–S06 每项都有实现、自动化和原生/跨设备验证栏，缺项不能笼统写“全部完成”。
- 新增纯元数据/UI变化使用针对性检查；路径、授权、队列、恢复和协议必须有有意义的负向及故障回归。
- 根模块与桌面模块分别测试，且都检查 GOWORK=off；根普通/race/vet/build，桌面 verify/test/vet/build，前端类型/lint/组件测试/build，bindings与嵌入产物一致。
- 安全与协议覆盖旧客户端混合版本、拒绝/超限/未知字段、源变化、路径冲突、取消后无新写入、暂停优先、终态双方确认。
- Windows/macOS 原生入口、菜单、通知、关闭/退出、安装卸载单独验收。浏览器预览只能证明页面行为。
- 真实 LAN/跨 NAT/网络切换/强杀恢复的证据必须记录实际环境；缺少环境继续完成独立工作，并准确保留 NOT_RUN/BLOCKED。
- 本轮用户明确授权逐阶段自动推送、部署和发布测试预发布；执行 inspect/backup/change/verify/rollback 与精确源码产物核对。不申请签名证书或修改主机网络。
- 每阶段记录实际命令、退出码、关键证据、限制和下一步。包记录干净源码/dirty 状态、版本和哈希。

## 12. 官方资料与证据边界

- [Go 官方当前下载 JSON](https://go.dev/dl/?mode=json)：本次返回稳定 1.27.1 与 1.26.8。
- [Go 1.27 release notes](https://go.dev/doc/go1.27)：兼容性、JSON实现和标准库变化；不是本项目兼容测试。
- [Wails beta.18 固定源码](https://github.com/wailsapp/wails/tree/v3.0.0-beta.18/v3/pkg/application)：原生能力以固定源码和同版本示例为准。
- [Wails beta.20 release](https://github.com/wailsapp/wails/releases/tag/v3.0.0-beta.20)：预发布迁移候选。
- [Wails beta.18 剪贴板边界](https://github.com/wailsapp/wails/blob/v3.0.0-beta.18/docs/src/content/docs/features/clipboard/basics.mdx)、[原生通知说明](https://github.com/wailsapp/wails/blob/v3.0.0-beta.18/docs/src/content/docs/features/notifications/overview.mdx)：图片适配、通知授权与平台差异。
- [Microsoft Share Target](https://learn.microsoft.com/en-us/windows/apps/develop/windows-integration/integrate-sharesheet-receive)、[Apple Services](https://developer.apple.com/documentation/appkit/nsapplication/servicesprovider)、[Apple Share Extension](https://developer.apple.com/library/archive/documentation/General/Conceptual/ExtensibilityPG/Share.html)：不同系统入口的真实成本与边界。
- [React 官方 npm 元数据](https://registry.npmjs.org/react/latest)、[TypeScript](https://registry.npmjs.org/typescript/latest)、[Vite](https://registry.npmjs.org/vite/latest)、[Vitest](https://registry.npmjs.org/vitest/latest)、[pnpm](https://registry.npmjs.org/pnpm/latest)：当前发布版本不等于组合已经兼容。
- [Node 发行计划](https://nodejs.org/en/about/previous-releases)：优先仍支持的 LTS，具体 engines 以选定依赖为准。
- [TypeScript 7 官方发布](https://devblogs.microsoft.com/typescript/announcing-typescript-7-0/) 与 [typescript-eslint 兼容版本](https://typescript-eslint.io/users/dependency-versions/)：解释为何主线选择 TS 6，而非绕过检查采用 TS 7。
- [TanStack Query](https://tanstack.com/query/latest/docs/framework/react/overview)、[TanStack Virtual](https://tanstack.com/virtual/latest/docs/introduction)、[Radix](https://www.radix-ui.com/primitives/docs/overview/introduction)：按实际需要使用。
- [Vite 迁移指南](https://vite.dev/guide/migration)、[Vitest 迁移指南](https://vitest.dev/guide/migration)、[Query 默认行为](https://tanstack.com/query/latest/docs/framework/react/guides/important-defaults)：升级、WebView与事件缓存的检查依据。
- [OpenAI 官方提示词指导](https://developers.openai.com/codex/prompting/)：任务目标、必要上下文、结果格式、边界和验证；本轮不指定模型或增加 API 依赖。

本地补充研究保存于 .artifacts/product-planning-20260912。后续执行必须核对官方 API 与目标版本，不复制未验证的方法名。本文提出行为契约，不假定每个原生入口都已经由 Wails 封装。
