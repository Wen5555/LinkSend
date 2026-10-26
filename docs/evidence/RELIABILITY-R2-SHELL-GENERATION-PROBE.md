# R2 one-shot hosted OLE outer-lock probe, 2026-10-26

状态：**NOT_RUN，等待准确提交的 GitHub-hosted Windows CI 结果**。本机仅编译测试二进制，没有运行测试 / 探测、系统 clipboard 或 GUI。该探测只测 Windows API 的最窄组合，不激活生产 publisher 或 GC，也不以单次成功声称取得全局 CAS 保证。

## 唯一入口与准入

复用 `.github/workflows/wails3-packages.yml` 的现有 `TestDesktopTemporaryCodeNativeClipboardDoesNotSyncOrCache` 单一入口、原 GitHub-hosted / Windows 双重上下文检查和明确建立的空 clipboard fixture，不新增 workflow、job、测试矩阵或 helper 命令。现有真实临时分享测试既不失败也不 skip、且先恢复该空 fixture 后，才在同一锁定 STA 的最后一个 defer 调用 test-only `runHostedShellGenerationProbe`。

探测自行再次核对 GitHub Actions、runner / context 两组 github-hosted / Windows 值和原 native opt-in；`LINKSEND_SHELL_GENERATION_PROBE=1` 单独不足以通过本机准入。创建自己的非空 HWND 并持有外层 OpenClipboard 后，核对当前公开 locker、空格式集合和非零 generation；未知、忙、上下文不符或设置失败均在 mutation 前拒绝。只查看格式 / generation 元数据，不备份或记录用户正文。

## 一次调用与记录

采用官方 [CreateDataCache](https://learn.microsoft.com/en-us/windows/win32/api/objbase/nf-objbase-createdatacache)、[IOleCache::Cache](https://learn.microsoft.com/en-us/windows/win32/api/oleidl/nf-oleidl-iolecache-cache) / [SetData](https://learn.microsoft.com/en-us/windows/win32/api/oleidl/nf-oleidl-iolecache-setdata) 的标准 IDataObject，仅持固定合成 CF_UNICODETEXT；没有手写 COM callback、文件 / 路径 / 源 FD、Shell 虚拟文件格式或 OLE 私有 HWND / window-name 查找。另一锁定线程只以自己的窗口尝试 OpenClipboard / CloseClipboard，记录是否能在外层锁应持有期间抢锁，不读写正文。

锁内再次核对 expected generation 后，仅调用一次 OleSetClipboard，记录实际 HRESULT、前后 generation、公开 GetOpenClipboardWindow 是否仍为自己的 HWND、竞争线程 attempts / acquisitions 和外层 Close 结果。失败 HRESULT 是有效的负面平台证据；不卸锁后重试、不 flush / clear / restore、不做第二次发布或发布后回滚，只结束自己仍持有的锁并正常释放测试资源。

一次 `R2_SHELL_GENERATION_PROBE` JSON 记录写入既有 test 输出和原 CI summary / 上传日志。只有 one-shot flag 为 1 时 workflow 才强制恰好一条记录 / 一次 mutation 并解析；取得准确结果后移除该 flag 即同时关闭测试调用与强制记录门，不影响之后的原 native 测试。未知 setup 或缺记录是 NOT_RUN / inconclusive，不能写成 API 不支持。

HRESULT 失败且代次未变 / 自有锁仍在，只能说明该 runner 的外层锁组合不能完成发布；锁丢失或 competitor acquisitions 非零同样拒绝该组合。成功和零次竞争观察只说明该 runner 的采样结果，不能证明无瞬时解锁、普遍 Windows 合同或 Explorer 行为。所有情形仍须单独解决 [Shell primitives](RELIABILITY-R2-SHELL-PRIMITIVES.md) 所列 generation 原子门，生产发布器和 Native 自动删除继续关闭。

## 本机实际验证

desktop 工作目录，加载 `.tools/use-desktop-toolchain.ps1` 后仅一次：

`GOWORK=off CGO_ENABLED=0 go test -c -o D:/apps/Osend/.artifacts/r2-shell-generation-probe/desktop-probe.test.exe .`

exit 0，日志 `.artifacts/r2-shell-generation-probe/compile-only.log`。该命令只编译，不执行 test binary / init / clipboard API；没有用 `-run '^$'` 冒充 compile-only。产物 SHA256 `353945a377f121edadd6177b65aebf0c2a34bb5b65dd3ad6e045c0034aa6d324`。`gofmt`、定向 diff check 和修改后的 workflow PowerShell AST 静态解析另录冻结收据；真实 CI / HRESULT 尚未取得。

## 925首轮与静态检查修正

准确925的desktop显式`go vet ./...`在将GlobalLock的native地址转为Go slice处拒绝unsafe.Pointer用法；Go测试通过不等于该静态检查通过。Windows包另在原缓存生命周期用例失败，native步骤实际skipped、JSON0/mutation0/native artifact0，收据 `.artifacts/r2-helper-ci-925ba7c/CI-PROBE-NOT-RUN-RECEIPT.json`。这不是API unsupported证据，也不以撤flag冒充已观测。

仅将复制改为项目已有的`RtlMoveMemory`调用，从仍存活的Go UTF-16 buffer复制到native地址，不再反向构造Go指针；`runtime.KeepAlive`覆盖该buffer。一次`GOWORK=off CGO_ENABLED=0 go test -c -o <artifact>/desktop-probe-vetfix.test.exe .`及同模块`go vet ./...`均exit0，记录 `.artifacts/r2-shell-generation-probe/vetfix-check.json`、`vetfix-compile.log`、`vetfix-vet.log`。没有执行binary/init/clipboard，没有静态检查豁免。one-shot flag保留到首次有效hosted结果后再撤。
