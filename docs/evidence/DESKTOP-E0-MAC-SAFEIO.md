# E0 Mac SafeIO 修补与收尾

2026-09-13；总控分派E0-supervision-ui-v1，收敛指令E0-safeio-close-v1。范围为E0-04原型资源安全和真实菜单复测，不扩展新共享机制，不代表E0/U6完成。
工作分支codex/desktop-experience-upgrade，基于11c44df；本轮包是在提交前的明确源码快照构建，不冒充已提交发行包。

## 修补

- Host的payload和captured.json均用共享FixtureIO.readBounded读取，分别最多1MiB/16KiB；每次读取有界，超过限额的一字节探测即拒绝，不再Data(contentsOf:)之后才检查。
- O_NOFOLLOW拒绝末级symlink，O_NONBLOCK配合fstat拒绝FIFO/非普通文件，避免在大小检查前阻塞。
- 扩展独占创建0700请求目录，openat/O_EXCL创建输出；File.Sync后记录captured。接管失败只unlink已登记且dev/ino仍匹配的本请求文件，不扫描或递归删除容器。替换文件、其他请求和外国文件保留；空请求目录和失败诊断保留，不假称全目录清空。
- 测试源使用完整NSApplication生命周期，显式保持delegate生命周期；菜单嵌套循环用common-mode Timer退出。菜单动作与直接服务激活分进程，避免混淆来源。最终服务测试用非零退出表达失败/未完成，不能再只看wrapper退出0。

## 实际验证

真实Mac alias mac-test-102342413，wen@10.234.39.151，macOS26.5/ARM64，CLT Swift6.3.2；显式编译最低macOS13，不声称最低系统实机通过。
本轮resolve/probe/audit成功，未更改网络、安全策略、TCC或用户LinkSend profile/剪贴板。

`xcrun swiftc -swift-version 5 FixtureIO.swift FixtureIOTests.swift -o fixture-io-tests`、`./fixture-io-tests`、`sh build.sh`在修正版作业均退出0。
真实10项PASS：exact_payload_limit、grown_payload_rejected、oversized_receipt_rejected、symlink_rejected、fifo_rejected_without_blocking、duplicate_request_rejected、duplicate_file_rejected、cleanup_preserves_foreign_and_other_request、cleanup_preserves_replacement_identity、path_name_rejected。
这些验证真实文件IO和清理归属；不是内存基准、真实ShareOperation交接或跨设备发送结果。

最终host/.appex的swiftc、codesign --verify --deep --strict退出0；nm确认_OBJC_CLASS_$_LinkSendE0ShareViewController符号存在。
真实SDK NSSharingService.h确认standardShareMenuItem是NSSharingServicePicker的实例属性（macOS13+）。测试实际创建系统提供的Share…菜单项、展示/关闭菜单并用NSMenu.performActionForItem派发其系统action；没有自造Share菜单项或假设备。
服务枚举出现LinkSend E0，pluginkit精确ID登记为1 plugin。**Finder人工点击仍NOT RUN**；系统菜单动作测试不能冒充Finder操作或文件交接。

菜单与服务分进程后，服务仍通过NSSharingServiceDelegate回调失败。最终精确错误：

`NSCocoaErrorDomain/4097: Couldn’t communicate with a helper application.`

最终service-only进程和作业退出1，没有captured.json成功回执。此为E0-04待修技术缺陷，根因未定位，不归因为外部证书阻塞，不标PASS。原有0签名身份/AX=false仍为独立限制；没有降低U6要求。

## 作业、失败与原始日志

所有job均在 `/tmp/codex-ssh/`，原始stdout/stderr/exit-code经manager download保存至ignored `.artifacts/desktop-experience/e0/`。

| job后缀 | 退出码/真实结果 | 本机日志前缀 |
|---|---|---|
| linksend-e0-mac-safeio-20260913T032406Z | 1；测试precondition的非throwing autoclosure编译失败，尚未跑用例 | mac-safeio- |
| linksend-e0-mac-safeio-fixed-20260913T032528Z | 0；修正测试写法后10项PASS、host/appex构建和签名PASS | mac-safeio-fixed- |
| linksend-e0-share-sdk-20260913T032617Z | 1；误猜独立NSSharingServicePicker.h，实际头文件不存在 | 会话原始回执 |
| linksend-e0-share-sdk-header-20260913T032647Z | 0；从NSSharingService.h读取真实API | 会话原始回执 |
| linksend-e0-mac-native-menu-20260913T032838Z | 143；测试源未按期退出，精确路径PID73013终止；不计菜单PASS | mac-native-menu- |
| linksend-e0-stop-menu-tracking-20260913T033024Z | 0；仅终止上述owned测试源 | mac-menu-stop.json |
| linksend-e0-mac-menu-lifetime-20260913T033115Z | 0；生命周期/common-mode调度修正，真实菜单展示/关闭 | mac-menu-lifetime- |
| linksend-e0-mac-menu-final-20260913T033340Z | 0；构建/符号/标准菜单动作；服务回调FAIL，wrapper0不抵消 | mac-menu-final- |
| linksend-e0-mac-menu-service-separated-20260913T033539Z | 0；两个进程分离复测仍服务FAIL，wrapper0不抵消 | mac-separated- |
| linksend-e0-mac-service-result-20260913T033855Z | 1；修正测试退出语义，准确NSCocoaErrorDomain/4097 | mac-service-result- |
| linksend-e0-safeio-unregister-20260913T034016Z | 0；注销后精确IDno matches | mac-safeio-unregister.json |
| linksend-e0-safeio-final-state-20260913T034147Z | 0；no matches、OWNED_PROCESS_COUNT=0、CAPTURED_RECEIPT_COUNT=0 | mac-safeio-final-state.json |

manager的Console.Out直接输出不能由同一PowerShell进程的`& manager.ps1 | Tee-Object`可靠捕获，本轮发现后改为`pwsh -NoProfile -File manager.ps1 ... | Tee-Object`。上表“会话原始回执”没有冒称已写成本机JSON；下载的远端日志文件确实存在。未修改manager本身。

## 包与收尾

本轮host SHA256：`807452f9430af46aa95ba05fb743b767873077c6111909d2fad00be829c2a55a`。
本轮extension SHA256：`71fe39b13effb6aa93a6648953e8b3c1a8ab887d33cf5170b84e37a964427b0d`。
`.artifacts/desktop-experience/e0/LinkSend.E0.MacSafeIO.tar.gz` SHA256：`e2a83b1ee4414b1975bc9de62ed2dd0fcaeacc3583b30805f656d277d0ba95e0`，与远端`/tmp/linksend-e0-mac-safeio-review.tar.gz`一致。仅ad-hoc开发包。
归档作业linksend-e0-mac-safeio-archive-20260913T033857Z退出0。原有E0-A归档保留，不覆盖旧证据。

owned进程与注册已收尾；保留隔离原型源码、包和日志供审查。不重复Windows必失败安装，不复跑无变化核心基线。
本机`bash -n`检查本目录shell脚本、`git diff --check`及相关Markdown链接存在性检查均退出0；没有将文档检查当产品测试。
仓库外 `C:/Users/Wen/.codex/supervision/linksend-desktop-experience/executor-progress.json` 已记录动作、作业/退出码、待审提交、阻塞和下一步。
本安全修补提交后等待总控复审；当前不push。后续GitHub工作分支/PR/CI及E0-B双向网络/Mac根因仍由总控派发，E0整体未完成。
