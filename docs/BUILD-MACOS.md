# macOS 测试 DMG 构建

当前产品版本为 `0.2.0`，文件协议为 V1，Wails 3 固定为 `v3.0.0-beta.18`。测试包在没有 Developer ID/公证凭据时只能标记为 ad-hoc、NOT_NOTARIZED，不得描述为正式发行。

## 获取可追溯源码

在 Mac 上检出明确 commit，并确认工作树干净。不能把旧 EXE/.app/DMG 重新装包后改写 commit 元数据。

```sh
git fetch origin
git checkout <reviewed-commit>
git rev-parse HEAD
git status --porcelain
go run ./cmd/linksend --version
```

## 构建

仓库根目录串行执行：

```sh
bash scripts/build-macos.sh --arch arm64 --format dmg
bash scripts/build-macos.sh --arch amd64 --format dmg
```

脚本检查 Xcode Command Line Tools、Go/Node/pnpm、`hdiutil`/`plutil`/`lipo`/`codesign` 和 Wails 3 版本，设置 `GOWORK=off`，冻结安装前端依赖，重新生成 bindings/dist，然后调用 Wails 3 DMG task。输出位于：

```text
apps/desktop/bin/macos-arm64/LinkSend.dmg
apps/desktop/bin/macos-amd64/LinkSend.dmg
```

前端构建与桌面编译不可并行。每个最终资产应使用 `LinkSend-v0.2.0-macos-<arch>-<shortsha>.dmg`，并记录 commit、workflow run/head SHA（本地构建则明确为 null）、`vcs.modified`、工具版本、UTC 时间、大小、SHA256、签名、公证和原生验证状态。

## 校验

```sh
shasum -a 256 apps/desktop/bin/macos-arm64/LinkSend.dmg
hdiutil imageinfo apps/desktop/bin/macos-arm64/LinkSend.dmg
hdiutil attach -readonly -nobrowse apps/desktop/bin/macos-arm64/LinkSend.dmg
plutil -p /Volumes/LinkSend/LinkSend.app/Contents/Info.plist
lipo -info /Volumes/LinkSend/LinkSend.app/Contents/MacOS/LinkSend
codesign --verify --deep --strict --verbose=2 /Volumes/LinkSend/LinkSend.app
codesign -dv --verbose=4 /Volumes/LinkSend/LinkSend.app
```

必须核对 `CFBundleIdentifier=com.linksend.desktop`、`CFBundleShortVersionString=0.2.0`、目标架构和嵌入二进制的真实 Go revision。挂载后及时卸载测试卷。首次打开若提示未验证，只能在“系统设置 → 隐私与安全”对该应用单独确认，不能关闭 Gatekeeper 或批量移除隔离属性。

## 原生验收边界

arm64 的原生窗口创建和 idle quit Apple Event 已有 r2 测试快照证据；文件/目录选择器、打开目录、红点实际点击、Cmd+Q 按键、活跃任务保护、恢复入口和 Intel Mac 启动仍需对 `0.2.0` 提交构建重跑。详见 [MAC-SMOKE-TEST.md](MAC-SMOKE-TEST.md)。

## 历史资产

2026-09-10 的 run `34387753173` 和 commit `555c3190c4c5f13a52eebe9b9a8f6208abb7ee7d` 是 `0.1.0` 历史测试快照，只证明当时的构建链和包结构；其 SHA256 和文件名保留在 PROGRESS 历史记录中，不能作为 `0.2.0` 资产复用。
