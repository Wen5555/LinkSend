# M0 依赖兼容性核查与升级记录

核查日期：2026-09-12。基线：`main` / `fbfc250159ea65dbb5840d6dadb03fe4423cf868`。
开始时有未跟踪的规划、`.playwright-cli/`、iOS/Linux build 文件，全部保留；没有 `.codegraph/` 索引。
本记录区分官方发布与声明兼容、实际安装/编译和原生验收。版本存在不代表本项目已经通过测试。

## 1. 官方证据与组合决策

以下请求均为 `Invoke-WebRequest` 访问官方公开 HTTPS 元数据，逐请求记录 HTTP 状态；汇总脚本退出 0。原始筛选元数据（含 npm integrity）保存在 `.artifacts/desktop-m0-dependencies-20260912/{npm-official,official-releases}.json`，不包含 token 或代理配置。

| 依赖 | 精确目标 | 本次官方结果 / 约束 | 决策 |
|---|---|---|---|
| Go | 1.27.1 | [下载 JSON](https://go.dev/dl/?mode=json&include=all) HTTP 200，`stable=true`，Windows amd64 ZIP 存在 | 升级两个 module 与 workspace toolchain，单独回归签名/JSON/manifest/恢复 |
| Node | 24.21.0 | [发行索引](https://nodejs.org/dist/index.json) HTTP 200，2026-09-07 发布、LTS `Krypton` | 本地便携安装；CI 同步精确版本 |
| pnpm | 12.4.1 | [精确包](https://registry.npmjs.org/pnpm/12.4.1) HTTP 200，Node `>=18.*` | 精确锁定 packageManager；保留唯一项目 pnpm lockfile |
| React / React DOM | 19.3.0 | [React](https://registry.npmjs.org/react/19.3.0)、[React DOM](https://registry.npmjs.org/react-dom/19.3.0) HTTP 200；DOM peer `react: ^19.3.0` | 同步升级 |
| TypeScript | 6.0.3 | [精确包](https://registry.npmjs.org/typescript/6.0.3) HTTP 200；Node `>=14.17` | 采用 6.0.3 |
| typescript-eslint | 8.70.0 | [精确包](https://registry.npmjs.org/typescript-eslint/8.70.0) HTTP 200；TS `>=4.8.4 <6.1.0`，ESLint `^8.57.0 || ^9.0.0 || ^10.0.0` | 保留 ESLint / @eslint/js 9.39.4，升级 TS lint 适配 |
| Vite | 8.3.0 | [精确包](https://registry.npmjs.org/vite/8.3.0) HTTP 200；Node `^20.19.0 || >=22.12.0` | 与 React plugin 一起升级，显式 WebView target |
| @vitejs/plugin-react | 6.1.1 | [精确包](https://registry.npmjs.org/@vitejs/plugin-react/6.1.1) HTTP 200；Vite `^8.0.0`；额外 compiler/Babel peers 是可选能力 | 采用普通 React 编译，不开启实验 compiler |
| Vitest | 5.0.0 | [精确包](https://registry.npmjs.org/vitest/5.0.0) HTTP 200；Node `^22.12.0 || ^24.0.0 || >=26.0.0`，Vite `^6.4.0 || ^7.0.0 || ^8.0.0` | 与 Vite 8 配套验证 |
| TanStack Query | 5.102.8 | [精确包](https://registry.npmjs.org/@tanstack/react-query/5.102.8) HTTP 200；React `^18 || ^19` | 用于查询快照/分页缓存，命令禁用自动重试 |
| TanStack Virtual | 3.14.12 | [精确包](https://registry.npmjs.org/@tanstack/react-virtual/3.14.12) HTTP 200；React / DOM peers 包含 `^19.0.0` | 暂不引入；先后端分页、再以实际瓶颈决定 |
| Radix Dialog | 1.1.23 | [精确包](https://registry.npmjs.org/@radix-ui/react-dialog/1.1.23) HTTP 200；React / DOM peers 包含 `^19.0` | 按 UI 实际需要引入，无必要不安装 |
| Wails Go / CLI / runtime | 3.0.0-beta.18 | [beta.18 release](https://api.github.com/repos/wailsapp/wails/releases/tags/v3.0.0-beta.18) 和 [runtime](https://registry.npmjs.org/@wailsio/runtime/3.0.0-beta.18) HTTP 200；release `prerelease=true` | 保留三者同版，不回迁 Wails 2 |
| Wails 候选 | 3.0.0-beta.20 | [release](https://api.github.com/repos/wailsapp/wails/releases/tags/v3.0.0-beta.20) 和 [runtime](https://registry.npmjs.org/@wailsio/runtime/3.0.0-beta.20) HTTP 200；2026-09-10 发布，`prerelease=true` | 当前六项无需靠升级解决，暂不迁移 |

本次没有“无法验证却写为已发布”的候选版本。TypeScript 7.0.2 官方 npm 也存在，但不在 typescript-eslint 8.70.0 peer 范围；[TS 7 公告](https://devblogs.microsoft.com/typescript/announcing-typescript-7-0/) 本次 HTTP 200。不使用 force、忽略 peer 或禁用 lint。

Go 依赖的缓存固定源码 `go.mod` 最低版本：Pion ICE 4.4.2 要求 Go 1.24.0，quic-go 0.62.0 要求 1.26.0，modernc SQLite 1.58.0 要求 1.25.0，Wails beta.18 要求 1.25.0。因此声明层面接受 Go 1.27.1；运行正确性仍须测试。

## 2. 实际基线与原生边界

| 命令 / 核查 | 退出码 | 观察 |
|---|---:|---|
| `node C:\Users\Wen\.codex\skills\web-access\scripts\check-deps.mjs` | 0 | Node22、Chrome CDP 和 proxy ready；官方元数据使用静态公开 HTTP，不操作用户 tab |
| `node --version` / `npm --version` / `pnpm --version` / `go version` | 均 0 | v22.15.0 / 11.12.1 / 11.19.0 / go1.26.5 windows/amd64 |
| `.tools\bin\wails3.exe version` | 0 | v3.0.0-beta.18 |
| `go version -m .tools\bin\wails3.exe` | 0 | CLI 源模块 beta.18，Go1.26.5 编译，CGO_ENABLED=1 |
| `.tools\bin\wails3.exe doctor` | 0 | Windows 11 build26200、WebView2 152.0.4191.66、NSIS3.12；可开发 |
| 固定 Wails 缓存源码符号检索 / 最低 macOS 配置读取 | 0 | 见下述 API 与平台证据 |

现有 Node22.15.0 已满足候选包 engines，升级 Node 是独立构建环境更新。`.tools/bin/wails3.exe` 存在但未全局加入 PATH。Go 模块缓存与 pnpm node_modules 缓存已有既有依赖。

Wails doctor 同时明确：MSIX Packaging Tool、MakeAppx、SignTool 未安装，Windows/macOS 签名未配置，Docker daemon 未运行。doctor 成功不等于 Share Target、签名、公证、托盘通知和安装卸载已经验收。

固定源码 `github.com/wailsapp/wails/v3@v3.0.0-beta.18` 已确认：

- `pkg/application/single_instance.go` 的 `SingleInstanceOptions`、`OnSecondInstanceLaunch` 与 `SecondInstanceData.Args/WorkingDir`；profile 键和 CLI/GUI 写入归属仍由产品实现。
- `pkg/application/systemtray.go`、窗口 `WindowFilesDropped` 与 notifications service 确实存在；未据此声称原生菜单/通知已操作成功。
- `pkg/application/clipboard.go` 与 `clipboard_manager.go` 只有 `Text/SetText`；截图必须经过薄原生适配。
- 当前 `build/darwin/Info.plist` 的 `LSMinimumSystemVersion=12.0.0`，Taskfile 使用 `-mmacosx-version-min=12.0`；升级不提高此部署目标。Vite target 明确为 Chrome109 / Safari15（macOS12 初始 Safari 系列），实际 Web API 支持仍单独验收。

[Go 1.27 notes](https://go.dev/doc/go1.27)、[Vitest migration](https://vitest.dev/guide/migration) 本次 HTTP 200。Vite 网站 migration 页面首次静态请求 TLS 连接失败；随后 [Vite 8.3.0 tag 的官方迁移原文](https://raw.githubusercontent.com/vitejs/vite/v8.3.0/docs/guide/migration.md) HTTP 200（21,418 字符），确认默认浏览器目标更新以及 Rolldown/Oxc 替代 Rollup/esbuild。实际源码已留存 `vite8-migration.md`，显式 target 不依赖新默认值。

## 3. 升级与回滚边界

审计文档先落盘，随后由同一依赖负责人串行修改：两份 `go.mod`、`go.work`、前端 `package.json` / 唯一 `pnpm-lock.yaml`、前端 tsconfig / Vite config、三份 GitHub workflow 工具版本。UI/应用/协议功能文件由其他实现负责人维护。

新工具只解压至被忽略的 `.tools`；不改全局 Go/Node/pnpm、代理或注册表。变更前把上述依赖文件复制到 `.artifacts/desktop-m0-dependencies-20260912/rollback/`。回滚时仅恢复本轮依赖文件，并按旧 lockfile frozen install；不回滚用户数据或其他代理的实现。

依赖安装、哈希与实际探针结果追加在下面。完整双模块测试、race、原生生产包和跨平台验收由集成阶段执行，不能以本段的元数据兼容结论代替。

## 4. 升级执行结果

以下命令在 PowerShell7 当前进程加载 `. .\.tools\use-desktop-toolchain.ps1` 后执行；该本地忽略脚本把项目 Go/Node/pnpm/Wails 加到进程 PATH，并设置 `GOTOOLCHAIN=local`。独立 module 探针明确设置 `GOWORK=off`。

| 实际命令 / 操作 | 退出码 | 结果 |
|---|---:|---|
| curl 官方 Go ZIP → SHA256 对照下载 JSON → Expand-Archive → 便携 `go version` | 0 | `go1.27.1 windows/amd64` |
| curl 官方 Node ZIP → SHA256 对照同版本 SHASUMS256.txt → Expand-Archive → `node --version` | 0 | `v24.21.0` |
| curl pnpm12.4.1 npm tarball → SHA512 对照官方 dist.integrity → tar 解压 | 0 | 包完整性相符 |
| 旧习惯入口 `node …/pnpm.cjs` | 1 | `MODULE_NOT_FOUND`；pnpm12 已使用 Rust native，未计为成功 |
| 官方 `@pnpm/exe.win32-x64@12.4.1` tarball → SHA512 验证 → 按包源码安装 native → `pnpm --version` | 0 | `12.4.1`；修复本地 wrapper，保留官方 dist/node-gyp payload |
| `pnpm --dir apps/desktop/frontend install --strict-peer-dependencies --registry=https://registry.npmjs.org` | 0 | 精确目标依赖安装，276 entries 供应链策略通过，没有忽略 peer |
| 前端 `pnpm install --frozen-lockfile --strict-peer-dependencies --registry=https://registry.npmjs.org` | 0 | 唯一 pnpm lockfile 可重放 |
| 前端首次 `pnpm run typecheck` | 2 | TS5107：旧 `esModuleInterop=false` 弃用；随后改为 `true`，没有 ignoreDeprecations |
| 前端修复后 `pnpm run typecheck` | 0 | App、Vite config、独立 bindings tsconfig 全部通过；bindings `skipLibCheck=false` |
| 前端 `pnpm run lint` | 0 | 无新增 lint 豁免 |
| 前端 `pnpm run test` | 0 | Vitest5.0.0，1 个 test file / 11 tests PASS |
| 前端 `pnpm run build` | 0 | Vite8.3.0 production、47 modules；本次资源 `index-BmIm2Dha.js`，278.54kB / gzip87.90kB |
| 根 `GOWORK=off go mod verify` | 0 | all modules verified |
| 根 `GOWORK=off go test ./internal/protocol ./internal/identity ./internal/transport` | 0 | Go1.27.1 下协议/身份/传输全部 PASS |
| 桌面 `GOWORK=off go mod verify` | 0 | all modules verified |
| `git diff --check` | 0 | 依赖升级整合时通过 |

官方存档完整性：

- Go ZIP SHA256：`a3911b5e0e1b1053f25ed0675f4c1c6aad1e2bfcf253df2b9be4caabd2edd95d`。
- Node ZIP SHA256：`158f7685b44de51f6c0df1d153526cbcd3e1bc739a8dfc607721cef75de9e541`。
- pnpm npm 包 SRI：`sha512-LoHjmdc/6DkNqyXgaqeIq3pZCCSNL1o3D4K0gRR6ano2e/gEj5pv22Rg8hpm8FQt7bi5TKLIcjWWdBkgsWVtTA==`。
- pnpm Windows native 包 SRI：`sha512-x7gJHZgHo6hp354xCYA2NvoFzYJkHwovt2kVsUBK5EmXULLcP51JGMq09CX+5FRFJUZFKoXjLu3mZWmfP7o6PQ==`。

前端 `packageManager=pnpm@12.4.1`、`engines.node=^24.21.0`；两份 module 与 workspace `go=1.27.0` / `toolchain=go1.27.1`。三份 GitHub workflow 同步 Go1.27.1 / Node24.21.0 / pnpm12.4.1，Windows 多命令步骤增加逐条退出码检查，避免前面失败被最后成功掩盖。Wails 保留 beta.18；前端和 packaging 产品版本经本轮集成决定提升为 0.5.0，M0 阶段标识由测试预发布 tag `v0.5.0-m0` 区分；此处不声明 Release 已发布。

安装日志有 ESLint9.39.4 与间接 glob10.5.0 的 deprecated 警告；本次保留已经与 TS-eslint peer、实际 lint 通过的 ESLint 9，不把警告描述为失败或升级为未核实版本。现有 `pnpm-workspace.yaml` 仅含构建许可及 Wails 发包等待排除策略，没有创建第二个 package/workspace。

本节首次交接时未执行 Wails production build、bindings 再生成、全量双模块/race、macOS 实机、系统入口/通知/托盘、网络矩阵；这些由随后串行整合和原生验收执行。当时现有 bindings 已用新编译器单独检查通过；后来接手的实际绑定再生成和 CLI 修复见下一节。

## 5. Wails CLI 编译工具链兼容修复

随后的 M0 集成实际发现：虽然 `.tools/bin/wails3.exe version` 为 beta.18，该 CLI 最初由 Go1.26.5 编译。主线程在 Go1.27.1 环境执行 `wails3 generate bindings -ts` 返回 0，却有 21 条 warnings，不能计为生成通过。主线程报告的首例为 `math/rand/v2/rand.go:213: method must have no type parameters`，另有 `this application uses version go1.26 of source-processing packages but runs version go1.27 go list`。旧完整输出只存在该次工具输出，未保存独立日志，本记录没有补造旧日志。

修复步骤与实际结果：

1. 旧 CLI 复制至 `.artifacts/desktop-m0-dependencies-20260912/rollback/tools/wails3-go1.26.5-beta.18.exe`，未删除。
2. 当前进程用便携 Go1.27.1，`GOWORK=off`、`GOBIN=.tools/wails3-go127-bin`，执行 `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.18`，退出 0。不修改 Wails、x/tools 或其他模块版本。
3. `go version -m` 确认新 CLI 由 Go1.27.1 编译，源模块仍 beta.18；替换 `.tools/bin/wails3.exe`。新 CLI SHA256：`2dd3fe8072981b6c20177c051f01086a798199062a692de557dee55bd88445db`。完整 build info 保存为 `wails3-go127-buildinfo.txt`。
4. 桌面目录 `GOWORK=off wails3 generate bindings -ts -i -clean=true` 退出 0，处理 406 packages、1 service、33 methods、17 models；日志 warning 匹配数 0。完整日志为 `.artifacts/desktop-m0-dependencies-20260912/bindings-go127.log`。

生成后 `pnpm --dir frontend run typecheck:bindings` 与 `pnpm --dir frontend run typecheck` 均退出 0。首次复查错误地把 `--dir frontend` 放在脚本参数末尾，使 pnpm 在 desktop 目录查找 package.json，返回 `ERR_PNPM_NO_IMPORTER_MANIFEST_FOUND` / 退出 1；修正命令参数位置后通过，未改动绑定规避检查。

Wails production build 仍由集成负责人串行运行。CI 已先 setup Go1.27.1 再安装同版本 Wails CLI，避免复用旧 Go 编译的 CLI。
