# M4 传输层接收计划证据

日期：2026-09-12。独立工作树：`.artifacts/desktop-six-features/m4-worktree`；分支 `codex/desktop-receive-plan`，起点 `b3d5fc7`。验证在该基线加本轮未提交 M4 改动上进行；本文件随代码提交后，由父任务取得实际提交 SHA 并整合。没有修改主工作树、推送、部署或发布。

范围：`internal/transfer`、必要的协议错误/NoContent状态、[ADR0005](../adr/0005-receive-plans.md) 与协议/安全/测试说明。原始 Manifest/TransferID/digest、V1、身份验证、Pion ICE、QUIC 数据面和产品版本均保留。App/UI接线属于后续阶段，不把这份传输层结果称为S05原生验收完成。

## 实现与兼容

- 新 ReceiveOptions/ReceivePlan/PlanRequest/Offer.ResumePlan/PlanChanged API 已运行；nil选择全量、空数组全部跳过、目录递归及祖先闭包明确。
- 原offer可选能力 → accept选择摘要 → 完整终态选择确认；旧accept全量、旧offer拒绝子集；不增加前置hello。
- checkpoint保留原manifest及计划摘要、请求范围、选中ID、目标相对映射；绝对目录只留本机Go字段。原旧checkpoint仍能恢复，新计划checkpoint使旧binary明确拒绝。
- 文件no-replace、commit intent和真实SameFile恢复；目录使用真正OS identity记录，不用CommitStarted或同hash推断归属。
- 明确Selected/Skipped/OriginalTotal计数、NoContent、已选源再验证、持久映射优先、相同skip请求不扩大选择、每次提交重查portable目标冲突。

## 实际验证

Windows PowerShell7；工具入口为主项目ignored `.tools/use-desktop-toolchain.ps1`，Go1.27.1，Windows race使用实际C工具链。下面多命令均逐条检查退出码，失败没有被后续成功掩盖。

| 实际命令 | 退出码 | 结果 |
|---|---:|---|
| `gofmt -w internal/transfer/*.go`（实际逐个列本轮文件） | 0 | 格式化完成 |
| 根 `GOWORK=off go mod verify` | 0 | all modules verified |
| 根 `GOWORK=off go test ./...` | 0 | 全部根包与集成测试通过 |
| 根 `GOWORK=auto go test ./...` | 0 | 工作区解析路径下全部根包通过 |
| `GOWORK=off go test ./internal/transfer ./internal/transport ./internal/protocol -count=1` | 0 | 既有与新增传输/QUIC/协议用例通过 |
| 同三个包 `go test -race ... -count=1` | 0 | 定向race通过 |
| 首轮完整 `GOWORK=off go test -race ./...` | 1 | 目录替换负例的错误类型随map遍历顺序变化：先看到缺失子目录时返回系统NotExist，而非稳定FILE_CONFLICT；没有放宽数据保护 |
| 修正后 `go test -race ./internal/transfer -run TestReceivePlanDirectoryIdentityRejectsReplacement -count=20` | 0 | 20轮通过，缺失已记录目录统一保留cause并分类FILE_CONFLICT |
| 修正后完整 `GOWORK=off go test -race ./...` | 0 | 全部根包race通过 |
| 根 `GOWORK=off go vet ./...` / `go build ./...` | 均0 | 通过 |
| 桌面独立 `GOWORK=off go mod verify/test/vet/build ./...`（verify不带./...） | 均0 | 独立桌面module通过；未生成绑定或执行Wails production build |
| `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -c ... ./internal/transfer` | 0 | Darwin目录identity helper及传输测试编译通过；未运行 |
| `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c ... ./internal/transfer` | 0 | Linux目录identity helper及传输测试编译通过；未运行 |
| `git diff --check` | 0 | 通过 |

交叉编译产物在本工作树 ignored `.artifacts/m4/`，只用于编译验证，不是可发布桌面包；交叉编译进程的CGO=0未用于Windows race。

新增26个transfer顶层测试（含2个真实QUIC顶层测试、多组子用例）以及protocol NoContent终态测试。既有error-code roundtrip补充所有计划/恢复稳定代码，并验证私有路径不进入peer error。`plan_review_test.go`保留独立审查发现的四项故障回归：

1. 通用CommitStarted不证明mkdir已运行；已持久mkdir意图但无目录identity证据时明确冲突，不合并foreign目录、不另取名字重复创建；已记录目录被替换也拒绝。
2. 同一ConflictSkip请求重启后复用过滤后的集合；被跳过文件后来删除也不自动选回来。
3. 未选源文件删除、全部跳过后源全部删除，不阻止已接受结果完成；新能力只重验选中ID。
4. 首个文件commit之后外部创建下一项NFD等价名称，下一次提交刷新目录并改用有界保留两份路径。

其他用例实际覆盖：部分/全部skip字节、空目录、Unicode/NFC与保留名、计划损坏、旧frame固定结构双向兼容、unsupported旧offer、未选chunk请求、错误terminal选择摘要、初始/晚更名PlanChanged失败无后续提交、partial commit重启不换名/重复计数、hardlink已成功但checkpoint未写入的实际文件系统窗口、foreign同hash不能冒充own commit、最终目录选择不上线。

真实QUIC测试使用两个真实loopback UDP socket、双方新生成Ed25519身份及生产TLSConfig pin校验，实际TLS1.3；接收端所选文件SHA/BLAKE3内容与源一致，未选文件未创建，NoContent两端0B且一致。阻塞接收端读取confirmed时，sender不能提前返回成功，解除后匹配confirmed_ack才完成。这是本机网络证据，未称LAN/双NAT。

## 剩余接线与环境边界

父任务需接入接收清单/目录选择/冲突UI、保存PlanChanged中的最终Directory和Plan摘要、恢复时展示Offer.ResumePlan、NoContent终态和新错误文案、真正空间查询API。RemainingBytes只代表重新校验后缺失正文；CommitBytes=0不包含目录/inode和恢复staging复制峰值，不能作为ENOSPC保证。

Windows/macOS真实原生接收决策、物理双机LAN/跨NAT、网络切换、现场强杀和新发布包均未由本分工运行。已有NAT失败不改为PASS。mkdir和目录identity记录之间的模糊崩溃窗口保留明确需处理结果，不声称能在无证据时自动确认目录归属。
