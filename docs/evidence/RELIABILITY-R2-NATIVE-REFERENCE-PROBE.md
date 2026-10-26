# R2：Windows 原生引用与 helper 寿命隔离原型（2026-10-26）

本轮三阶段原型分别通过同进程直接 COM 消费、真实跨进程/跨 apartment COM 消费，以及 launcher 正常退出后的独立 helper 服务与退出门。**它们没有接入生产发布或回收器，不能据此标记 R2、Explorer 键盘粘贴或已发布缓存自动回收完成。**

整理时生产源码冻结在 `701e23b`。原型源码、二进制、夹具和原始日志只保存在本机 `D:\apps\Osend\.artifacts\r2-native-reader-draft`，不属于该提交的生产实现，也没有随本证据文档提供发行包。以下路径未注明时均相对于该本机证据目录；普通仓库克隆不会自动取得这些忽略产物。

## 三阶段实际结果

| 阶段 | 真实执行范围 | 最终 trace / 保护快照 / quiescent witness | 自有源文件 | 结果 |
| --- | --- | --- | --- | --- |
| 同进程 | Windows SDK 的 `IDataObject`、`IDataObjectAsyncCapability`、`IStream` 直接消费者 | 166 / 8 / 4，四个 publication 各一次 witness | 4 个，65,585B | 严格 build、运行 exit 0 |
| 跨进程 | STA producer PID `70072`；独立 MTA consumer PID `27080`、`62548` | 496 / 5 / 1 | 4 个，65,580B | build、运行、收据复核 exit 0；两 consumer exit 0 |
| helper | launcher PID `26936` 正常退出；仍存活的 STA helper PID `66508` 服务 MTA consumer PID `16784`、`60100` | 499 / 6 / 1 | 4 个，65,581B | build、runner exit 0；launcher/helper/两 consumer 均 exit 0 |

三份夹具的普通文件内容不同，字节数按各自收据独立计算。三阶段均未删除源文件；中文目录、普通文件、零字节文件和空目录保留。文档整理时再次按收据 SHA256 回读全部普通源文件，三组均匹配；再次按精确探针可执行文件名核对进程，无残留。进程收尾与源夹具删除是不同事项，本轮没有执行后者。

三个阶段的 `GetClipboardSequenceNumber` 都为 `958 → 958`。这只是当次只读观察值；“没有发布/修改系统剪贴板”的依据还包括探针调用边界、源码检查与导入审计，不能仅用序号未变代替证明。

## 同进程直接 COM 消费

最终原始证据为 `native-reader-final-trace.jsonl`、`native-reader-final-receipt.json`，夹具为 `run-final-20261026-034042/cache`。

- 六个 Unicode 相对描述符包含目录和空目录；另建仅含空目录的独立 publication。provider 仍存活时，即使普通文件句柄数为零也不产生 quiescent。
- 真正调用 `Read`、`Seek`、`Clone`、`CopyTo`，逐字节校验；原流与克隆的游标独立，拒绝最小 `LONGLONG` 等非法 seek。源对象关闭后，已复制到独立目标 memory stream 的内容仍可读。
- 保留 `IDataObject` 而尚未 `GetData`、可选同步消费者、`StartOperation` 未 `EndOperation`、`EndOperation` 先于 stream 释放、EOF、仍持有 stream/clone，均不会提前授权。最后 native 对象、活动调用、异步引用及源句柄结束后，每个 publication 才产生一次 witness。
- trace 中 `injected_clipboard_changed_*` 是注入到自有 owner 的观察信号，从未改变系统剪贴板，不能记为真实 ownership-change 测试。
- `FILEDESCRIPTORW.cFileName` 超出 `MAX_PATH` 的相对名称/目录树完整拒绝，不截断。这里只验证拒绝行为，不证明 Explorer 物化兼容，更不能据此缩小产品已有路径支持。

本阶段收据中的跨进程/并发消费者 `NOT_RUN` 是该阶段当时的边界；后两阶段有各自独立证据，不能回填为本阶段已测。

## 真实跨进程与 apartment marshalling

原始证据为 `marshal-run-20261026-042832-trace.jsonl`、`marshal-run-20261026-042832-result.json`、`marshal-run-20261026-042832-receipt.json`，夹具为 `marshal-run-20261026-042832/cache`；复核脚本为 `verify-marshalling-receipt.ps1`。

producer 使用 `OleInitialize` STA 和真正的 Windows 消息泵。调用系统 `CoMarshalInterface(..., IID_IDataObject, MSHCTX_LOCAL, ..., MSHLFLAGS_TABLESTRONG)`，把 68B 标准 OBJREF 经两个私有 inherited pipe 分别交给两个 MTA 子进程；consumer 通过 `CoUnmarshalInterface` 取得系统 proxy。子进程参数只有自有 consumer 角色，不接收 cache 路径；文件正文由系统本机 COM `IStream` 传递，私有控制 pipe 只传 OBJREF 与有限控制/事件。

两个进程都持有独立 stream 和 clone 后，producer 在原 STA 调 `CoReleaseMarshalData`，再释放本机 `IDataObject` 引用。两 consumer 随后继续 `Read`、独立 `Seek/Clone` 和双向 marshaled `CopyTo`，逐字节校验通过。A 使用可选 async 接口，B 不使用；两者同时保持 consumer 生命周期，STA 按 COM apartment 规则派发调用，不宣称 provider 方法在 STA 内并行执行。

| 观察点 | provider 对象 / stream 实例 / async operation / mode hold / 源句柄 | 结果 |
| --- | --- | --- |
| producer 本机引用与 marshal table 释放后 | 1 / 4 / 1 / 1 / 4 | 保持保护；系统远程引用仍可持有 provider |
| 两 consumer 读取到 EOF | 1 / 4 / 1 / 1 / 4 | EOF 不构成完成许可 |
| A `EndOperation` 并释放 async 接口后 | 0 / 4 / 0 / 0 / 4 | provider 对象已终结，stream 仍保护 |
| 原始 stream 全部释放，两个 clone 保留 | 0 / 2 / 0 / 0 / 4 | 仍保护 |
| A clone 释放，B clone 保留 | 0 / 1 / 0 / 0 / 4 | B clone 可再次从头读取完整内容 |
| B clone 也释放后 | 0 / 0 / 0 / 0 / 0 | 活动调用归零、全部源句柄关闭，恰好一个 witness |

源引用结束后，两个 consumer 各自的独立目标 memory stream 仍逐字节可读；两个 MTA apartment 正常退出。这里的“provider 对象已终结”是 `IDataObject` 对象最终 `Release`，不是 producer 进程退出。producer 进程在整个远程调用阶段保持运行。

## launcher 退出后的 helper 与退出门

原始证据位于 `helper-run-20261026-044201/`：`launcher-stdout.jsonl`、`launcher-stderr.log`、`helper-trace.jsonl`、`helper-stderr.log`、`helper-receipt.json`；夹具在其 `cache/`。runner 为 `run-helper-probe.ps1`。

launcher 用明确的五句柄继承 allowlist 启动 helper；交给 helper 的 launcher 进程句柄只有 `SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION`。runner 在允许 launcher 退出前，按其真实启动回执取得 helper PID，打开同样有限权限的稳定进程句柄并核对可执行文件完整路径。测试没有靠进程名称或猜测 PID 来终止其他进程，没有改变令牌、权限、注册表、桌面、网络、启动项或服务。

helper 复用上一阶段标准 COM 实现，在自己的 STA 中创建两个真正的 MTA consumer。双方已持有 stream/clone，且本机 `IDataObject` 与 marshal table 引用已释放后，helper 才通知 launcher；launcher 正常 exit 0。helper 继续泵消息，通过继承的稳定进程句柄确认 launcher 已结束且 exit 0，然后才发出继续读取命令。

六个保护快照覆盖 launcher 退出前后持有四条 stream/clone、读到 EOF、`EndOperation`、原始 stream 释放，以及最后 B clone 仍存活。最后一个 clone 保留时，`providers=0`、`streams=1`、`source_handles=4`、`quiescent=false`，B 仍可重新读取完整内容。最后引用释放后才关闭全部源句柄并产生唯一 witness；独立目标 stream 仍可读，两个 consumer 均 exit 0。

helper 只在上述源引用和 consumer apartment 均结束后打开退出门：本次 `gate_after_quiescence_ms=15`，断言上界为 10,000ms。runner 在 launcher 退出后通过已持有的稳定 helper 句柄等待 28ms，取得 helper 的真实 exit 0；没有把“输出退出日志”当作进程已经退出。15ms/28ms 是这次夹具观察，不能推广为生产退出时延保证。

**helper 本身一直是存活的 COM producer。这里只证明 launcher 退出后它仍可服务已有消费者；没有测试或证明 producer/helper 自身退出后 source stream 仍可用。** 目标 memory stream 的独立内容可用性也不能替代 source stream 的生存能力。

受限范围是继承句柄、只读进程观察与私有控制协议；此原型没有实现 OS 安全沙箱、签名 delegation、安全 inventory 交接或持久签名 receipt。原始 JSON trace/收据仅是本地测试证据，不是生产 owner 可接受的回收授权。

## 构建、来源与保留的失败

工具链为 `C:\Users\Wen\scoop\apps\mingw\current\bin\g++.exe`，实际版本 `g++.exe (x86_64-posix-seh-rev1, Built by MinGW-Builds project) 16.2.0`；系统收据为 `Microsoft Windows 10.0.26300`。三个最终构建都启用 `-std=c++17 -Wall -Wextra -Wpedantic -Werror -municode`，不是把带警告的早期构建记为最终通过。

同进程最终构建还使用 `-static-libgcc -static-libstdc++`，build/probe exit 均为 0；`native-reader-final-build.log` 实际为 0B，收据保留了严格参数与 compiler/source/binary 身份，没有完整 shell 命令字段。后两阶段收据保留的完整编译调用如下，路径相对于证据目录：

```powershell
g++ -std=c++17 -O1 -g -Wall -Wextra -Wpedantic -Werror -municode -static native_reader_marshalling_probe.cpp -o native_reader_marshalling_probe.exe -lole32 -luuid -luser32
g++ -std=c++17 -O1 -g -Wall -Wextra -Wpedantic -Werror -mwindows -municode -static native_reader_helper_probe.cpp -o native_reader_helper_probe.exe -lole32 -luuid -luser32 -lshell32
```

后两阶段最终编译没有 stdout/stderr 输出，执行工具均返回 exit 0；PowerShell `Tee-Object` 在没有管道内容时没有创建 `native-reader-marshalling-build-02.log` / `native-reader-helper-build-02.log`，不能将不存在的文件描述成已捕获的空日志。跨进程 run 的 stderr 为实际 0B 文件；helper 的 launcher/helper stderr 都实际为空。

| 实际保留产物 | SHA256 |
| --- | --- |
| `native_reader_probe.cpp` | `a5ff44a558c2caf97818139441c0529b78f427242079258f12a3932c1c975482` |
| `native_reader_probe.exe` | `5ba4d0aac165eed49b0ed2b12b8af10d9e66716a84a01c4110bfc1ea3a6d94d3` |
| `native_reader_marshalling_probe.cpp` | `19da4ab93ae9737f4733ede2868f2cd52ab006723647e7b51fa3b59ac524df6f` |
| `native_reader_marshalling_probe.exe` | `277e0b6364fd4024b052d58c384446d2e1e8f42a957ad64a016d7e90d1bc6761` |
| `native_reader_marshalling_shared.hpp` | `2e888e878466e606ae602641a63a0e16a830377ab5b29d9030e93329849b2438` |
| `native_reader_helper_probe.cpp` | `3f398d7e99fafd3c995d73b9564635f318e55dc9153224015caeb5322a86af1e` |
| `native_reader_helper_probe.exe` | `c46573e9f901657c3dcdbe5c6d6e8caf046d51564814dc8bb9b752a05bd3293e` |
| `run-helper-probe.ps1` | `a26acbb18979c76b3074b7c7ee6ce5c251331560fc4e47885d10810d1f749974` |

文档整理时重新计算三对源码/EXE 哈希，与上表一致。helper 的共享 header 是原跨进程 `.cpp` 的快照，只有外层 `wmain` 改名和说明注释；已逐字验证这一个变换，原跨进程源码/EXE 未被覆盖。直接 consumer 源码也是后两阶段包含的实现。

早期开发记录全部保留，均不计入上面的最终 PASS：

- `native-reader-build.log` 是带 `UNICODE` 重定义及缩进等警告的首轮构建，`native_reader_probe.first-pass.{cpp,exe}` 和 `native-reader-first-{receipt.json,trace.jsonl}` 单独保留。
- `native-reader-strict-build-first-failure.log` 是 `-Werror` 下 `UNICODE` 重定义的真实编译失败。加宏保护并修复代码后才完成最终严格构建。
- `before-directory-only-*` 是尚无独立 empty-directory-only publication 的旧三 publication 版本，未覆盖成四 publication 结果。
- `native-reader-marshalling-build-01.log` 是异常清理中多条单行 `if` 触发 `-Werror=misleading-indentation` 的真实失败；改成各自独立语句后严格构建通过，没有关闭告警。
- helper 首轮编译虽 exit 0，runner 报 `Launcher exited without a helper PID`。反汇编证实 MinGW Unicode CRT 先调用 include 带入的旧 `wmain`，尚未进入新 `wWinMain`，当时连 launcher fixture 都没有创建。`helper-entry-01-failure.json`、`helper-entry-01-crt-disassembly.txt`、`native_reader_helper_probe-entry-01.{cpp,exe}` 保留该失败。只给共享快照的外层入口改名后，才进行上述 499 条 trace 的成功运行。

`native-reader-design.json` 和 `native-helper-exit-proposal.json` 是先前阶段的设计快照，含有当时的 `NOT_RUN` / 未启动表述；应结合本证据按阶段读取，不能把设计中的签名交接、生产接入或平台兼容当作已经实现。

## 未触碰的边界与下一步

三阶段从未调用系统剪贴板/native publication，也没有调用生产 `ClipboardAdapter.WriteFiles` 或 cache collector。只读取系统剪贴板序号，不读取、导出、备份或覆盖用户剪贴板正文。三份 EXE 的导入审计均无 `OleSetClipboard`、`OleFlushClipboard`、`SetClipboardData`、`EmptyClipboard`；`RegisterClipboardFormatW` 仅注册格式。跨进程/helper 静态 C++ filesystem runtime 带有 `DeleteFileW` / `RemoveDirectoryW` 导入，源码没有删除调用，不能把导入存在/不存在单独当成执行证明。

生产在冻结提交仍使用 Windows `CF_HDROP` 绝对路径和 macOS `NSPasteboard` file URL；`ClipboardAdapter.WriteFiles` 仍只返回 generation/error。原型的相对 descriptor/stream 合同没有替换这两条发布路径，既有 `published/publishing/protected` 对象没有新增删除许可。EOF、`EndOperation`、clipboardChanged、TTL、boot 变化、进程消失、零打开句柄或原型中的 witness 都不能单独解除保护。

后续必须继续实施并验证：

1. 对新 publication 定义无物理路径出口的完整合同，绑定 cache/publication/run、manifest 与实际 inventory。审查格式枚举、`Stat`、Shell 默认 fallback、Open/Reveal/drag/history 等所有可能暴露物理 cache 路径的出口；一旦路径外泄，该对象整体继续按 path-protected 处理，不能追认旧发布对象。
2. 在同一 Go/SQLite owner 下完成可信 adapter、用途签名 delegation、固定来源/有界 receipt 交接与签名/CAS 持久化。helper 仅凭内存计数/JSON trace 不得获得回收权；缺失/半写/错误签名/旧回调/崩溃均继续 protected。安全 inventory 交接、helper 版本升级占用与正常退出策略尚未实现。
3. 在可安全操作系统剪贴板的真实 Windows 环境验证 Explorer Ctrl+V：中文、多文件、目录/空目录、长相对路径、重复与并发粘贴、取消、新复制、持有旧 `IDataObject`/stream/clone、缺失或提前 `EndOperation`、主程序退出/重启/升级。不能以本地 COM fixture 抵扣系统消费者、系统 clipboard owner 变化或原生物化结果。
4. 独立完成 macOS Finder Cmd+V 和原生读取/退出寿命实测。Windows 原型、官方 drag/drop 示例、命名 pasteboard 和交叉编译都不能代替这些证据。
5. 取得可验证且持久的读取结束许可后，再结合原来的活动引用、完整封存清单、固定文件身份、自动保留期及逐项删除门，验证真实自动回收和崩溃对账。旧路径发布和任何不确定交接保持保护，不增加手动清理、容量或期限设置。

本文件只记录局部技术证据；唯一 TODO 的完成状态由完整验收维护，不在此改写。

## 已核对的系统 API 资料

- [CoMarshalInterface](https://learn.microsoft.com/en-us/windows/win32/api/combaseapi/nf-combaseapi-comarshalinterface)、[CoUnmarshalInterface](https://learn.microsoft.com/en-us/windows/win32/api/combaseapi/nf-combaseapi-counmarshalinterface)：只传自有可信标准 packet；不实现自制 marshaler。
- [CoReleaseMarshalData](https://learn.microsoft.com/en-us/windows/win32/api/combaseapi/nf-combaseapi-coreleasemarshaldata)：在创建 marshal packet 的同一 apartment 释放 table strong 引用。
- [IDataObjectAsyncCapability](https://learn.microsoft.com/en-us/windows/win32/api/shldisp/nn-shldisp-idataobjectasynccapability)、[IStream::Clone](https://learn.microsoft.com/en-us/windows/win32/api/objidl/nf-objidl-istream-clone)、[FILEDESCRIPTORW](https://learn.microsoft.com/en-us/windows/win32/api/shlobj_core/ns-shlobj_core-filedescriptorw)：实际 SDK 接口与字段，不推导系统粘贴或全消费者结束保证。

没有调用 `CoInitializeSecurity` / `CoSetProxyBlanket` 降低 COM 保护，没有改注册表/权限来取得通过结果。
