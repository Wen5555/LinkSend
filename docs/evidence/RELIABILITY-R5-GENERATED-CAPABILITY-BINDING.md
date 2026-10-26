# R5能力字段生成绑定同步

日期：2026-10-26。只同步既有 Go 能力结构对应的 TypeScript 派生接口，不改 core、helper、权限、文件 wire、来源算法或发行 gate。

## 准确失败

准确 `850b9d115ac2a2e460a0678039a1642cd2c1de9b` 第1次 push core `40271024250`、desktop `40271024385` 及 PR 事件 core `40271029276`、desktop `40271029230` 全部成功；PR 事件不是 rerun。packages `40271024300` 中 Windows job `121403715174` 在 `Build Windows amd64` 真实失败。

独立 job 原日志 `.artifacts/r2-helper-ci-850b9d1/windows-job-original.log:572-575` 明确只列一个 dirty：`apps/desktop/frontend/bindings/github.com/Wen5555/LinkSend/internal/protocol/models.ts`。实际 helper、CGO0桌面和NSIS均编译完成，builder保持 `source clean=False`，随后由 `Clipboard helper source/platform mismatch` 拒绝发行打包。没有上传或下载Windows发行资产，没有把来源改写为clean，没有重跑旧CI或运行候选。`gh run view --log` 因Mac仍运行而拒绝取整体日志后，改用现有GitHub job logs只读端点保存本job日志，没有等待Mac或改变其运行。

## 最小修复与验证

在新ignored目录 `.artifacts/r2-helper-ci-850b9d1/source` 用本地共享Git对象创建准确850 detached checkout，首次 `git status --porcelain` 为零项/exit0，机器回执 `BINDING-SOURCE-INITIAL.json` 保留。只运行锁定 Wails beta.18 的既有命令：

```text
GOWORK=off wails3 generate bindings -f '-tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"' -clean=true -ts -i
```

命令exit0，仅同一protocol `models.ts`改变；差异只有在既有 `Capabilities` 接口增加 `"share_grants_v1"?: boolean;`。该可选JSON字段已在R5 Go结构中实现，本次补齐生成结果，不创造新Go字段或授权行为。主树目标文件先确认无其它修改后，只同步该一个派生文件。

同一隔离source执行一次 `pnpm run build`（包含既有typecheck与Vite build）exit0，结束后Git仍仅该TS文件有差异，已跟踪dist没有变化。不新增依赖、不重复完整前端test/lint矩阵、不重编Go或运行GUI/安装/系统clipboard。

复现日志 `bindings-reproduction.log`、`frontend-reproduction.log`、机器回执 `BINDING-REPRODUCTION.json` 及本增量冻结清单保存在同ignored目录。来源clean算法、确定syso豁免、完整输入摘要/前后稳定性和发行拒绝gate均保持。原850 failure保留；新提交的准确Windows job与实际ZIP/installer/helper静态同包验收仍待，不能把本地生成器通过记为准确发行包通过。
