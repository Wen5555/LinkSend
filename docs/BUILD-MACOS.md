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

## 本轮 CI 产物（2026-09-10）

GitHub Actions run [34387753173](https://github.com/Wen5555/LinkSend/actions/runs/34387753173)
在 commit `555c3190c4c5f13a52eebe9b9a8f6208abb7ee7d` 的 Windows、macOS arm64、macOS amd64
矩阵均通过（`fail-fast: false`）。artifact 名称分别为：

- `LinkSend-wails3-preview-windows-amd64-555c3190c4c5f13a52eebe9b9a8f6208abb7ee7d`
- `LinkSend-wails3-preview-macos-arm64-555c3190c4c5f13a52eebe9b9a8f6208abb7ee7d`
- `LinkSend-wails3-preview-macos-amd64-555c3190c4c5f13a52eebe9b9a8f6208abb7ee7d`

包内 SHA256（以 `SHA256SUMS.txt` 为准）：

```text
5ff0217fab3b257999eae710f29bc76f08979b37100b2f1b4347546a6d3645a2  LinkSend-wails3-preview-macos-arm64-555c3190c4c5f13a52eebe9b9a8f6208abb7ee7d.dmg
653c2ceb098c5d81bc4c6f0a351cb20b446772c206888463aec7b34efff73819  LinkSend-wails3-preview-macos-amd64-555c3190c4c5f13a52eebe9b9a8f6208abb7ee7d.dmg
1c43b7576118c5480d139bd691a1c84b8c3d09f67d96f8815c1877c4055f8f80  apps/desktop/bin/LinkSend-wails3-preview-windows-amd64-555c3190c4c5f13a52eebe9b9a8f6208abb7ee7d.zip
```

Windows 端 ZIP 含 `LinkSend.exe` 和 `README-WINDOWS-TEST.txt`；两个 DMG 均含完整
`LinkSend.app`、`README-MACOS-TEST.txt`、Applications 拖拽入口和对应架构的 Mach-O。
离线结构检查还确认 `CFBundleIdentifier=com.linksend.desktop`、Go 1.26.5、Wails 3
`v3.0.0-beta.18`。当前 Windows 主机无法执行 `hdiutil` 挂载、`codesign --verify` 或原生
WebKit 窗口操作，这些项目保持 `NOT_RUN`/`BLOCKED_BY_EXTERNAL_ENV`。
