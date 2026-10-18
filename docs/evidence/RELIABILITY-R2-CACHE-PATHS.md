# R2：固定目录与不确定发布保护（2026-10-19）

基线 `33edf10d13acbb67a5b4ba771d954e43acea8f0d`，前一个产品源码为 `f2712dccce6f6b7e90f9a7af6e52c40aefae283f`。本批修复已对系统可见文件可能被删的缺陷，并固定缓存接收目录；完整自动回收仍未实现，不替代R2的原生粘贴或回收验收。

## 原代码失败

`TestClipboardFilesUncertainNativePublicationKeepsVerifiedPaths` 使用原有真实认证QUIC文件夹具，在内存原生adapter接收路径并推进generation后，注入最终确认错误。原代码把返回错误等同于“未发布”，递归删除了已验证文件；测试实际退出1：`uncertain native result deleted an OS-visible verified file`，日志 `r2-uncertain-publication-before.log`。这是原生边界注入与真实文件系统反例，不冒充Windows/macOS系统剪贴板已经实测此故障。

## 实现

- 一旦调用原生WriteFiles，无法确认结果的错误也保留已验证缓存；只有尚未尝试原生写入的失败走未发布清理。仍完整验证、确认并复核授权/CAS后才调用原生写入。
- 增加安全码 `file_copy_publication_uncertain`，设置和大文件窗显示“系统剪贴板状态未能确认，文件已保留”；不能再断言“文件未写入剪贴板”。取消意图与旧任务不能覆盖新任务状态，成功的新复制只清除自己的旧错误。
- 缓存目录由同一profile owner按profile→cache→新对象逐级固定`os.Root`，拒绝已有对象和symlink/reparse目录，核对打开前后文件身份及新对象为空。发布前再核对可见路径仍指向原目录。
- 文件引擎增加Go-only `ReceiveOptions.Root`（JSON排除）。设置时checkpoint预读、接收和提交始终用其子句柄，关闭只释放借用的子句柄；目录字符串不能重新授予另一个路径写权限。普通调用默认nil，原行为和V1 wire不变，无新RPC/capability/schema。
- 未发布清理仍限于当前worker创建的对象，使用固定cache句柄并先复核对象身份；本批没有启用已发布对象自动删除，尚不能据此宣称完整清理竞态验收。

## 已取得的验证

- 定向普通3轮（app14.255秒）通过：不确定发布路径实际仍可读、稳定原因、原状态隔离、认证复制、空间不足与新复制恢复、四种取消边界。
- Root引擎回归完成真实帧/文件引擎传输：有伪造checkpoint的无关目录保持原样，内容及中文空目录只出现在借用root；关闭root不会回退到目录字符串。该回归使用net.Pipe，单独证明文件系统授权边界，不充当网络证据。Root定向race3轮退出0（6.092秒）。
- Windows本地三项路径专项通过（`r2-cache-paths-native.log`）：已有未知目录/用户文件保留，NTFS junction被拒绝且目标目录未写入；Windows拒绝重命名固定cache，故本机没有执行“替换成功后删除原对象”的分支，该分支留给Linux CI。
- 前端typecheck/lint/test/build全部退出0，16文件/127项；渲染回归确认不确定状态不会再出现“文件未写入剪贴板”。日志 `r2-paths-frontend-*.log`。前端先完成dist，再做最终Go embed检查。
- 最终两模块workspace/off的 `go test ./...`、`go vet ./...`、`go build ./...` 共12条命令均退出0；日志 `r2-paths-final-{core|desktop}-{workspace|off}-{test|vet|build}.log`，核心workspace app94.554秒。最终app组合race3轮退出0（23.836秒，`r2-destination-final-race.log`）。前端dist已先完成，未并发改写dist与最终embed构建。

日志目录 `.artifacts/reliability-20261019/`。无日常剪贴板读取/覆盖，无用户安装、生产或宿主网络变更；Windows junction仅在`testing.T.TempDir`内创建，不申请系统symlink权限。

## 设计修正与下一步

复核[ADR0016](../adr/0016-automatic-clipboard-file-cache.md)发现boot变化也不足以证明安全：应用重启前，系统恢复的旧路径可能已开始新的粘贴。已纠正此假设；不得靠boot+TTL删除发布对象。Microsoft文档的异步数据对象协商是可选的，Apple文件协调只覆盖协调读写，文件承诺文档的拖放示例也不证明Cmd+V。原生交接与读取结束证明、持久所有权/崩溃对账、有界自动回收和实际双平台粘贴仍须继续。

普通原生错误不能随意消耗更高generation，否则会吞掉用户的新复制；后续原生结果契约需区分“确定未写入”“可证明的本机变更”“结果未知”，保持新复制与防回环边界。本批只解决缓存保留与准确状态，不声称该扩展契约已完成。

## 准确提交与CI

源码 `bed8d2c651c9f8c7512b3884a7c096b2efaa387c` 已推送，远端SHA一致。[core](https://github.com/Wen5555/LinkSend/actions/runs/39167827133)、[desktop](https://github.com/Wen5555/LinkSend/actions/runs/39167827116)、[三平台包](https://github.com/Wen5555/LinkSend/actions/runs/39167827151)均success，Windows/Mac arm64/Mac amd64三job逐项核对。Linux core完整test/race包含实际目录替换分支；这不替代Finder/Explorer键盘粘贴、安装或跨网旅程。未合并main、发布或替换生产。
