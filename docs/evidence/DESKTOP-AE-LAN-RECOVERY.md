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

## 阶段推送与 CI

首批源码提交 `d5412642e49460413fdda249f77d03647075f41c` 已推送正确分支，本地和远端 SHA 一致，沿用 [Draft PR #9](https://github.com/Wen5555/LinkSend/pull/9)。该提交五项 workflow 均 success：core push/PR `39060667110` / `39060670788`；desktop push/PR `39060667283` / `39060670904`；[三平台包构建39060667017](https://github.com/Wen5555/LinkSend/actions/runs/39060667017)。Windows amd64、Mac amd64/arm64 job 均通过；构建通过仍不等于物理包验收。

## 修复前双机发现基线（10轮，harness 边界）

从 `638b2aa` 的 `git archive` 导出原生产源码，加入同一 `scripts/lanprobe/main.go` 控制面测量器后分别交叉构建 Windows amd64、Darwin arm64。测量器不接信令/STUN，不授予文件或剪贴板权限，不传正文，不使用手动 IP 探测、记忆地址或注入候选；双方 expected identity 只经 SSH 交换已公开的 ID，以防串测。它调用原 `discovery.Manager`，不是原生桌面包。

测量器 SHA256 `31417502a11d468b2f99cc31a662c82ef69b038bad22d37627e4691fe410da7e`；Windows probe SHA256 `bbbb99a53d7c255034df881fecdbb5d48881fbc68fe587e95eff7859bff637f4`，Mac probe `93c5f6f2803cafc76dcfa97a0d4614195818a046a7420263ed74eedee7a9a09e`。原生产源码与同一测量器的构建/哈希分别可复核；不得将附加测量器叫作原提交已有程序。

专用进程每轮12秒，Mac/Windows各先启动5轮，顺序交替，10轮均为新内存identity。`ready_to_seen_ms` 从**本进程 Manager 就绪**计至首次发现，每20ms轮询；它包含先启动一端等待后启动一端的自动编排时间，不是双端ready后的纯网络耗时。刷新指标只证明调用Refresh后观察到更新公告，不将它伪称有请求关联的ACK。

| 指标 | Windows | Mac |
|---|---|---|
| 自动发现 | 10/10 | 10/10 |
| 本进程ready至发现，中位数/范围 | 226ms / 106–444ms | 141ms / 21–1061ms |
| 刷新后观察到新公告 | 10/10 | 10/10 |
| 向对端发起固定身份TLS控制 | 10/10 | 0/10 |

Mac 十轮均在 TCP dial 阶段以 `LAN_CONTROL_UNREACHABLE` / `no route to host`（0–1ms）失败。包装批次exit0仅表示十轮均完成收集，不能改写子probe的失败。最早pilot01缺少细粒度错误且失败后提前退出，保留但不计入n=10。

在同一正在运行的 Windows owned listener 上，Mac系统 `/usr/bin/nc` 普通连接和显式绑定 `192.168.1.243` 均成功；Mac route/ARP明确走en0，Application Firewall状态disabled。未改任何网络/安全配置。这排除了“所有进程都无法连接该端口”的前提，尚不能证明具体拒绝原因。probe为linker ad-hoc签名、`Identifier=a.out`、无绑定Info.plist；TCC日志显示SSH responsible attribution。Apple DTS 有[类似后台工具Local Network行为的说明](https://developer.apple.com/forums/thread/778457)，仅作待验证假设；不能据此判定当前原生LinkSend包也失败，或自行改安全策略。现有桌面plist已经有NSLocalNetworkUsageDescription，下一步应在准确原生包上下文核对。

本机原始jsonl/摘要：`.artifacts/ae-20261018/baseline-{02..11}-{win,mac}.jsonl`、`baseline-batch-results.json`、`baseline-summary.json`。Mac原始日志：`~/.codex/supervision/linksend-ae-20261018/baseline-{02..11}.{jsonl,err}`。管理job：`/tmp/codex-ssh/job-linksend-ae-lan-baseline-batch-20261018-01`（完成）；反向对照：`.../job-linksend-ae-mac-reverse-20261018-01`，签名/TCC：`.../job-linksend-ae-mac-lnp-20261018-01`。所有probe已按有界期限退出，未启动剪贴板，未产生测试文件内容。

因此，上节“真实自动发现NOT RUN”已由本节限定为**原生桌面包完整旅程仍NOT RUN**；harness的生产发现模块已取得修复前实机基线，配对/文件、优化后对照及最终准确包仍待继续。

## 刷新合并、provider可见与早期host候选（2026-10-18）

重复刷新共用单个在途调用与同一结果（包括失败），完成后750ms内不重复枚举/公告；下次正常刷新仍重试失败provider，Close/cancel后拒绝。周期公告与记忆地址退避均±12.5%有界抖动，不改变端口恢复或单源读写所有权。设备页进入/后端重新就绪主动发现，目录数据更新不额外触发；provider状态进入Inbox与可导出诊断，页面说明临时端口/组播等限制、手动IP与配对边界。组播/广播发送分别记录，任一失败不取消另一provider。

真实Pion Agent、共享UDP Endpoint和签名候选管道，配合绑定但不回应的STUN UDP sink，复现连接已nominate host↔host后仍等3.0012372s。原判断只对`lan_direct`早停；loopback和其他证据为`direct_unknown`的真实host pair仍等待。改为根据已选pair两端均host及时结束trickle，保留真实路由分类、签名/代次与QUIC pin边界。修复后race三轮ready分别12.4926、8.9137、10.0052ms；STUN sink实际被探测且不回复，无TURN/正文中继。这里只证明局部时序，无真实LAN吞吐收益或NAT成功主张。

本地日志 `trickle-host-before.log`、`trickle-host-after.log`、`refresh-verified-race.log` 保留修复前后。新增测试覆盖65个并发刷新只派发一次、失败限频后重试、在途等待取消、抖动范围、provider到Inbox/诊断与页面恢复隐藏、设备页重渲染不重复发现。已有端口冲突/释放、reader/close测试继续通过。新增诊断断言曾暴露启动覆盖具体错误，已保留具体状态；旧FAIL日志保留。首次desktop与前端build同时运行造成旧embed资产暂缺，已改为dist稳定后复验，不把编排失败算产品失败或通过。

前端准确Node24.21.0/pnpm12.4.1：92tests/typecheck/lint/build通过；真实Wails beta.18绑定重新生成。核心off完整test（app67.027s）、vet/build通过；desktop workspace test、off-race/vet/build和最终dist稳定后的off build通过。发现完整race三轮通过（18.914s）；最终发现刷新/应用定向race三轮通过（5.333/10.530s）。最新日志为`refresh-verified-{core,race,desktop,frontend-*}.log`，不使用之前FAIL日志替代终态。未把源码候选替代准确包原生或真实双机。owned持久服务/监听未新增，普通夹具结束关闭自身socket；外部STUN授权与Mac公网/反控限制保持。
