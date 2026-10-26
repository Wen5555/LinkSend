# R2 Windows Shell primitives candidate, 2026-10-26

本增量增加独立、尚未接线的 C++ Shell provider primitives。生产 Go helper 仍只调用原 receipt-only broker；没有执行新 Shell 函数、注册系统格式、操作系统剪贴板或 GUI，也没有启用 Native 缓存删除。R2 不据此标记完成。

## 源码范围

仅 `apps/desktop/cmd/clipboardfilehelper/bridge.h` 和 `bridge_windows.cc`。`ShellSession` / `ShellWitness` 与原 `BrokerSession` / `BrokerWitness` 分开，旧 controlled v2 codec 不接受或调用新合同。构造阶段只借用调用方 FD，并在 native 内复制最小只读权句柄；成功和失败后原 FD 仍由调用方关闭，native 只关闭自己的副本。

候选提供 `FileGroupDescriptorW`、`FileContents` / `IStream` 和 copy effect，相对 descriptor / Stat 不提供物理源路径；没有 CF_HDROP、TYMED_FILE 或按物理路径重新打开的 fallback。shared Owner 的 failed 状态继续实际核减已有引用、结束活动调用和关闭源，但不再签发 witness。源 close 失败或未知 native 状态均保持保护。

首次 mutation 前的准入期限使用有溢出检查的 uint64 毫秒；可能 export 后没有 deadline、abort 或 launcher 退出强杀路径。helper 保留自己的 IDataObject 控制引用，只有 `OleIsCurrentClipboard` 明确为 S_FALSE 才释放，已有 provider、marshalling、stream / clone、async operation 和活动 I/O 继续服务。全部实际引用结束且源成功关闭后才可获得单次 opaque witness；`shell_destroy` 对仍活动的 session 不强制释放。

构造预算为最多 20,003 个借用句柄、10,000 个发布条目、总计 4,194,304 UTF-16 名字单元；正文没有旧 controlled 64 MiB 总量上限，文件大小 / seek 使用 64 位。FILEDESCRIPTORW 的发布相对名字仍必须少于 260 UTF-16 单元，超界明确拒绝，不能截断或转为 CF_HDROP。Go 分页 handoff、原 1,024-node 清单预算、大目录和长路径仍须继续实现；这些构造预算不等于已证明完整生产范围。

## 实际最小验证

固定证据为 `.artifacts/r2-shell-primitives/freeze.json`，构建前 HEAD 记录为 `f9a2c5951f23db9445de3211d093c740aee38f48`，这两个源码文件当时为未提交增量。该记录不把 dirty EXE 当作发行包。

- desktop 工作目录，一次 `GOWORK=off CGO_ENABLED=1 CC=<verified MinGW>/gcc.exe CXX=<verified MinGW>/g++.exe go build -o D:/apps/Osend/.artifacts/r2-shell-primitives/clipboardfilehelper-wip.exe ./cmd/clipboardfilehelper`，exit 0。沿既有 cgo 严格 `-std=c++17 -Wall -Wextra -Wpedantic -Werror`。
- WIP helper 10,551,378 bytes，SHA256 `1975ab99d576e64b6a36fb7464f69bee92c7090cb94cd42aa80300316a410b63`。源码 SHA256 为 `bridge_windows.cc=3790289a0d6da3797f12f4f593abc3daab0788beded3d1898accd5c4fc51ad93`、`bridge.h=8fcc2eac76b12ba2703507ee56f4cc55af3723668fd82e7be1c2c7edbe16df8d`。
- 根工作目录，固定 `LINKSEND_NATIVE_HELPER_TEST_EXE` 为上述 EXE、`LINKSEND_NATIVE_HELPER_TEST_SHA256` 为上述摘要后，仅一次 `GOWORK=off go test ./internal/app -run '^TestClipboardCacheWindowsNativeControlled$' -count=1 -timeout 60s -v`，exit 0，package 6.380s / case 1.76s。日志 `controlled-regression.log`，SHA256 `5f0c4ce4e5c46d5c118390f595a47bbc1bf8a8470afb3e9830cc92993af6127e`；`CONTROLLED-REGRESSION-RECEIPT.json` 记录原七门、五源关闭、同 run spool 单次消费和源保留。

该回归只验证 shared Owner 改动没有破坏原私有受控 COM / source-close 路线；没有运行新的 Shell API，不是系统发布、Explorer 或自动回收证据。回归后冻结的两个源码和 EXE 摘要仍匹配。不重复 full matrix / race 或扩展 COM 场景。

## 发布代次的阻断

候选当前在 `GetClipboardSequenceNumber()==expected_generation` 后调用 `OleSetClipboard`，返回后还验证稳定代次和准确的 IDataObject ownership。这能约束成功 ack 的出处，但两次独立调用之间仍可能有其他程序完成新复制，因此不构成原子 compare-exchange，不能声称会保留这个竞争窗口中的更新复制。不得在发布后回滚旧剪贴板来弥补竞争。

官方 [OpenClipboard](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-openclipboard) 明确另一 window 已打开时失败；[OleSetClipboard](https://learn.microsoft.com/en-us/windows/win32/api/ole2/nf-ole2-olesetclipboard) 自行调用 OpenClipboard / EmptyClipboard / SetClipboardData / CloseClipboard，并使用 OLE 内部 window 持有 clipboard。两份 desktop 合同没有承诺先以自有非空 HWND 持外层锁，再从同一 STA 调 OleSetClipboard 可重入且保持该锁。Windows CE 的 same-thread 说明不外推为 desktop 保证，兼容实现源码也不作为 Windows 平台合同。

新 publisher 在此问题解决前保持未激活。若需最小实测，仅复用现有 GitHub-hosted Windows、已明确建立空剪贴板的隔离入口；不在本机或 self-hosted runner 操作 clipboard，不引入新矩阵。probe 仅判断 own-HWND 外层 OpenClipboard、锁内代次检查与 OleSetClipboard 的组合是否成立，记录实际 HRESULT、外层锁是否仍持有及竞争窗口行为；任何失败均保守拒绝发布，不自动移除锁重试、不回滚、不降级路径。即使一次 probe 成功，也须分别界定其支持范围和官方合同缺口，不据此宣称全部 Windows 版本或真实 Shell 已通过。

## 后续准入

正式接线仍须独立 `windows-virtual-files-v1`、生产分页元数据 / source inventory、实际 helper 信任锚、持久意图及最新 grant / cancellation / generation 门。之后实际 Explorer 键盘粘贴、目录 / 空目录 / 大文件 / 长路径、重复和并发消费者、main 退出 / 重启 / 升级与最终自动回收分别取得证据。只有可信原 publication 回执、完整封存清单、Go 活动引用、零物理路径出口和自动保护期全部满足，才可另行连接逐项删除；旧 receipt-only、CF_HDROP / file URL 和未知对象永不追认。
