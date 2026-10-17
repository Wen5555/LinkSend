# 2026-10-18 A 首批 LAN 恢复证据

起点 `638b2aacc87d3f3c3b093a3bf9ffc68fcfbd0e96`；Go `1.27.1 windows/amd64`。本页只保存实际命令和证据，当前完成状态统一维护在 [TODO](../DESKTOP-EXPERIENCE-TODO.md)。

## 恢复来源与修复前失败

逐文件核对 `.artifacts/astra-stop-20260923/PRESERVED-WORK.json` 的九个 SHA256，均与 `candidate-source` 及原 `C:/Users/Wen/.codex/worktrees/208a/Osend` 相同；当前这些文件相对 `8fcc76d` 无已有源码差异。未使用换行损坏 patch。

- 原版本 `go test -count=1 ./internal/discovery ./internal/app`：PASS，3.391s / 93.991s。
- 新增临时公共入口回归 `go test -count=1 -run '^TestOccupiedDiscoveryPortDoesNotDisableLAN$' ./internal/discovery`：修复前 FAIL，`LAN multicast listen: ... bind: Only one usage ...`。该临时测试随后由永久 `TestDiscoveryFixedPortConflictKeepsTLSAndRestores` 及相关完整回归替代，没有删掉失败要求。
- 恢复候选后，`go test -count=1 -run '^TestDiscoveryLateReaderFailureCannotRestoreDegradedStatus$' ./internal/discovery`：FAIL，`accepted=true status=map[receive:socket_read_degraded]`。收据称最后检查待做，实际候选仍忽略 reader 参数，因此本轮补齐同锁比较/状态写入，并拒绝旧 reader 成功读取后的过期处理。

## 已验证源码行为

固定端口占用可临时监听，TLS 控制端口保持；刷新失败保留健康临时 socket，释放后手动刷新与既有 ticker 均恢复固定端口。旧 reader 不能重建/降级替代 socket；Close 取消、释放响应 workers，关闭后拒绝新发送/重建。定向探测尝试所有匹配源，最多32条；配对/恢复成功立即记忆到 live manager，拒绝/身份不匹配不消耗记忆容量，provider 已满不破坏持久授权。错误 pin、签名篡改、重放与未同意仍拒绝。

| 实际命令 | 结果 |
|---|---|
| `go test -count=1 ./internal/discovery` | PASS / 5.893s |
| `go test -race -count=1 -timeout 3m ./internal/discovery` | PASS / 7.140s |
| `go test -count=1 -timeout 3m -run 'Test(RememberLAN\|LANPair\|LANConfiguration\|NetworkChange)' ./internal/app` | PASS / 5.163s |
| `go test -race -count=1 -timeout 3m -run 'Test(RememberLAN\|LANPair\|LANConfiguration\|NetworkChange)' ./internal/app` | PASS / 5.695s |
| `go test -race -count=5 -timeout 2m -run 'TestDiscovery(LateReader\|CloseRejects)\|TestRebuildUDPSocket' ./internal/discovery` | PASS / 2.735s |
| `$env:GOWORK='off'; go test -count=1 -timeout 5m ./...`（核心） | PASS，app 52.666s，discovery 6.107s，其他包通过 |
| `go vet ./...; go build ./...`（核心 workspace） | PASS |
| `$env:GOWORK='off'; go test -count=1 -timeout 4m ./...; go build ./...`（desktop） | PASS，desktop 5.176s，nativeclipboard 0.896s |
| `$env:GOWORK='off'; go vet ./...`（desktop） | PASS |
| `gofmt` 与 `git diff --check` | PASS |

## 主机与保留资源

Mac `192.168.1.243` / macOS26.5 arm64：en0 `/24`，另有 `utun1024`，未运行 LinkSend。Windows 以太网 `192.168.1.122/24`。日本猫猫机 Debian13.7、根分区31GiB可用，现有 Nginx 监听80/443，已有多个 wildcard 站点及 Origin 证书，未发现 LinkSend/coturn 服务。本地 DNS 返回 `198.18.2.191`，不能用它推断真实 DNS 记录/服务来源；待 Cloudflare 精确记录核对。

备份并只更新 Mac 与新机两个 SSH 条目；Mac 保留既有 `HostKeyAlias=10.234.35.5`，两者严格校验。备份及本机收据：`.artifacts/ae-20261018/ssh-update-receipt.json`；备份位于原配置旁，后缀 `.linksend-ae-20261018T025843049.bak`。没有复制密钥。

已完成的可恢复远端只读 job：

- Mac：`/tmp/codex-ssh/job-linksend-ae-inspect-mac-20261018-01`，exit0。
- 日本猫猫机：`/tmp/codex-ssh/job-linksend-ae-inspect-jp-20261018-01`，exit0。

本批未启动产品/剪贴板/fixture 监听，无 owned 产品进程需停止。用户另授权自动允许本任务的 Windows 公用网络弹窗，当前无该弹窗，未改规则；不扩大为其他宿主网络变更。

## 尚未覆盖

真实 Windows↔Mac 自动发现/明确同意/双向文件、同条件性能、准确包与双 NAT：NOT RUN。源码回归不是上述物理/原生结论。A 后续仍需刷新合并/抖动、设备页触发、外部服务不可达独立旅程及实际测量；B/B2/C/D/E 和 N1–N4 按唯一 TODO 继续。
