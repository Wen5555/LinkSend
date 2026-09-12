# M6 独立收件箱页面与原生定位绑定

更新：2026-09-12。工作分支 `codex/desktop-inbox-ui`，基于已提交 `ec905b9` 加 M6 后端 `47110f8`。未拷贝主工作区未提交文件。

## 接线约定

- 新增 `apps/desktop/inbox_desktop.go`，薄封装 Inbox、InboxFiles、ResendInbox、ForgetInboxRecords、CleanupInboxStaging，并提供 `RevealInboxFile(taskID,fileID)`。前端不能传入路径或 shell 字符串。
- Windows 通过确定的 SystemRoot 下 `explorer.exe`，以独立 argv `/select,` 和已解析路径定位；macOS 使用固定 `/usr/bin/open`，独立 argv `-R` 和已解析路径。未使用 shell 拼接。只请求文件管理器定位，未执行接收文件。
- 页面 `frontend/src/pages/InboxPage.tsx` 的 Props 为 `devices/run/op/available`，可选 `focusTaskID/navigationRevision`。导航、App/main/hooks 和最终 generated bindings 由主任务统一集成。
- 查询键统一以 `['inbox']` 开头；工作区事件可适度 invalidate 此前缀，后端 epoch 改变时应清理旧查询缓存。navigationRevision 只用于新的导航动作，不应传每次进度事件的 revision。

## 页面行为

- 按设备、方向、状态、日期和文件名筛选；记录与文件清单独立分页，每页最多 25 项，切换筛选重置游标。没有虚构全库总条数。
- 后端不可用时禁用操作，活动/恢复/队列引用中的记录无法选中删除。记录 revision 更新或选择改变后，旧删除确认自动失效。
- 重新发送先展开确认；离线等待需主动勾选。只把不透明 task ID、request ID 和等待选择保存到 sessionStorage，确认丢失后重试沿用原 ID；后端确认之后的新动作才生成新 ID。标识持久化失败时不先发起新的发送。
- 删除历史与清理本机孤立暂存使用不同确认区。删除仅提交任务 ID 和期望 revision；清理展示实际 removed/protected/issues，绝不把保留项报成已清理。
- 文件定位只提交 task/file ID；已移动、删除、变化或旧记录无清单均有明确提示。通知的 focusTaskID 只打开文件面板，不自动打开文件管理器。
- 文件名按 React 普通文字渲染，图片和正文不会经过此页面或 IPC。

## 自动与浏览器验证

工具环境显式加载 `D:/apps/Osend/.tools/use-desktop-toolchain.ps1`（Go 1.27.1 / Node 24.21.0 / pnpm 12.4.1）。Wails beta.18 在独立 worktree 生成 52 个方法、34 个模型用于类型核对；生成的 bindings 不包含在本提交。

PASS：

```powershell
# desktop
go test ./... -run TestInboxReveal -count=1
$env:GOWORK = 'off'
go test ./...
go vet ./...
# frontend
pnpm run typecheck
pnpm run lint
pnpm exec vitest run src/pages/InboxPage.test.tsx
```

新前端测试 10 项 PASS：SSR 的不可用状态/分页/保护操作/恶意文件名/旧清单提示，以及重发持久身份、确认更新失败、日期和查询限额、真实结果文案。原生 argv 测试 2 项 PASS，包含 Unicode、空格、逗号、shell 样式字符；测试没有启动文件管理器或执行用户文件。

随后发现不应采用的依赖共享方式：该 worktree 的 node_modules 曾以 junction 指向主树，pnpm 12 的自动依赖维护重写了共享链接，导致主树工具入口异常。已立即停止自有 9267 fixture（PID 254508），核实后仅删除 worktree junction，未删除主树依赖或源码。主任务负责恢复主树依赖；此后禁止复用可写 node_modules 链接，验证使用独立安装/virtualStore。最后一次因工具入口不可用的重复 type/lint/test 命令未通过，不计入上述成功结果。

独立 Chrome CDP 测试页使用真实 React 页面和 **模拟后端**，12 项交互检查 PASS：分页、惰性文件清单、文件名转义、按 ID 定位和移动提示、重发前确认、不确定重发复用 ID、已确认后的新动作新 ID、取消删除、按 revision 删除、独立暂存确认、保护/问题项文案、搜索后游标重置。测试没有调用真实 Go 删除/队列操作，没有清理用户文件。

700px iframe 的实际布局：`innerWidth=700`，document/body `scrollWidth=685`，无控件越界。截图接口 `Page.captureScreenshot` 实际超时，未保存可用截图，不将 DOM 几何检查冒充截图或原生验收。

证据在独立 worktree `apps/desktop/frontend/.artifacts/inbox-ui-fixture/`：`browser-result.json`、`narrow-result.json`、实际 fixture 和检查脚本。首轮长 async CDP evaluate 超时，改为后台执行并单独读取完成结果后取得 12 项 PASS。

## 已知基线问题与剩余验收

完整 `pnpm test` 在此基线为 37 PASS / 2 FAIL：既有 `workspace-ui.test.tsx` 的两个 TransferPage SSR 用例没有 QueryClientProvider，而 M2 已在 TransferPage 增加 useQuery。新 Inbox 10 项全部 PASS；已反馈主任务统一修补旧测试，遵守范围未修改它。

仍待主任务：导航/事件/统一 bindings，M4/M5 集成后的真实列表数据和内容动作，Windows Explorer/macOS Finder 真实定位，以及最终 Wails 原生交互。浏览器测试与 argv 单测不能替代这些原生验收。
