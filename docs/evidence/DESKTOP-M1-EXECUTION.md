# M1 设备、持久草稿与队列执行记录

日期：2026-09-12。源码阶段：M0/M1 合并候选，产品0.5.0，协议V1。
用户明确选择 Go1.27.1 与最低 macOS13；M0 不单独发布。

## 实现

- 任务历史库 schema2→3，增加设备偏好、草稿与队列表；迁移前执行可读性、完整性与fsync验证的备份。
  设备别名、固定及顺序、“我的设备”、专属目录和最近完成交互保存在同一数据库。
  身份pin、屏蔽和免确认仍由trust schema1唯一所有者管理，设备显示字段不授权。
- 草稿由Go持久化，默认`main`，相对路径按原WorkingDir解析；更换目标保留文件、取消选择不清空，
  CAS拒绝旧表单覆盖新编辑。元数据路径允许经绑定，文件正文不经过JavaScript。
- Go单实例scheduler：幂等入队、事务调序/取消、全局一个活动传输、离线旁路、最多1000项待处理、
  7天默认/最多30天到期、按程序复用的presence检查和带抖动等待。等待不创建每项WSS/ICE/QUIC。
- 入队前通过真实Prepare取得源摘要；派发重新准备并比较，源变更先进入needs_attention。
  队列与逻辑task的持久关联发生在传输goroutine启动前，重复请求不重复派发；数据库失败不报告加入。
  重启时所有积压/运行项变为需要明确确认；有可恢复task时沿用逻辑task，否则明确重新准备产生新任务。
- 后端元数据事件只发送epoch/revision失效通知，桌面每200ms合并；Query取真实快照，旧epoch/revision不能覆盖。
  窗口恢复、事件丢失与重连会重新查询。命令不自动重试；入队保持未确认请求ID。
- App.tsx已拆成页面、组件与hooks，移除全局any/lint豁免。发送期间可以加入下一项；入队哈希进行时
  现有任务暂停、取消和接收确认仍可独立操作。界面显示设备权限、队列状态与真实任务字节。

当前源没有把S02系统入口、S03托盘、S04内容类型、S05选收和完整S06收件箱当作已完成。
这些由独立分支推进后合并，见总执行记录。

## 已执行验证

以下均使用最终选择的Go1.27.1。每一原生命令失败即停止该检查链，不用最后一条成功掩盖失败。

| 命令 / 场景 | 结果 |
|---|---|
| 根 `GOWORK=off go test ./...`、`go vet ./...`、`go build ./...` | exit0 |
| 根 workspace `go test -race ./...` | exit0 |
| 桌面 `GOWORK=off go mod verify/test/vet` | exit0 |
| `wails3 generate bindings -ts -i -clean=true` | exit0；43methods/23models/1event，无warnings |
| 前端 frozen install、typecheck含独立bindings、lint、Vitest | exit0；3files/29tests |
| 前端 `pnpm run build` | exit0；Vite8/Safari16/Chrome109 |
| 8并发相同入队、不同payload冲突、持久恢复确认 | 普通/race PASS；只有一个队列ID，离线等待无task/正文连接 |
| 三设备fixture，第一目标离线、第二目标在线 | 真实TLS/QUIC PASS；在线项完成且内容一致，离线项继续等待 |
| 入队后源变化→停止→显式重新准备继续 | 真实QUIC PASS；未确认时目标无文件，确认后内容一致 |
| 专属目录在AwaitingAcceptance后修改 | 真实QUIC PASS；既有任务写原目录，未被改向新目录 |
| 别名/固定不授予信任，专属目录失效不回退 | 普通/race PASS |
| 只读队列库写失败 | PASS；无加入成功、无队列行、无派发 |
| 设备/草稿CAS、旧schema备份、失败事务/未来schema | 存储普通/race与vet PASS；详见存储证据 |
| cache旧响应/事件先到/丢失epoch事件、ID丢响应、不同命令并发 | Vitest PASS |

网络场景使用临时loopback rendezvous和真实Pion/quic-go，不算物理LAN/跨NAT验收。
普通浏览器冷加载和700px DOM检查仅证明界面布局，不替代Wails原生控件。
Windows原生UI、最终提交CI资产及合并发布结果将另行追加。

## 回滚与阶段边界

旧版本最大支持task schema2，不能打开schema3。关闭所有进程后保留当前完整profile，在独立目录
恢复`task-history.sqlite.schema-v2-<UTC>.bak`并配套审阅trust迁移前备份；不得覆盖用户当前文件或静默复活已屏蔽身份。
队列历史清理、任意规模历史分页及内容文件生命周期在M6完善；当前活动队列优先返回，最多1050项，
不会因前1000项已经完成而挡住新待发项。
