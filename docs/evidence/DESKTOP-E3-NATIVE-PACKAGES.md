# E3 双平台原生包生命周期

## 2026-09-22 满容量交接重试

两个原生适配器原先都在读取既有 request ID 前检查 64 条 journal 上限，满容量时连完全相同的重试也得到 storage full。Windows 新增真实临时目录自测，修复前 `dotnet run -c Release --project apps/desktop/native-share/windows -- --self-test` 在第 64 条后的原请求重放以 `SHARE_JOURNAL_FULL` 失败；修复后 `PASS share_target_journal` 与 `PASS share_target_full_journal_retry`，exit 0。原始日志分别为执行树 `.artifacts/astra-resume/share-before.log` 与 `share-after.log`。

Windows/C# 和 macOS/Swift 在同一 journal 锁内先核对已有 ID/内容，再对新请求执行原容量限制：一致重试不占新槽，冲突仍拒绝，第 65 条新请求仍拒绝，已存请求字节不变。没有改变交接 schema、复制文件策略、Go 队列所有权或授权边界。两平台自测均已增加满容量幂等/冲突/容量/已有内容保留断言，并接入 packages CI 的对应原生 runner。本机 Windows 自测及 `096d79a` packages run `35724937012` 的 Windows/ARM Mac/Intel Mac 自测均通过，两个 Mac job 都输出五项 `PASS share_store_*`，完整日志在 `.artifacts/astra-resume/checks/ci-macos-arm64.log` 与 `ci-macos-amd64.log`。Mac 物理机本轮不可用，正式签名、系统共享注册/激活与安装生命周期仍保留原缺项。

日期：2026-09-14。范围 E3-03，状态 **PARTIAL / 签名安装由用户暂缓**。

Windows Wails 构建任务会发布 self-contained `LinkSend.ShareTarget`，正式 manifest 同时声明产品宿主与 Share Target；macOS bundle 任务会编译并嵌入 `LinkSendShare.appex`，宿主和扩展保留同一正式 App Group entitlement，宿主 Info.plist 注册 `linksend-share` 唤起 URL。两平台适配器与 Go 使用相同的 schema 2 request ID，持久交接仍由 Go 的唯一 profile owner 消费。

Windows 本机已完成 .NET Release 构建/自测和 desktop test/vet/build；`GOWORK=off go test -race . -run 'Activation|NativeShare|NativeArguments' -count=5` 通过。真实 Mac 已完成 Share Extension 源码构建、provider/store 测试和 desktop Go/Objective-C test/vet/build。两平台安装任务的签名、identity package、系统注册、冷启动/后台/主窗打开、连续共享、升级和卸载均为 NOT RUN；源码结果不继承 E0 原型的系统入口结果。

用户没有 Apple 或 Windows 签名证书并明确暂缓签名。当前保留真实 entitlement/manifest 和可审查构建任务，不生成不受支持的假 App Group，不修改证书信任，也不把 ad-hoc/未安装包记为通过。无签名源码候选验收后可继续 E4；未来恢复准确包时必须固定提交与包 SHA256，再执行完整安装生命周期和文件 hash 验收。
