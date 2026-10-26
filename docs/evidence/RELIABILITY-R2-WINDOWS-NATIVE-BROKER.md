# R2 Windows private receipt-only broker, 2026-10-26

本次仅新增私有源码和受控COM闭环。系统clipboard/格式注册/WriteFiles、GUI/scene、生产profile与用户文件未触碰；Native删除门保持保护，无新schema/数据库/Wails API或用户缓存设置。源码没有默认publisher调用或发行包trust anchor；测试显式使用本地准确源码构建的helper及独立预期SHA256。

## 源码范围

- `internal/app/clipboard_cache_native_launch_windows.go`：固定镜像摘要与image pin，CreateProcess的稳定hProcess、挂起后实际镜像身份核对，最小继承句柄、签名challenge、原SQLite授权回读后commit。
- `internal/app/clipboard_cache_native_boot_windows.go`、`clipboard_cache_native_spool.go`：真实boot provider；固定profile内spool目录/预建句柄，严格限长读、原签名/原raw CAS消费。
- `internal/clipboardnativecontract/{contract.go,boot_windows.go,pipe_windows.go}`：最窄私有canonical JSON、固定用途域、实际内核BootIdentifier、本机first-instance/拒远程/当前用户ACL的已连接继承管道；overlapped取消join后释放固定buffer。
- `apps/desktop/cmd/clipboardfilehelper/{main_windows.go,bridge.h,bridge_windows.cc}`：独立Go/cgo helper，最小只读继承源、元数据与内容复核，标准COM STA↔MTA受控消费者；HRESULT/noexcept/失败保护、真实逐源CloseHandle与opaque witness，固定原publication签一次。
- `internal/app/clipboard_cache_native_launch_windows_test.go`：实际boot稳定读取与一个显式opt-in真实闭环，必要拒绝与保留字节断言。原`clipboard_cache_native.go`仅更新实现范围注释，golden/签名/collector逻辑不改。

## 实际验证

在`D:/apps/Osend`运行；用现有`.tools/use-desktop-toolchain.ps1`的Go1.27.1和既有MinGW g++16.2.0，未安装依赖或修改宿主DLL/PATH。

1. root：`GOWORK=off go test ./internal/app -run '^TestClipboardCacheNativeNilGoldenCompatibility$' -count=1 -timeout 60s`，exit0，app6.657s。此结果早于最后的私有profile-work封装，只作旧codec兼容检查，不冒充最终整个增量检查。
2. desktop：`GOWORK=off CGO_ENABLED=1 go build -o ../../.artifacts/r2-native-production-increment/clipboardfilehelper.exe ./cmd/clipboardfilehelper`，准确static源码exit0，`helper-build-static.log`。C++使用`-std=c++17 -Wall -Wextra -Wpedantic -Werror`，链接`-static -lole32 -luuid -luser32 -lstdc++ -lwinpthread`。
3. root设置`LINKSEND_NATIVE_HELPER_TEST_EXE`为该准确本地EXE、`LINKSEND_NATIVE_HELPER_TEST_SHA256`为该可信构建输出摘要，运行`GOWORK=off go test ./internal/app -run '^TestClipboardCacheWindowsNative(BootStable|Controlled)$' -count=1 -timeout 60s -v`，exit0，app8.080s，真实受控闭环1.73s。日志`controlled-static.log`。
4. `GOWORK=off go vet ./internal/app ./internal/clipboardnativecontract`及desktop的`GOWORK=off CGO_ENABLED=1 go vet ./cmd/clipboardfilehelper`均exit0，分别`core-vet.log`/`helper-vet.log`。按用户最小验证要求未重复完整矩阵或race/压力多轮。

准确闭环先拒绝wrong-image，再由实际继承管道验证child challenge；wrong-child与SQLite完整row变化都在任何COM export前拒绝，spool仍0B。合法commit后，真实provider、marshal table、consumer及stream、EOF、clone、async和source-close保护检查位图为0x7f；实际5个源句柄全部成功关闭才取得native witness并签回执。helper确实exit0。仍活动的原接收引用禁止消费；伪造回执拒绝；原Service关闭后新Service重载同profile真实身份，消费原OwnerRun的spool一次，重放拒绝且首次观察不变；老化/压力collector后源字节仍匹配。

这里的重启是同测试进程中的Service关闭/重开，独立helper实际退出。**没有证明launcher/GUI进程退出时仍有活跃consumer，也没有执行实际OS reboot。** 七门是本native owner的真实受控引用/关闭检查；不是系统Shell消费者，也不扩大为Explorer证据。

提交前root核对11份冻结代码及helper实际SHA全部匹配，另完成core workspace的codec/BootStable定向测试（exit0，app5.142s）与desktop helper在workspace/off下的`go test -count=1 ./cmd/clipboardfilehelper`（均exit0，无测试文件，只证明Go/C++编译连接）。结合上述core off真实COM闭环，两个模块/两种workspace模式均有对应最小检查；未重跑完整矩阵。回执`root-source-verification.json`及`root-*-test.log`。独立只读CABI/COM审阅未发现阻止本次receipt-only提交的问题；CopyTo提前EOF会继续读取至0并保留S_FALSE，无须为疑似HRESULT覆盖修改实现。SECURITY/PROTOCOL已同步私有本机合同边界，网络V1保持。

## 保留失败与API依据

首个命令工作目录错误导致toolchain/gofmt路径不存在，并启动了未完成的初次构建；该构建随后因15处`-Werror=misleading-indentation`真实失败。已拆开语句保持告警，不关闭-Werror。首个成功动态runtime构建的真实握手前退出保留于`controlled-first.log`；PE有`libwinpthread-1.dll`，随后使用与冻结原型一致的GNU标准runtime静态链接。static版PE无该DLL依赖，见`helper-imports.txt`；没有改宿主DLL、权限或安装运行库。动态首版未取得receipt，不记成功。

Win32调用与固定`golang.org/x/sys v0.47.0/windows`的CreateProcess/StartupInfoEx、ProcThreadAttributeList、DuplicateHandle、QueryFullProcessImageName、GetFileInformationByHandle、overlapped pipe/CancelIoEx实际声明核对，并检索了Context7 `/golang/sys`同族API。Boot使用该固定版本的SystemBootEnvironmentInformation/NtQuerySystemInformation；native结果首字段GUID BootIdentifier已与[phnt原结构](https://github.com/winsiderss/phnt/blob/master/ntexapi.h)核对，源码摘录在`boot-api-source-excerpt.txt`。不支持、长度异常、全零或查询失败均不提供boot证明；墙钟/uptime/PID均不作为boot身份或删除许可。

冻结artifact仍为`.artifacts/r2-native-reader-draft/go-native-bridge/`，本增量没有改它。所有新本地证据在`.artifacts/r2-native-production-increment/`；最终源码/EXE摘要与逐文件清单见该目录`freeze.json`。本轮没有GUI/clipboard、正式Shell注册、scene05、Mac、CI/GHA或生产部署操作。R2完整原生发布、安装升级/进程退出一致性与自动回收尚未完成。
