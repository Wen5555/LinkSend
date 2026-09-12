# M1 官方 CI macOS 安装包原生复核

2026-09-12，复核 GitHub Actions workflow `34676198796`、精确提交 `2f2656d940c96e142d5639b2bc3012ab0ce5fb6e` 的产品 0.5.0 / 协议 V1 安装包。本记录使用下载的 CI DMG，不复用 [Mac 源码构建记录](DESKTOP-M1-MAC.md) 中的另一个 DMG。

## 包来源与静态验证

Windows 下载后已检查 CI 归档与包内 SHA256SUMS，上传实体 Mac 后再次计算下列 DMG SHA256。原始 `BUILD-INFO.txt` 保持原样：`native_validation=not_run_in_workflow` 没有被改写，本文件单独记录后续实体机验收。

| 架构 | DMG SHA256 | 镜像内 executable SHA256 |
| --- | --- | --- |
| arm64 | `b0e029581ba075b9adf0df43bf94742882f128dc0c4ce7b6f22986b9c534b378` | `b9d47a8b039f530b415169db33a845634adc6177ec08db8f1f346bcadd1aa918` |
| amd64 / x86_64 | `7e116b673bf75ab497ce932d884908225fa0f38405effefb96d8fa8f4989d563` | `e1e213881d229bbe009b68c601f9a31af012d7e16ee683debcaf6124b3b02dab` |

两份 `BUILD-INFO.txt` 的 `source_commit`、`workflow_head_sha` 均匹配精确提交，`source_state=COMMITTED`、`source_checkout_clean=true`；使用 Go 1.27.1、Node 24.21.0、pnpm 12.4.1、Wails beta.18。来源依据是经哈希核对的 CI 资产元数据；Wails 二进制使用 `-buildvcs=false`，不声称可从二进制读取 Git 提交。

通过 SSH manager alias `mac-test-102342413` 在 macOS 26.5 / arm64 上将两份 DMG 只读挂载，实际执行：

- `shasum -a 256 -c -`：两份 DMG 通过。
- `codesign --verify --deep --strict <LinkSend.app>`：两份通过，ad-hoc 签名、未公证。
- `lipo <executable> -verify_arch arm64` / `x86_64`：与各自包架构一致。
- 直接解析 Mach-O load commands：两份最低版本 `13.0.0`、SDK `15.5.0`，与各自 plist 的 `LSMinimumSystemVersion=13.0.0` 一致；bundle ID `com.linksend.desktop`、产品版本 `0.5.0`。
- `ditto` 复制到全新独立目录后，用 `cmp` 核实 executable 与仍挂载的镜像内原件完全相同，再正常卸载镜像。

成功 job `/tmp/codex-ssh/desktop-m1-ci-mac-verify-v2-20260912T061947Z` 退出 0。首次 job `/tmp/codex-ssh/desktop-m1-ci-mac-verify-20260912T061723Z` 已通过 DMG 哈希，但因验证脚本把 lipo 输入路径放在 `-verify_arch` 后而退出 1；修正参数顺序后使用全新 v2 目录重跑，不将该脚本失败解释为产品失败或忽略记录。

## 精确 arm64 包实体机启动与退出

job `/tmp/codex-ssh/desktop-m1-ci-mac-native-smoke-20260912T062024Z` 使用从 CI DMG 得到的 `/Users/wen/linksend-desktop-six/m1-ci-2f2656d-v2/arm64/LinkSend.app`，启动前 executable SHA256 与上表一致。创建独立 `native-profile`，仅设置无人监听的 loopback 信令地址和显式开发开关，未访问用户 profile。

实际结果：测试 PID `59206`；CoreGraphics 查到该 PID 的真实可见普通窗口 ID `7987`，大小 `1008 × 684`；NSRunningApplication 的正常退出请求被接受，进程退出码 0。原用户 LinkSend 0.3.0 进程 PID `47950` 保持运行。未替换 `/Applications/LinkSend.app`，未关闭用户应用，未改变系统权限、网络或剪贴板。

此处通过项为精确 CI arm64 包的实体窗口启动和空闲正常退出；amd64 仅通过包、签名、架构和元数据验证。未执行 Intel 实机、macOS 13 实机、Gatekeeper 双击安装、完整原生控件点击、Finder Services、通知点击、登录启动、睡眠/唤醒或网络传输，不能据此增加这些项目的 PASS。

## 保留证据

证据导出 job `/tmp/codex-ssh/desktop-m1-ci-mac-export-20260912T062124Z` 退出 0。归档包含两架构包元数据、codesign 与 Go build 信息、原始 BUILD-INFO、首次失败及成功脚本的 stdout/stderr/退出码、原生探针源码和窗口/应用日志，不含测试 profile 身份文件。

归档 `m1-ci-mac-evidence-2f2656d.tar.gz` 为 12,726 bytes，SHA256 `5466da7823953834da2713ea9c0f9ec81d76e6892974ec760a8b618fa08bc969`，经 manager 下载后再次哈希一致。本地位置 `D:/apps/Osend/.artifacts/desktop-six-features/m1-ci-mac-native/`；远端位置 `/Users/wen/linksend-desktop-six/m1-ci-2f2656d-v2/`。
