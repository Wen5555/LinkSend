# R2 publisher owner first source increment, 2026-10-26

先在ADR0016冻结系统publisher合同，再实现最窄owner规则。没有调用系统clipboard/格式注册/GUI、没有运行新的native场景、没有打开Native删除。实际WriteFiles仍是原适配路径，不能以这些源码政策宣称生产Shell已接通。

## 源码与因果

`internal/clipboardnativecontract/lifetime.go`只拥有helper终止和准入政策，不拥有native引用计数、quiescent签名或删除能力。Authorize仅在调用方回读实际durable delegation后登记固定digest；CommitMayStart在第一次控制写之前不可逆进入export_possible。即使之后未得到Held、写失败或main取消，该状态也不回退。AcknowledgeGeneration只接受一次固定generation，供未来已验native ack接线，不能由watcher后续复制替换。

`internal/app/clipboard_cache_native_launch_windows.go`已实际接入该政策：真实SQLite row/delegation复核后才Authorize，控制写前先CommitMayStart；close只有确定尚未提交/未可能export才允许TerminateProcess。此前commit已发但Held尚未确认的取消/失败窗会强杀自有受控helper，现不再如此。

NewShellLifetime在export_possible后不返回任何原生硬deadline；NewControlledLifetime保留原测试deadline。**当前helper仍只运行受控路径，Shell政策/真正无限原生寿命尚未接线**，没有将原30秒直接用作真实Shell消费者寿命，也没有假称已验证真实系统消费者保活。

## 最小验证

日志根`.artifacts/r2-publisher-owner/`：

- `GOWORK=off go test ./internal/clipboardnativecontract -run '^Test(ShellLifetime|PublicationAdmission)' -count=1 -timeout 30s -v`，exit0，1.366s，`lifetime-tests.log`。验证无授权/错digest不可commit、未知commit之后不能终止/到期、旧publication不能重启、后续generation不可替换，以及到期前未export仍可安全终止。这是owner安全政策测试，不是原生refs证据。
- root `GOWORK=off go test ./internal/app -run '^$' -count=1`，exit0，`core-compile.log`，只作编译，不记功能测试通过。
- desktop `GOWORK=off CGO_ENABLED=1 go test ./cmd/clipboardfilehelper -run '^$' -count=1`，exit0，`helper-compile.log`，只作Go/cgo/C++编译，没有执行候选main/helper。
- `GOWORK=off go vet ./internal/clipboardnativecontract ./internal/app`，exit0，`core-vet.log`。不重复原真实进程、压力、race或完整矩阵。

## 接续阻断与明确边界

源级实查：clipboard-file job只向UI返回id/peer/state/字节，不创建普通task/inbox路径历史；普通Enqueue/StartSend/Resume/ResendInbox、ResolveInboxFile的Open/Reveal与OpenTaskDirectory仍可能访问内部缓存，不能仅凭IDataObject不含CF_HDROP取得GC权。须先统一出口保护/真实别名与父目录处理，明确物化的用户副本独立保存；任何路径泄露使对象保持path-protected。

watcher当前只认CF_HDROP；真实publisher必须用自己的实际generation/签名ack和不可逆job完成点处理echo及紧接的新用户复制，未知ack不重签、不路径fallback。现65句柄/64MiB/30秒都是受控预算；原manifest与分页inventory、生产按需source pin/分块stream、大目录和FILEDESCRIPTORW长路径限制尚未实现，不能冒充完整文件剪贴板支持。

正式`windows-virtual-files-v1`尚未被当前receipt-only codec广告或接受。只有独立合同、可信准确helper、真正Shell生命周期、零物理路径出口、实际源关闭、Go活动引用及封存清单/自动保护期均齐备后，才可另外审查连接逐项删除。当前没有手动清理、容量或期限设置，也没有新Wails API/数据库/schema/wire变更。
