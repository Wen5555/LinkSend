# 2026-10-18 客户端灾后恢复与授权隔离

状态以[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)为准。本节首先交付现有成员切换中的授权屏障修复；显式恢复intent/API/UI、旧上下文退休和旧pending revoke发送前隔离仍待实施，不以本节代替完整B退出条件。

## 旧组与独立LAN重叠的修复前失败

旧实现把跨组缺席peer从 `group+lan` 降为LAN时保留原GrantGeneration/GrantedAt，因此service的前后代次比较看不到变化：旧活动任务未取消，队列仍可能借保留LAN关系确认/派发。成员响应重建peer时还会丢LAN request ID；跨组/本机incarnation改变会丢LAN地址/历史，GrantedAt归零。

新增实际回归命令：

```powershell
go test -count=1 ./internal/identity ./internal/app -run 'TestMembershipTransition'
```

修复前exit1：identity四种场景均暴露屏障或LAN证据丢失；service场景在等待1秒后明确报告旧活动任务未取消。原始日志 `.artifacts/ae-20261018/membership-transition-red.log` 留在本地，不提交随机identity/完整载荷。

## 修复行为与通过结果

- 旧group+lan降为LAN、组或本机incarnation改变均提升授权代次并更新GrantedAt，关闭免确认；代次耗尽原子拒绝，原文件保持。沿用service的现有代次比较来清剪贴板grant/关session/取消旧任务，不新增旁路权限。
- 有效独立LAN的地址、最多8条历史及request ID保留；普通同上下文刷新维持原代次和同意；无关LAN-only peer保持。
- 持久队列保持原authorization_generation，旧任务/旧队列确认被拒；profile重启后仍拒旧队列，新的显式LAN入队取得当前代次。identity文件与queue使用真实临时磁盘，传输未启动，不用该fixture冒充实际双机文件测试。
- wire与trust schema2保持，SPEC/SECURITY/ADR0007同步契约；本批没有客户端重置按钮、自动401恢复、旧快照退休或撤销请求跨上下文的新承诺。

修复后相同定向命令exit0；新增代次耗尽/磁盘原子性负例和profile重启确认也通过。进一步命令及结果：

| 验证 | 实际命令 | 结果 |
|---|---|---|
| 核心完整 | `GOWORK=off go test -count=1 -timeout 5m ./...` | PASS；app59.304s |
| 核心静态/构建 | `GOWORK=off go vet ./...`、`go build ./...` | PASS |
| 授权与应用并发 | `go test -race -count=1 -timeout 5m ./internal/identity ./internal/app` | PASS；app122.937s |
| 桌面两个模式 | desktop内workspace及`GOWORK=off go test -count=1 ./...` | PASS |
| 桌面静态/构建 | desktop内`GOWORK=off go vet ./...`、`go build ./...` | PASS |

前端本批无变更，沿用紧邻STUN阶段准确Node24.21.0/pnpm12.4.1的86tests/typecheck/lint/build结果，不重复称为本批原生验收。最终准确包与真实网络测试仍待完整功能集成。
