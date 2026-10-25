# ADR0016：程序自动管理文件剪贴板缓存

日期：2026-10-19。状态：设计冻结，分增量实施；完成范围与未验项目只在唯一 TODO 维护。

空间预留、固定接收Root及不确定发布保护已分增量落实；持久所有权正按下述同一SQLite owner实施。完整启动对账、原生读取结束证明和已发布缓存自动删除仍未启用。设计参数与状态机不能当作全部已有行为；实际范围与测试见唯一TODO及阶段证据。

## 所有权与边界

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
