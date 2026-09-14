# E0-03 Windows 真 Share Target 原型

2026-09-13。状态：源码/构建/打包/开发签名PASS；系统安装FAIL，激活与文件授权 NOT RUN。不是正式共享交付。
Windows11 Pro x64 10.0.26200；.NET SDK9.0.318、Windows SDK10.0.26100.0。
原型独立包名LinkSend.E0.SharePrototype，版本0.0.1.0，Publisher CN=LinkSend E0 Prototype，不覆盖LinkSend安装。

## 原型与来源

微软官方AppModelSamples固定提交 `fcf66497096845bae1b9284d90642af58730e392` 的 PackageWithExternalLocation/cs/PhotoStoreDemo StartUp.cs、manifest和README已实际读取。
核实 `Windows.ApplicationModel.AppInstance.GetActivatedEventArgs` → ShareTargetActivatedEventArgs → ShareOperation → GetStorageItemsAsync。
E0先采用独立完整MSIX测该激活路径；NSIS+external-location组合仍待E3评估，未称已实现。
原型没有假设备列表或发送。真实StorageFile权限读取后按请求ID接管小fixture，记录文件hash、包身份、系统报告。
128项/实际64MiB总限额；复制中校验增长/缩短/mtime。capture成功与ReportCompleted是分开的回执；系统报告失败不重写同名CreateNew，也不吞掉失败。
这仅是小fixture可行性工具，不把全文件复制作为E3默认大文件策略。

## 实际命令/结果

- `dotnet publish ...csproj -c Release --self-contained true -o .../windows-share-package` 初版和审查修正版均退出0，NuGet默认源实际可用。
- `makeappx.exe pack /d <stage> /p <msix> /o` 退出0，未用/nv跳过校验。
- 开发自签证书生成于CurrentUser/My，仅7天；signTool /fd SHA256 /s My /sha1 <recorded-thumbprint> 退出0。
- 修正版包 `LinkSend.E0.SharePrototype-v2.msix` SHA256：`3ce50c03a78ef651bbb5b83762dd070b90759f581bd8c8bba655e0e89c33cb15`。
- 初版和修正版 `Add-AppxPackage -Path <msix>` 均失败：0x80073CF0 / **0x800B0109** 根证书不受系统信任。CurrentUser/TrustedPeople导入不足，进程IsInRole Administrator=false。
- 开发路径 `Add-AppxPackage -Register <stage>/AppxManifest.xml` 失败：**0x80073CFF**，系统未启用旁加载/开发许可。没有修改开发模式/机器信任/安全策略。

修正版原始失败：e0/windows-share-v2-install-failure.txt，ActivityId0364ae7a-3c9b-0002-5a5c-e4109b3cdd01。
开发注册失败：e0/windows-share-development-registration-failure.txt。
所有包和签名日志在ignored `.artifacts/desktop-experience/e0/`，不要提交私有签名材料。公共测试证书为LinkSend.E0.cer，私钥只在Windows证书库，不导出。

## 准确阻塞与最后一步

需由具备管理员权限的用户审阅并信任该**开发测试证书**用于本机临时包安装，或提供系统已信任的有效签名身份；现有工具进程不能获得该权限，也不能把系统安全策略变更推定为已授权。
安装成功后继续真实Explorer系统共享→此原型、冷启动/后台、StorageItems、源变化/权限、回执、卸载复核；全部目前NOT RUN。
开发自签成功仍不证明普通用户正式安装信任。E3依旧需要明确发行签名与包身份方案。


清理回执：本轮临时证书thumbprint 909B9E32881853D8037259E9ADFE927635AE1AAE 已从CurrentUser/My和CurrentUser/TrustedPeople精确移除，Get-AppxPackage精确包名计数0。公共.cer与已签名包仍在ignored产物目录；未导出私钥。e0/windows-prototype-cleanup.json记录两证书不存在与包数0。
