# E3 macOS Share Extension 源码候选

日期：2026-09-14。范围 E3-02；真实 Mac 为 `mac-test-102342413`、macOS 26.5 arm64。正式签名/App Group 注册按用户要求暂缓，原生系统激活为 **NOT RUN**。

## 4097 根因
只读诊断与一次改变诊断方法的复现证明旧 E0 executable 和 ObjC principal class 都存在，扩展实际启动、完成 RunningBoard handshake 并收到 ShareKit `Prepare UI`。随后 `containermanagerd` 明确记录：

`requesting [group.com.linksend.e0.share]: REJECTED. Requestor's signature does not allow it to access a TCC-protected group container. Group containers identifiers should be prefixed by requestor's team ID`

旧原型在容器不可用及其他失败分支只返回，没有 `cancelRequest`；10 秒后系统以 signal 9 终止扩展，调用方才得到 `NSCocoaErrorDomain/4097`。关键作业：

- `/tmp/codex-ssh/linksend-e3-mac-share-4097-inspect-20260913T152844Z`
- `/tmp/codex-ssh/linksend-e3-mac-share-4097-reproduce-20260913T153047Z`
- `/tmp/codex-ssh/linksend-e3-mac-share-4097-errors-20260913T153447Z`

旧原型全部失败分支现调用 `cancelRequest`，避免继续把资源/签名错误折叠成超时 4097；没有用无签名假容器复测。

## 正式源码

`apps/desktop/native-share/macos` 使用正式 bundle `com.linksend.desktop.share` 和 `group.com.linksend.desktop`，原生面板读取最多 256 个真实授权设备。原位文件在表示回调内生成只读 security-scoped bookmark；系统临时表示才复制到 App Group 内本请求独占的 request 目录。路径/bookmark 元数据原子提交后尝试 `linksend-share://handoff/<request>` 冷启动；2 秒内不能确认唤起也会完成 extension request，持久记录留待下次宿主启动。取消、容器/设备/附件/复制/写盘错误均准确 `cancelRequest`。

真实 Mac 作业 `/tmp/codex-ssh/linksend-e3-macos-share-source-build-r9-20260913T162317Z` 完成 plist lint、设备快照、真实 `NSItemProvider` handoff、请求幂等/冲突 4 项测试以及 arm64 executable 编译；extension SHA256 `fb9d17affcdc43f189c75c3d3dde1a04db9ff6c9a8973ffa6e379970e1f6182b`，ObjC class 符号存在。Go/Objective-C 宿主桥在 `/tmp/codex-ssh/linksend-e3-go-macos-build-r3-20260913T160246Z` 完成 desktop test/vet/build；deployment-target 历史 warning 保留。

用户没有 Apple 签名身份并明确暂缓签名，故正式 App Group entitlement 生效、系统注册、Finder/Share 菜单、bookmark 跨进程读取、URL 冷启动、扩展退出后持续发送均 NOT RUN。源码编译与测试不替代这些结果。

## E3-C1 审查修复

扩展现在依据 loadInPlaceFileRepresentation 返回的真实 inPlace 值决定 bookmark 或临时接管，不再把 fileURL 类型等同于持久授权；临时表示按 1 MiB 块复制并同步。journal 随 queue request 保留，重启重新解析 bookmark，completed/cancelled/expired 后按 request 停止 security scope 并删除 journal 和 owned 临时源。单坏请求不会阻塞其他请求。

宿主已接入 Wails ApplicationOpenedWithFile URL 事件；后台 handoff 不显示主窗。设备快照读端使用 60 秒 TTL，设备选择不自动提交，离线等待需独立勾选，并提供添加设备入口。Mac 定向作业 /tmp/codex-ssh/linksend-e3-c1-mac-swift-r2-20260913T165721Z 完成 ShareStore 四项测试和 extension Swift 编译；签名、注册和系统激活仍为 NOT RUN。

## E3-C2 审查修复

Windows 安装器恢复同一 package root，manifest、宿主和 Share Target 路径一致；卸载显式删除安装器拥有的 Share Target 文件和 Logo。在线设备按钮直接提交，离线目标仍需勾选确认；错误不会调用 ReportError 锁死面板。临时目录/Temporary 属性或无稳定路径的 StorageFile 强制复制，累计上限 16 GiB，复制后校验长度并 Flush(true)，入队前失败会清理 owned 目录。

Go 对清理失败保留 journal，主界面列出未入队请求并提供明确放弃入口。Mac 使用 NSWorkspace.openApplication(arguments: --native-share-background, activates: false) 覆盖冷启动，解析带小数秒的 RFC3339Nano；provider 调用并发上限 4，按实际复制 chunk 扣 16 GiB 总预算。定向 Mac 作业 /tmp/codex-ssh/linksend-e3-c2-mac-swift-20260913T171611Z 通过。

## E3-C3 正确性补丁

放弃未入队请求与后台消费使用同一 consumeMu 串行边界，并在释放来源前查询持久队列列；一旦 request_id 已存在于队列，用户必须在发送队列取消，不能再由系统共享错误入口删除。owned 清理或 journal 删除失败仍保留可恢复记录。

Windows 临时来源改为 1 MiB 流式接管，按实际读取字节执行累计上限，源增长也不能越过 16 GiB；写入采用 WriteThrough、FlushAsync 和 Flush(true)，失败删除未完成文件。面板先静默唤醒 owner 并等待最多 2 秒的新鲜设备快照；超时显示状态未知及刷新入口，不把旧快照当离线。Mac 同样先后台启动 owner，再读取设备并提供刷新。Mac 定向作业 /tmp/codex-ssh/linksend-e3-c3-mac-swift-20260913T172620Z PASS。
