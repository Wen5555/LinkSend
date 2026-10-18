# R2：确定未发布对象的活动引用与自动回收（2026-10-19）

基线 `c4910f97fa929fc92257170721ac12724989d644`。本批接入活动引用、关闭前来源观察、未发布清单封存与有界自动回收。**不代表完整R2完成**：已发布对象仍需原生读取结束许可，真实Finder/Explorer键盘粘贴、双平台系统重启引用及原生回收旅程仍待。

## 行为与安全边界

- Service复用既有profile锁/共享SQLite owner。缓存注册前建立活动引用，直到接收器、发布/失败处理与固定目的目录句柄实际关闭才释放；不以ctx取消或UI终态推断worker退出。收集器与引用建立互斥，退出加入收集器后才关闭DB/profile。
- 新Go-only `StorageClosing`在接收器关闭前观察实际part、checkpoint与本次创建的目录句柄。checkpoint须与引擎最后成功写入的字节相同，目录须匹配运行中创建记录，正式文件须与实际part同inode；未通过检查不能获得清理权限。句柄不进入wire/JS。
- 只有当前worker证明从未开始native时，在引擎关闭后签名封存完整清单，进入reclaimable/deleting。普通文件同时记录大小/mtime；未知文件、链接、身份替换和可见用户改动均拒绝整对象清理。取消时不用已取消ctx立即放弃必要收尾，而用独立有界清理ctx。
- 先核对整棵已登记树，再逐项核对固定父Root和条目后unlink；不用RemoveAll。中断后仅继续同一签名清单的剩余条目；删除后忘记DB记录失败则保留deleting，下轮确认同cache根中对象缺失后CAS忘记。
- 启动、任务退出、剪贴板变化、5分钟空闲周期及空间不足触发同一收集器。每轮64条、至多8次对象删除尝试（失败也计）、4MiB metadata、合作式2秒，压力限频30秒；分页游标避免前页保护对象饿死后页。失败指数退避至1小时、8次后等新启动/压力，最多256项重试状态。
- 每次完整树封存至多1024条，关闭观察器额外目录句柄至多64个。超过预算不缩小文件协议/产品接收数量，接收完成仍可发布，缺少完整封存的对象继续保护；大对象增量封存及未封存崩溃残留回收仍需继续。同步文件系统调用本身不可被ctx强制中断，不宣称硬实时。
- published/protected/publishing、未封存abandoned/receiving、损坏/未来/冲突记录与无记录目录均不获得删除授权。任何TTL、墙钟快进或boot变化均不能解除可能已发布保护。没有新增用户清理按钮、容量/期限设置或手工删缓存流程。

## 实际验证

本地 Windows/PowerShell，使用锁定Go1.27.1/Node24.21.0/pnpm12.4.1；测试只使用专属临时profile及自有文件，无日常剪贴板、安装、服务器、宿主网络变更。

- 先修正旧的目录整体递归删除为无清单时仅移除空目录；新增真实认证QUIC失败旅程：在native前故意使generation冲突并加入未知文件，接收失败且OS未收到路径，已接收正文和未知内容都保留；测试撤去自有未知条目后，压力扫描自动移除已封存残留与记录。
- 当前worker引用在ctx取消后仍保护实际文件，关闭句柄并幂等释放后才回收。70条分页验证前64个受保护对象不饿死后6个；10对象每轮8次上限、5分钟普通限频、删除后SQLite trigger失败、重试/时钟回拨与快进、8次上限、真实profile重新New后启动继续封存意图且不重放复制均有回归。
- 未知文件、普通用户改动、文件inode替换、符号链接、超大Unicode metadata、签名篡改均有负例；断言被保护的文件内容仍可读。符号链接创建在Windows无权限时该分支SKIP，不计Windows symlink PASS；已有NTFS junction根边界回归继续覆盖。
- 实际目录树含70个空目录及正文，经认证QUIC完成并发布，证明可选观察器预算不会拒收正常较大目录集。测试的native adapter为内存函数，不冒充Explorer/Finder系统粘贴。
- 引擎net.Pipe旅程核对借用目录/part/checkpoint在回调内真实可用、返回前均关闭；篡改checkpoint或替换已创建目录时不授予来源且外来内容不变。net.Pipe只证明引擎资源边界。
- 初轮内部stage名被普通用户路径策略拒绝导致清理记录回读失败，日志`r2-lifecycle-first.log`保留；修正为仅精确允许本对象stage及已登记part/state后，普通3轮通过（`r2-lifecycle-security.log`，app14.963秒、transfer1.145秒）。分页回归最初未越过受保护页，拆分夹具后保留游标测试，不绕过产品页数预算。
- 初轮Windows关闭句柄测试错误地只接受os.ErrClosed，而平台实际返回ERROR_INVALID_HANDLE；改为先证明回调内句柄可用，再证明返回后不能Stat。定向transfer race3轮退出0（`r2-lifecycle-transfer-race.log`，6.572秒）。
- app文件/缓存/退出组合race3轮退出0（`r2-lifecycle-app-race.log`，90.230秒）；含封存、未知保护、正常大目录和超大行/篡改的最终组合race3轮退出0（`r2-lifecycle-final-race.log`，app43.484秒、transfer4.124秒）。最后合作式封存截止和额外目录句柄上限以最终普通矩阵/CI为准。

最终两模块workspace/off的`go test -count=1 ./...`、`go vet ./...`、`go build ./...`共12命令全部退出0，日志`r2-lifecycle-final-{core|desktop}-{workspace|off}-{test|vet|build}.log`。核心workspace完整test111.04秒、off106.78秒；桌面workspace完整test16.43秒、off12.30秒。最终源码定向race3轮也退出0（`r2-lifecycle-committed-source-race.log`，app43.659秒、transfer3.864秒），gofmt显式文件清单及`git diff --check`退出0且无输出。此前PowerShell通配符未展开的gofmt检查报错已用显式清单修正，不能以随后的test退出码掩盖该检查错误。

前端未改，复用`bed8d2c`已通过的127项/typecheck/lint/build与`c4910f9`准确desktop CI；本批不改dist，不用旧包结果抵扣原生验收。新提交/CI在推送核对后单独追加，不预记PASS。

日志：`.artifacts/reliability-20261019/r2-lifecycle-*.log`，早期失败单独保留，不把失败日志算PASS。

## 来源核对与接续

基线`c4910f9`三组CI现均completed/success：core [39170491604](https://github.com/Wen5555/LinkSend/actions/runs/39170491604)、desktop [39170491601](https://github.com/Wen5555/LinkSend/actions/runs/39170491601)、packages [39170491614](https://github.com/Wen5555/LinkSend/actions/runs/39170491614)，Windows amd64及Mac arm64/amd64三个包job全部success。它们是上一持久owner增量的准确构建，不是本批CI或原生通过。

继续实现安全的大对象/未封存崩溃对账、可靠原生交接结束许可及双平台真实键盘粘贴/重启/回收。R1物理LAN/恢复/跨网、R3诊断、R4安装退出、R5临时分享与R6同候选收尾保持必做，不用本批局部回收抵扣完成。
