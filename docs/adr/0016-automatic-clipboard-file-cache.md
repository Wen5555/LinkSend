# ADR0016：程序自动管理文件剪贴板缓存

日期：2026-10-19。状态：设计冻结，分增量实施；完成范围与未验项目只在唯一 TODO 维护。

空间预留、固定接收Root及不确定发布保护已分增量落实；持久所有权正按下述同一SQLite owner实施。完整启动对账、原生读取结束证明和已发布缓存自动删除仍未启用。设计参数与状态机不能当作全部已有行为；实际范围与测试见唯一TODO及阶段证据。

## 所有权与边界

2026-10-26普通源与OS出口增量：transfer在准备/hash/每次ReadChunk使用借用实际FD的追加式约束，成功Close后才释放；app统一普通/队列/恢复/重发/剪贴板发送/临时分享入口，拒绝当前profile私有缓存树及实际目录别名。选择祖先且排除私有子树仍允许普通用户文件。Open/Reveal/任务目录和excluded preview元数据增加同一拒绝门；这不是持续OS读租约，校验后按path重开窗口仍在，Native删除继续关闭。无配置/wire/schema变化，实际范围与最小验证见[出口证据](../evidence/RELIABILITY-R2-CACHE-EXPORT-GATES.md)。

缓存只包含当前 profile 内由 Go 创建、用于文件剪贴板的一次性接收目录；发送源、普通接收目录、历史和用户副本均不归清理器所有。Service 在现有 profile 锁下拥有缓存生命周期，不增加数据库 owner、前端正文通路或用户配置。旧版无可验证所有权元数据的目录保持原样，不以十六进制目录名推断归属。

新对象使用随机 ID，创建时记录版本、profile 身份、目录文件身份、创建时间、运行代次和状态。既有`desktopStore`借用同一task-history SQLite连接，在metadata的`clipboard-cache/v1/`命名空间保存私有记录，不新增数据库或数据库owner。记录使用现有设备Ed25519身份、固定用途domain签名，严格版本/字段/大小/签名检查，以旧完整值作CAS条件，再回读验证；不从库内字符串重建删除路径。实际清单、接收计划摘要、引擎当前持有的stage/part句柄身份在接收正文前持久化，发布意图在原生写入前持久化。持久记录不等于可删除授权；未知或不一致记录保留。

引擎Go-only `StorageReady`通知在首次计划checkpoint及后续计划变更后、accept/body之前传递借用的Root/Stage/实际文件句柄。owner必须核对当前part名字仍对应引擎打开的同一inode，不能只打开一个可被替换的目录名。句柄不得关闭、写入或暴露给JS/信令。数据库发布意图在权限和剪贴板状态锁外写入，随后仍执行最新授权、取消、原生generation及deadline检查。

`receiving → verified → publishing → published|protected`记录接收和原生阶段；尚未调用原生写入的失败可在确认本次对象移除后忘记记录，删除失败记abandoned。运行代次只赋予当前worker完成/忘记其自身记录的权限；重启后旧记录不能借丢失的内存native-start标记重获未发布或发送权限。无法确认最终发布结果的崩溃一律按“可能已发布”保护，不能因一次失败抹去publishing意图。

未发布对象现已接入`reclaimable → deleting → removed`：仅当前worker仍能证明未调用原生写入时，在接收器关闭后签名封存完整清理清单；清单来自引擎关闭前的实际part、checkpoint及已创建目录句柄，提交文件还须与part为同一inode。原生调用后和结果不确定的对象不进入该状态。封存清单包含每个相对条目的文件身份、类型及普通文件大小/mtime；未知条目、链接、替换或可见用户改动均保护整个对象。删除逐项执行，不递归RemoveAll，先验证完整树，再核对每个固定父目录及条目；中断留下同一deleting记录，只允许继续其仍匹配的剩余条目。

同一Service的活动引用在注册前建立，持续至接收器、清理与原生发布返回且目的目录句柄实际关闭；ctx取消和UI终态不释放引用。扫描与引用建立互斥，退出先停止任务、加入worker与扫描器，再释放共享数据库和profile。重启只继续签名且已封存的未发布意图，不从旧运行丢失的RAM标记推断是否曾发布；旧receiving/abandoned且未封存的非空对象仍保护，不能按名称补认领。完整发布对象的原生读取结束许可仍须独立实施。

## 原生读取的保守安全界限

当前 Windows CF_HDROP / macOS file URL 发布没有可靠的“Finder/Explorer 已完成整个目录粘贴”回执。剪贴板变更、应用退出、TTL、某一时刻无打开句柄均不足以证明系统复制已结束。因此首版不以这些条件单独删除 published/publishing 对象：同一次系统启动内继续保留，包括应用重启与网络变化。

设计复核修正：kernel boot identity只能说明旧启动的进程已结束，不能排除系统恢复剪贴板路径后、应用启动前又开始的新粘贴；因此boot变化加“当前无引用”也不能单独授权删除。发布对象必须另外具备受控原生交接/读取结束证据，并排除当前引用和在途消费者后，才可进入reclaimable。缺少可靠回执时继续保护，不能以启动标识、墙钟或TTL推断安全。不请求用户重启，也不把重启当作正常错误解决提示。

原生设计须分别实测：Windows的[IDataObjectAsyncCapability](https://learn.microsoft.com/en-us/windows/win32/api/shldisp/nn-shldisp-idataobjectasynccapability)提供可选的StartOperation/EndOperation协商，不能假设每个消费者都会使用；Mac的[NSFilePresenter](https://developer.apple.com/documentation/foundation/nsfilepresenter)仅通知协调访问，不证明任意低层读取已结束。[文件承诺](https://developer.apple.com/documentation/appkit/supporting-drag-and-drop-through-file-promises)文档以拖放为例，不可据此声称Finder键盘粘贴已通过。必须验证实际支持的原生交接及退出/重启持久性后再启用对应回收许可；未取得许可的发布对象保守保留，R2仍不能因此宣告整体完成。

即使跨启动，当前剪贴板仍引用的对象也必须保留；读取忙、generation变化、格式不可判定均视为引用未知，暂不回收。检查只读取文件路径元数据，不读取或导出用户正文。第三方剪贴板历史无限期保存旧路径不在承诺内，但不因此缩短系统粘贴保护。

一旦调用原生写入，返回错误也可能意味着路径已对系统可见。发布结果不确定时保留已验证缓存；只有能证明原生写入尚未尝试的失败才能沿未发布清理路径处理。接收和checkpoint通过存储owner持有的`os.Root`子句柄，目录字符串只作本地显示；发布前复核可见profile/cache/object与原句柄的文件身份，不能重新解析被替换的绝对路径作为写入授权。

正常回收条件为安全无引用且创建后至少7天；空间压力仅可提前到安全无引用后至少24小时。时钟回拨、未来时间和缺失时间均延后回收；时钟快进不解除 boot、引用、所有权与原生读取保护。未发布失败对象不需要该保留期。

### 2026-10-26 Windows 隔离原型的实现边界

已在本机独立 `.artifacts/r2-native-reader-draft` 验证三阶段：同进程真实 COM consumer 166条trace、真实 STA producer与两个MTA进程的标准COM marshalling 496条、launcher正常退出后的独立STA helper服务与退出门499条。持有的`IDataObject`、stream/clone、async operation及活动调用都参与保护；EOF或`EndOperation`不提前产生quiescent。最后所有源引用结束且源句柄关闭才产生每个publication唯一witness。严格构建、实际进程退出、哈希、保留源文件与早期编译/CRT入口失败见[原生引用原型证据](../evidence/RELIABILITY-R2-NATIVE-REFERENCE-PROBE.md)。

这些原型从未调用系统剪贴板/native publication、生产adapter或回收器；本轮只读clipboard序号均958→958，没有删除任何源夹具或改动生产权限。生产仍为Windows CF_HDROP/macOS file URL，旧published/publishing/protected状态与删除门保持。原型plain JSON witness不构成生产删除许可；签名delegation、安全inventory交接、持久签名receipt及Go/SQLite owner的验证/CAS尚未实现，不能以计数或进程退出补出许可。

helper实验只证明launcher exit 0后，**仍存活的COM producer/helper**可以继续服务两个真实consumer，并在全部源引用/consumer结束后有界退出；没有证明producer/helper自身退出后源stream可用。有限句柄继承/观察不是完整安全沙箱或安全交接。Explorer Ctrl+V、实际clipboard owner变化、正常退出/重启/升级时的原生可用性及macOS Finder均未通过本实验验收，R2仍未整体完成。

下一步先冻结新publication的无物理路径出口合同及退出owner，完整审查格式、Stat、Shell fallback和Open/Reveal/drag/history出口；任何cache物理路径外泄都使整个对象继续path-protected，不追认旧发布。再实现同一Go owner验证的用途签名delegation、固定身份inventory和有界持久receipt；缺失、半写、签名不符、旧回调或崩溃一律protected。取得真实Explorer/Finder键盘、目录/空目录/长路径、重复/并发消费者与退出/重启/升级证据后，才可把新合同的可验证quiescent与原封存清单、活动引用、自动保留期和逐项身份删除门连接，验证真实自动回收。维持无手动清理、无容量/期限设置。

## 2026-10-26 Go receipt-only 私有授权增量

本增量只提供私有 delegation/receipt 的签名、绑定核验与同 SQLite 单次 CAS，不接系统剪贴板或 `WriteFiles`，不启用 published 回收。沿用 schema12、原 profile Ed25519 身份、同一 metadata 连接及 owner；v1 私有记录新增可选 `native` 字段，nil 时原序列与签名须由 golden 实证保持。未知、重复、非规范序列与篡改记录拒读并保留，不重签认领旧资料。

delegation 固定用途域，绑定原 OwnerRun/BootID、cache/profile/root/object/stage 文件身份、完整 inventory/manifest/plan 摘要、随机 publication/nonce 与代次、helper image SHA256 和 child ephemeral 公钥。原 receiver 完全关闭后才能签名封存其实际 inventory。当前没有实际 launcher、稳定 child/image 句柄及 challenge 核验实现；私有入口只接受未来强类型验证凭据，当前生产没有该凭据的实现或构造路径，不能靠调用方自报 hash/布尔值建立可信授权。child 私钥只允许未来 Go helper 内部使用标准 Ed25519，不交给 COM、IPC 或主 owner。

receipt-only 的 Go 验签只证明签名出处与原绑定相符，不证明 native 引用语义。未来可信 helper 必须在唯一内部路径永久关闭新准入、全部真实 provider/marshal/stream/clone/async/活动调用结束且源句柄实际关闭成功后，才签发一次 quiescent receipt；valid-child-key 加虚报关闭、plain trace、EOF、EndOperation 或进程退出都不取得删除许可。实际 helper/image/challenge、平台 BootID provider、持久 spool 和无物理路径出口合同尚未实现。

新 main 可在同 profile 锁下消费匹配原签名 row/delegation 的旧 OwnerRun 原 publication 回执；独立 consume 不放宽现 `receipt.save` 的当前 run 限制，也不重新签发旧授权或认领新 publication。原 raw CAS、同一事务回读与提交后回读绑定单次消费摘要/首次观察时间；活动引用、根或条目身份变化、冲突、错签/重放、半写或失败均保护。boot 改变本身不授权；没有可验证的原 receipt 时始终保护。

`native` 非 nil 的 receipt-only 对象在扫描和逐对象 collector 两层都保持保护，即使已 sealed、老化、压力触发或模拟 boot 变化也不得进入现删除通道。原 CF_HDROP/file URL、legacy、unknown 不调用或追认新入口。未来须以新的受支持 native contract、新 publication 及真实系统消费者/正常退出实验证据，另行审查后才能接原引用、保留期及逐项删除门；本阶段测试中的 child 关闭声明不是生产回收权。

## 2026-10-26 Windows私有受控broker首增量

上述receipt-only授权基础当时无launcher/boot实现；本次新增私有源码实现与独立helper，仍无默认publisher调用或发行包trusted-image锚。launcher持只读且拒写删的image句柄，按明确的可信预期SHA256验证；用CreateProcess返回的稳定hProcess挂起启动，核对实际process image对应同一文件后才恢复。私有本机first-instance管道两端由owner先建，只继承精确管道/源/回执/launcher寿命句柄；child签随机challenge、公钥、实际BootIdentifier与inventory摘要，自报hash/PID不构成授权。

owner按receiver已关闭后的完整sealed inventory重验实际身份/类型/size/mtime，文件只读pin拒新写删并核对manifest内容；只给helper最小只读源句柄、相对描述符和一个已创建的spool写句柄，不传profile key、DB或profile目录。helper重验继承句柄及内容，产生原生对象前必须收到与原inventory/manifest/child绑定且profile签名有效的delegation；launcher仅在原SQLite完整row/CAS回读仍匹配后commit。错序、半写或失败保持保护。源清单最多65句柄/64MiB及相对描述符小于260个UTF-16单元是这个私有受控broker的预算，不缩减原文件协议，也未自动改走CF_HDROP。

Go child签名路径仅在本进程C++ opaque witness门真正消费成功后签固定receipt；C++ COM方法使用HRESULT/noexcept及失败保护，全部provider/marshal/stream/clone/async/活动调用结束、逐源CloseHandle成功才quiescent。受控标准STA↔MTA消费者关闭后，回执经预先打开的私有spool句柄写入、Sync/关闭，再由现有profile/SQLite owner核验消费。没有系统clipboard/注册格式、物理路径发布出口、Native删除或手动缓存设置；实际GUI/launcher进程退出仍有活跃consumer、Explorer互操作、升级与最终自动回收须后续独立证据。准确范围和一次最小验证见[证据](../evidence/RELIABILITY-R2-WINDOWS-NATIVE-BROKER.md)。

## 2026-10-26 真实launcher进程退出与新main消费

私有本机launch合同升级到v2：prepare固定原绝对deadline与是否等待原launcher成功退出；child challenge签入整个canonical prepare摘要，Held再绑定同一launch/delegation摘要和期限。旧launch v1与v2混用严格拒绝，仍须匹配准确可信helper镜像；profile delegation/receipt保持v1、schema12与现有签名字节不变，不重新认领旧publication。

helper在原delegate持久并核验后创建真实STA↔MTA consumer，实际持有provider、stream/clone和async；native只读Held门不接收refs、trace或任意签名payload。launcher收到并验签Held且helper仍存活后才能detach；已detach的close只释放owner本地句柄，不强杀helper。helper随后等待原稳定launcher进程句柄实际exit0，期限未延长才允许consumer继续真实读完；非零退出、原deadline、关闭失败均不签，source与Native记录继续保护。成功回执写原spool后helper正常exit0，新的实际main进程重新加载原profile身份、同SQLite和旧OwnerRun回执进行单次消费。

这里的30秒硬deadline、同helper内受控MTA consumer，以及commit已发送但Held尚未确认的失败窗可终止helper，均是当前受控合同边界。没有宣称取消失败期间活跃consumer保活；不得把该超时/终止策略直接用于未来系统Shell publisher。系统clipboard/正式格式/Explorer、升级与已发布自动删除仍需另行实现和真实证据。最小一次正例/必要非零退出负例与独立只读审查见[进程证据](../evidence/RELIABILITY-R2-NATIVE-PROCESS-LIFECYCLE.md)。

## Windows系统publisher接入合同（实施前冻结）

正式方向采用独立的`windows-virtual-files-v1`合同；当前受控`receipt-only-v1`对象和旧CF_HDROP/file URL永不追认。先冻结生产owner/出口/寿命边界，再接实际系统publisher；未取得真实Shell及无路径出口证据前，所有Native仍由现collector保护，不能因源码状态机或签名齐全宣布可回收。

1. **准入期限不是源寿命。** 可信image/child/源inventory准备和最后授权/generation CAS可有绝对期限；只要commit可能发出或原生export可能开始，owner即进入不可逆的`export_possible`。取消、IPC错误/响应丢失、main正常退出、升级与准入期限都不得强杀该helper。准备阶段尚可证明未export时允许终止；正式Shell helper在export后没有30秒或其他硬TTL。Native I/O错误/未知状态保留，不把helper消失、boot变化或墙钟代替最终源结束证明。
2. **唯一最终原生门。** helper独立STA持有实际IDataObject与消息泵，提供FileGroupDescriptorW/FileContents/IStream/相对Stat和copy effect，不给CF_HDROP、TYMED_FILE、物理路径或读写源句柄。实际clipboard ownership丢失只允许关闭新factory并释放helper自己的clipboard控制ref，已有provider/marshal/stream/clone/async/活动调用继续有效；全部实际引用和源句柄成功关闭才签一次原publication的终静止回执。COM消费者异常/未知引用宁可继续保护；不得通过超时或关闭布尔值补出许可。
3. **原授权和echo竞争。** 完整源准备/持久CAS在grant/state锁外；实际系统mutation仍在现接收授权revision、取消、新复制、generation和准入deadline门内。只接受固定helper签发、绑定本次publication和实际OS generation的成功ack；成功点在watcher可推进新本机事件前固定job完成，之后新复制只淘汰新的等待内容，不倒改已完成任务。ack丢失保持publication uncertain且helper存活；不得退回路径发布。helper自己的generation沿现State去重，未知/其他generation按新本机事件处理。
4. **全部路径出口。** 文件剪贴板job当前只向UI投影id/peer/state/进度，不创建普通task/inbox路径记录。但普通队列/StartSend/历史重发可重新读取路径，Inbox Open/Reveal和OpenTaskDirectory会交OS，原生拖入最终经过同一发送入口；接通新合同前须统一拒绝内部缓存路径及其目录祖先/真实别名，或在真正独立用户副本中完成明确物化。任何实际路径外泄或未经owner跟踪的读取都令整个对象保持path-protected；不会给该对象事后补无路径证明。
5. **目录和大文件范围。** 生产流按原manifest逐项/分块服务，不把65句柄、64MiB正文、30秒测试寿命当产品上限；需分页封存inventory并按实际stream/clone引用懒开源、校验固定身份/内容，内存和在途I/O有界，64位size/seek覆盖原文件协议。FILEDESCRIPTORW的相对名字固定260 UTF-16和当前1024-node封存上限是尚未接通的阻断：超界不得截断、漏目录、缩协议或降级CF_HDROP后授GC权。长路径/完整大目录在有可验证实现前明确保持不支持虚拟发布且对象受保护，不宣称原文件剪贴板完整范围已通过。
6. **自动回收的最后连接。** 只有新的受支持合同、可信准确helper、零物理路径出口、原publication最终native静止证据、Go活动引用结束及原sealed inventory均齐备，再沿现同SQLite owner的首次静止观察/自动保护期/压力预算、逐项身份核验和删除意图CAS接回收。不增手动清理、容量或期限设置；任何一项缺失、回拨、损坏或重放继续保护。当前不开放该删除连接。

首个源码步骤只落实不可逆export intent与helper退出策略，纠正commit已发/Held未确认的错误强杀窗；随后实现统一缓存路径出口保护与真正OLE publisher。实际WriteFiles仍待按本合同接线，当前不运行系统clipboard/GUI或新原生场景。

### 2026-10-26 尚未接线的Windows Shell primitives

独立私有Go v3分页codec追加在同一合同：固定首帧/总数/摘要和64KiB页，64MiB只约束metadata；旧controlled v2入口不变。完整manifest/inventory/source同序、实际身份格式及用途签名内部一致性通过后才返回整个catalog，不能把合成metadata、声明文件大小或caller句柄值当真实FD证据。该增量未升级app原1024-node持久inventory、未接launcher/helper或GC，亦未解除下述generation发布门，见[codec证据](../evidence/RELIABILITY-R2-SHELL-CATALOG.md)。

独立C++ `ShellSession`/`ShellWitness`候选使用最小只读FD副本、正式虚拟文件格式和真实引用结束门；可能export后没有硬期限/强杀出口。一次严格构建及原controlled COM/source-close回归通过，没有调用新Shell函数、系统clipboard或GUI。生产Go仍只接receipt-only，Native删除保持关闭。构造可容纳10,000发布条目但FILEDESCRIPTORW相对名字仍少于260 UTF-16单元；分页handoff、完整inventory/长路径和真实Shell范围未完成，见[候选证据](../evidence/RELIABILITY-R2-SHELL-PRIMITIVES.md)。

最后generation检查与OleSetClipboard为独立调用，稳定准确对象ack不能补成原子CAS。官方desktop合同未保证自有非空HWND外层OpenClipboard与OLE内部window重入/保锁；Windows CE说明和兼容源码不能替代该保证。此缺口解决前禁止激活publisher；禁止发布后回滚。若需最窄实测，仅复用既有GitHub-hosted Windows空clipboard隔离入口，失败保守拒绝且不卸锁重试。本机/其他runner不运行该probe，单次成功也不能冒充真实Explorer或普遍平台保证。

## 自动空间预留

接收前以实际经过文件协议验证的 manifest 计算所需内容字节，不能相信 files 事件的 Bytes 提示。每个缓存接收并发预留全部正文、每条目4KiB元数据估计及16MiB事务余量；卷上另外保留自动计算的余量 `max(64MiB, min(1GiB, available/100))`。两路接收共享同一预留 owner，校验剩余空间时扣除其他在途预留；已写入字节仍不释放预留，采取保守上界。

预算不是磁盘 quota 保证。接收中真实 ENOSPC 仍由既有引擎终止、关闭句柄、禁止发布和回收未发布残留。失败、取消、提交后均恰好释放一次内存预留；进程重启从实际卷可用空间重新计算，不恢复旧传输或旧复制。

不足时先触发有界安全回收，再重新核对；仍不足则拒绝新复制，提示“空间不足，暂时无法复制文件”，保留当前剪贴板和其他活动对象。无手动清理按钮、容量/期限设置或删除确认；同类错误只保留当前状态，不循环弹窗。

## 清理与崩溃对账

启动、任务结束、剪贴板变更、空闲与空间压力触发同一 owner 的有界回收。每轮最多检查64条、尝试删除8个对象（失败也计预算），普通周期至少5分钟；压力触发至少间隔30秒。单轮metadata累计读取至多4MiB，过大行在SQL侧按字节拒读；查询游标保证前页受保护对象不饿死后页。单轮合作式截止2秒，每个完整清理树至多1024条目、关闭观察器额外目录句柄至多64个；超过预算保留对象，不缩小文件协议的接收数量。同步文件系统调用不能被context强制中断，不宣称硬实时。大对象的增量封存与未封存崩溃残留后续继续，不把当前保护当作完整回收通过。

失败按对象指数退避，最大1小时，最多8轮后等待下次启动或新的空间压力；重试表最多256项，超过表容量的失败仍受全局分页/周期预算，不忙循环。墙钟回拨延后重试，快进不解除发布/所有权保护。删除后忘记元数据失败保留deleting意图，下轮确认相同cache根中对象已不存在后才CAS忘记。

只通过持有的 profile/cache `os.Root` 操作已核验对象，不从远端路径拼绝对清理目标。根与对象都核验真实目录身份并拒绝符号链接/reparse point；删除过程中不跟随链接。清单外文件、身份替换或用户更改一律保留。删除先持久化 deleting，再对已验证的条目逐项删除；崩溃重启仅继续相同签名记录中的剩余对象，未知残留不扩大删除范围。不得递归扫描用户目录或按名字删除旧版缓存。

兼容增量：现有私有记录v1新增可选sealed/cleanup字段和reclaimable/deleting状态，旧记录未封存时保持原签名序列与保护行为。更早的严格解析器会拒绝新字段/状态并保留对象；不能把拒读当作无所有权从而删除。仍使用同一metadata命名空间及schema12，不改变wire、网络信任或用户配置。

## 验证与兼容

不改 V1 文件 wire、clipboard_files_v1、授权默认值或既有任务库 schema。元数据有独立严格版本；旧版不会自动清理新对象，回退保护已发布路径。未来若需修改数据库必须使用既有备份迁移。

本批启用与回退：沿用schema12既有metadata表，只增`clipboard-cache/v1/`私有键，首次对象注册以单SQLite提交创建，其他现有键不改。缓存记录版本1与DB schema不是同一版本；未来/损坏记录只保护，不能重签并认领。兼容schema12的旧程序按原key读取设置/历史并忽略该命名空间，旧程序也没有发布缓存自动回收器；更早schema11程序的拒绝边界不变。程序回退和数据回退分开，不能用旧备份覆盖升级后的新任务/历史写入。准确旧包安装/回退旅程仍留R4，不用源码兼容检查冒充已实测安装。

测试必须实际核对保护文件仍存在且可用，覆盖：并发预留/释放、恶意大小提示/manifest、ENOSPC、取消、新复制、发布临界崩溃、应用重启、当前引用/未知引用、同启动任意TTL、跨启动安全无引用、墙钟跳变、删除退避、未知对象、签名/身份替换、symlink/reparse与根逃逸；并发删除及生命周期运行 race。Windows/Mac 原生系统粘贴与实际自动回收另验，注入 boot/容量/时钟不冒充真实系统重启或 Finder/Explorer 操作。

## Windows同次构建锚（2026-10-26增量）

可信helper摘要必须由同一构建事务读取实际静态CGO镜像后编入CGO0桌面，同时固定平台、源提交、完整约定构建输入摘要和真实clean状态；输入在前后变化则不生成打包metadata。私有resolver只按自身EXE目录与内置摘要派生固定路径，验证实际PE/hash并拒绝reparse；不能把可写BUILD-INFO/sidecar、用户路径/hash、环境或helper自报当授权。该resolver只提供未来core pin/recheck所需镜像信息，不自行发布、启动或接Wails API。

本次只支持Windows原生构建的ZIP/NSIS同候选载荷；按摘要分目录，installer只复制当前文件，uninstaller只精确删除当前文件及空目录，旧helper/未知内容保留。这样不证明同摘要活跃consumer安装锁已解决，也不赋予Native缓存删除资格。实际dirty构建、静态包/必要负例和准确来源边界见[构建证据](../evidence/RELIABILITY-R2-HELPER-BUILD-ANCHOR.md)；系统publisher/Explorer、安装旅程及完整R2继续待实现/验证。
