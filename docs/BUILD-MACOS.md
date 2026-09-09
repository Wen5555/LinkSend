# macOS 测试 DMG 构建

本轮目标是可安装测试包，不做 Developer ID 签名、公证或 App Store 发布。Wails 3 固定为 `v3.0.0-beta.18`，Go/Node/pnpm 版本分别为 Go 1.26.5、Node 22.15.0、pnpm 11.19.0。

## 获取候选源码

在 Mac 上检出本轮任务分支或已核对的 commit（不要把未审查的 `main` 当候选）：

```sh
git fetch origin
git checkout <任务分支或commit>
git rev-parse HEAD
```

## 构建

仓库根目录执行：

```sh
bash scripts/build-macos.sh --arch arm64 --format dmg
bash scripts/build-macos.sh --arch amd64 --format dmg
```

脚本会检查 Xcode Command Line Tools、`go`/`node`/`pnpm`/`hdiutil`/`plutil`/`lipo`/`codesign`、Wails 3 版本，设置 `GOWORK=off`，冻结安装前端依赖，生成绑定和图标，调用：

```sh
wails3 task darwin:package:dmg ARCH=arm64 BIN_DIR=bin/macos-arm64
wails3 task darwin:package:dmg ARCH=amd64 BIN_DIR=bin/macos-amd64
```

输出分别为 `apps/desktop/bin/macos-arm64/LinkSend.dmg` 与 `apps/desktop/bin/macos-amd64/LinkSend.dmg`，旁边生成 `SHA256SUMS.txt`。脚本只做 ad-hoc 签名并验证 bundle，不声称已公证。

## 校验

```sh
shasum -a 256 apps/desktop/bin/macos-arm64/LinkSend.dmg
hdiutil imageinfo apps/desktop/bin/macos-arm64/LinkSend.dmg
lipo -info apps/desktop/bin/macos-arm64/LinkSend.app/Contents/MacOS/LinkSend
plutil -p apps/desktop/bin/macos-arm64/LinkSend.app/Contents/Info.plist
```

首次打开：将 DMG 中的 `LinkSend.app` 拖入 `/Applications`，若系统提示未验证，进入“系统设置 → 隐私与安全 → 仍要打开”针对该应用确认。不要关闭 Gatekeeper 或批量清除隔离属性。
