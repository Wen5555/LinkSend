# E3 双平台原生包生命周期

日期：2026-09-14。范围 E3-03，状态 **PARTIAL / 签名安装由用户暂缓**。

Windows Wails 构建任务会发布 self-contained `LinkSend.ShareTarget`，正式 manifest 同时声明产品宿主与 Share Target；macOS bundle 任务会编译并嵌入 `LinkSendShare.appex`，宿主和扩展保留同一正式 App Group entitlement，宿主 Info.plist 注册 `linksend-share` 唤起 URL。两平台适配器与 Go 使用相同的 schema 2 request ID，持久交接仍由 Go 的唯一 profile owner 消费。

Windows 本机已完成 .NET Release 构建/自测和 desktop test/vet/build；`GOWORK=off go test -race . -run 'Activation|NativeShare|NativeArguments' -count=5` 通过。真实 Mac 已完成 Share Extension 源码构建、provider/store 测试和 desktop Go/Objective-C test/vet/build。两平台安装任务的签名、identity package、系统注册、冷启动/后台/主窗打开、连续共享、升级和卸载均为 NOT RUN；源码结果不继承 E0 原型的系统入口结果。

用户没有 Apple 或 Windows 签名证书并明确暂缓签名。当前保留真实 entitlement/manifest 和可审查构建任务，不生成不受支持的假 App Group，不修改证书信任，也不把 ad-hoc/未安装包记为通过。无签名源码候选验收后可继续 E4；未来恢复准确包时必须固定提交与包 SHA256，再执行完整安装生命周期和文件 hash 验收。
