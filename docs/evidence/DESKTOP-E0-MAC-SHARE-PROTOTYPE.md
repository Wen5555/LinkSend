# E0-04 Mac 真 Share Extension 原型

后续安全修补与实测见 [E0 Mac SafeIO回执](DESKTOP-E0-MAC-SAFEIO.md)：10项原生文件边界测试PASS，真实标准Share菜单动作已派发，分离服务复测仍返回NSCocoaErrorDomain/4097；文件交接继续FAIL/待修。下面保留较早原型证据，不能复用其hash代表新包。

2026-09-13。状态 PARTIAL：真实host/.appex构建、ad-hoc签名、系统注册、共享服务发现/激活和注销状态有证据；**文件接管、后台冷启动交接和退出后可读尚未通过**。不能称U6完成。
目标为登记alias mac-test-102342413，wen@10.234.39.151，macOS26.5/ARM64；不是最低macOS13或Intel实机。

## 工具与凭据

Command Line Tools：Apple Swift6.3.2，target arm64-apple-macosx26.0；构建显式最低macOS13.0。
`xcodebuild -version` 报仅有CLT、缺完整Xcode；`security find-identity -v -p codesigning` 为0有效身份。
`AXIsProcessTrusted()` 返回false。没有更改TCC、购买证书或把ad-hoc当Developer ID/公证。

## 实际原型

独立bundle `com.linksend.e0.sharehost`，内嵌 `com.linksend.e0.sharehost.extension`，扩展点 `com.apple.share-services`。
Swift/Cocoa自定义NSViewController；真实App Group `group.com.linksend.e0.share`、sandbox与user-selected.read-only entitlements。
原型仅处理一个1MiB以内fixture，没有设备列表或网络发送。系统临时file representation须在回调内接管；file-URL单列security-scoped路径，不把序列化URL字节当文件。
Apple官方NSItemProvider与NSExtensionContext文档已实际读取：临时文件在completion handler返回后删除；open能否工作由具体extension point决定，不能假定后台一定能唤起。

初版错误地链接为dylib：codesign通过但pluginkit无匹配。修正为`-application-extension -parse-as-library -Xlinker -e -Xlinker _NSExtensionMain`后，file显示Mach-O executable，pluginkit开始列出真实扩展。保留该失败，不能将初版构建PASS冒充注册PASS。
后续对view生命周期和唯一ObjC controller名做诊断修补，仍未取得captured.json；该回调问题尚待定位，不把它直接归因为缺签名/AX权限。

## 验证结果

| 检查 | 实际结果 |
|---|---|
| swiftc host与extension、strict deep codesign | PASS，ad-hoc |
| App Group entitlements落入签名 | PASS（元数据）；不等于跨进程文件权限通过 |
| lsregister/pluginkit添加、按精确ID查询 | PASS，真实嵌入appex |
| NSSharingService列表出现LinkSend E0 | PASS |
| perform(withItems:)系统激活 | PASS，真实ShareKit/ExtensionKit进程与“Service window did show”日志 |
| Finder人工菜单点击 | NOT RUN，NSSharingService测试源不替代Finder |
| App Group目录存在 | PASS；没有captured.json成功回执 |
| 真实文件接管、hash、临时表示失效检查 | NOT PASS，未取得所需回执 |
| 后台冷启动/open返回、扩展退出后host读取 | NOT RUN（依赖接管成功） |
| 注销/进程回收 | 最终精确IDno matches，无原型进程；见下述失败与复核 |
| 普通用户安装/升级/正式签名/公证 | NOT RUN |

最后源码快照构建（并非已提交发行构建）host SHA256：`64c2ec17f4100e313162ceb28691ac6f01e6039542c6f53f984810f6ac24c20d`；extension SHA256：`20bff53203d963be756fbe58471a8c6ab7e014414c6f0d00cc3d463e66599caf`。
可审查.app归档：`.artifacts/desktop-experience/e0/LinkSend.E0.MacSharePrototype.tar.gz`，SHA256 `f3bcf2ce9ce53ca7a44dcf83ca4d6248a6171790dbcb696cd07e16ff3442c62a`，manager下载后重算一致。

## 关键作业与退出码

全部经manager run-script/upload/download；完整日志保留在本机ignored e0目录。

| /tmp/codex-ssh/ 下job | 退出/范围 |
|---|---|
| linksend-e0-build-mac-share-20260912T182138Z | 0，初版dylib编译/签名，仅构建 |
| linksend-e0-register-mac-share-20260912T182323Z | 0，注册查询no matches，不能称成功 |
| linksend-e0-rebuild-mac-share-20260913T024841Z | 0，可执行入口修复、实际1 plugin |
| linksend-e0-activate-mac-share-20260913T025212Z | 0，真实系统服务及扩展窗口 |
| linksend-e0-activate-mac-url-20260913T025707Z | 0，fileURL路径诊断，未产生接管回执 |
| linksend-e0-mac-group-diagnostic-20260913T030107Z | 0，补诊断构建/系统激活；无应用成功回执 |
| linksend-e0-mac-view-lifecycle-20260913T030345Z | 0，viewDidLoad路径；无应用成功回执 |
| linksend-e0-mac-unique-controller-20260913T030612Z | 0，当前源码快照；无应用成功回执 |
| linksend-e0-unregister-mac-prototype-20260913T030823Z | 1，host terminate accepted，lsregister已移除插件后冗余pluginkit -r报no plugin |
| linksend-e0-verify-unregister-mac-20260913T030903Z | 1，再次lsregister -u报-10814，未当成验证成功 |
| linksend-e0-unregister-final-state-20260913T030932Z | 0，先查精确注册状态，no matches且无原型进程 |

没有删除原用户LinkSend.app/profile；原型bundle与隔离Group Container留作审查，未创建任何发送任务。注销不等于删用户文件。
下一步：先定位自定义controller回调/附件交付，取得真实文件授权证据，再做后台冷启动、临时表示期限、Finder与安装矩阵。正式身份与系统人工权限缺口另行补齐，不能降低身份验证替代。
