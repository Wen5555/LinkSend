# M0 工具链平台兼容性纠正

日期：2026-09-12。发现来源：物理 Mac 的原生 adapter 链接探针。

Go 1.27.1 虽然能编译现有 Go modules，并在当前 Windows/macOS26.5 上通过测试，
但它不符合本项目仍承诺的 macOS12 最低系统版本。先前只对照依赖 go.mod 的最低语言版本、
并保持 plist/build target 为12.0，遗漏了 Go 工具链自身的最低操作系统要求。

确定证据：

- [Go1.27 官方发行说明](https://go.dev/doc/go1.27#darwin)：
  “Go 1.27 requires macOS 13 Ventura or later; support for previous versions has been discontinued.”
- 同页 Linker 段将默认最老支持版本说明为13.0.0。
- 官方 Go1.27.1 源码 `src/cmd/link/internal/ld/macho.go` 默认 `macVersionFlag{13,0,0}`。
- Mac 原生构建实际警告 `go.o built for macOS13.0`。只强写 `-mmacosx-version-min=12.0`
  或 plist 的 `LSMinimumSystemVersion` 不能降低 Go runtime 的实际要求。

最终决策由用户于2026-09-12明确确认：**最低支持 macOS13，取消 macOS12支持，使用 Go1.27.1**。
因此所有构建与 CI 使用 Go1.27.1，Wails仍为beta.18并用相同Go编译；plist、CGo链接目标同步为13，
前端显式Safari16/Chrome109。Node24.21.0、pnpm12.4.1、React/TS/Vite/Vitest的已验证升级保留。
用户全局 Go、Homebrew、PATH 和网络配置不修改。

确认之前曾按原macOS12承诺安装并验证Go1.26.8，官方声明它是最后支持macOS12的版本。
该工具保留为兼容研究记录，当前项目不使用；原本计划的回退已被上述用户产品决策取代。

`b3d5fc7` 的 Windows、Mac26.5、CI 和香港部署是实际通过的历史证据；该提交的1.27产物
不用于声称 macOS12 兼容，尚未发布 M0 Release。用户同时授权 **M0与M1合并发布**；最终发布使用
包含新最低系统版本声明与M1功能的精确提交，重新运行双模块/原生/CI检查及相应事务部署。

新的最小系统版本声明还需与最终 Mach-O load commands 一致；当前物理 Mac 是26.5，
无法据此声称已经在真实 macOS13 系统运行。
