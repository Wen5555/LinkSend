# R5 本地owner、两阶段数据面与桌面候选（2026-10-26）

本轮基于 `91cca8e`（专属签名协调已提交）的工作树；core已作为 `e53a806`、desktop/UI/workflow作为 `701e23b` 分别提交并推送，远端SHA已核对。不能将此前CI记到本候选；该提交的准确CI及后续测试修复见末节。唯一完成状态仍在[TODO](../DESKTOP-EXPERIENCE-TODO.md)。[ADR0017](../adr/0017-one-time-share-grant.md)先冻结授权与协议，当前实验开关仍为 `experimental_share_control=true`，`file_transfer_ready=false`，不广告正式 `share_grants_v1`。Goal工具已实际核对active，R5及R0–R6整体均未完成。

## 当前实现

`internal/app/share_direct.go` 使用真实Pion ICE、固定原双方公钥的互验TLS QUIC；sender固定发起、receiver响应，普通AuthorizationGeneration为0，不进入普通连接池或device registry，不增加永久trust、devices/groups、自动接收或剪贴板权。`ConnectShare`为新连接取得当前状态及签名WS证明；服务逐帧核验原grant/claim/task/generation/phase/manifest/plan，transfer入口、新文件流与接收计划再ShareAuthorize。helper中间阶段保持本机owner/current/cancel/blacklist/deadline门，已建立bound任务不因普通控制断线猜测撤权；明确安全终态通知有独立reader且不抢ICE reader。

`share_preview.go` 的严格 `LSSP01`（六字节ASCII＋四字节大端长度＋version1 JSON）仅在approved原scope和领取期限内交换真实Manifest。receiver明确选择目录、BuildReceivePlan、持久Bind意图并收到真实HTTP Bind成功后才回accepted/plan_digest，sender回同摘要confirmed；拒绝只回固定RECEIVE_REJECTED。清单帧最多 `transfer.MaxMetadata+2048`（当前8MiB＋2048），其他帧4096；重复/未知/Unicode大小写别名、多文档、错scope/task/digest及不完整计划均拒绝。接收完整计划与绝对目录留本机，预览不创建receiver storage、不发送文件body、不经信令传清单；成功用FlushTerminal收尾，不把成功FIN当中断。

Bind后preview退役，transfer用新ICE SessionID、新scope与新QUIC。`temporary_share_tasks.go` 复用 `SendWithOptions`/`ReceiveWithOptions`、BLAKE3、安全落盘/checkpoint、processFileBudget及TaskSnapshot/历史/进度。创建先真实PrepareSelected并冻结manifest；PrepareSelectedForResume重建原source并核对完整摘要，源变化停止owner且持久revoke意图，普通Retry不能绕过。绑定后的PlanChanged只能通知原同一plan，不能自动改名改变digest或目录。wire任务仍为Manifest.TransferID的128bit ID，本地TaskRecordID为既有历史ID；本机temporary_share_id将二者关联并使普通ResumeTask回到临时owner。

`share_ledger.go` 借同profile现有SQLite metadata的 `temporary-share/v1/<localID>`，不新增DB owner/schema迁移。设备Ed25519及固定用途域 `LinkSend temporary share owner v1\0` 签名绑定profile身份、server origin、revision/role/localID、原双方key、grant/claim/approval/task/generation、manifest/plan、source paths/selection、保存目录、控制intent和本机engine result；raw CAS写入后回读、严格签名/边界核验。单行最多2×MaxMetadata、总64条、sender活动8、receiver待处理4；换server设置不能重绑定旧origin。

明文码不进ledger/task/error/diagnostics，只在显式本机Create当次返回；丢码需撤销新建，重试原intent不承诺读回code。create/claim/approve/bind/revoke/complete响应丢失保留原稳定intent/ID；claim未知grant时重新输入同码沿用原ClaimID，不另建actor。重启仅回读ledger/任务，不联网、不批准、不自动Start/Resume；本机取消先持久意图并取消scope，sender revoke可重试，receiver取消不冒充sender撤销。

新建前最多扫描当前64行，只回收真实服务已确认Completed/Revoked/Expired且首次terminal_observed_at满七天、无active/controlBusy/pending intent的本profile签名metadata，DELETE也按raw CAS。重复读取不延长首次时间，CreatedAt不代替终态观察时间；旧v1无可选timestamp、未知/篡改、回拨及CAS替换均保护。仅删私有授权metadata，不删任务历史/checkpoint/source/正式接收或用户副本，不增手动清理或容量/期限设置。

## 两项受控失败与修复

旧beginProfileWork只登记worker，桌面runtime调用context仍活时，同步Create准备/hash及HTTP可能阻挡profile退出。`temporary_share_control.go` 的私有temporaryOperationContext以context.WithCancel(caller)并由context.AfterFunc(s.workCtx,cancel)关联退出；Create准备与execute/refresh共有HTTP均使用，释放时stop/cancel，不存context或锁内联网。私有beforePrepare仅作确定性测试屏障。旧实现准备、HTTP create/claim/refresh/retry五场景均FAIL；修后调用方context继续存活，profile退出实际取消并join，普通10轮通过，没有放宽deadline或用大文件延迟。

服务允许任一合法bound actor Complete，不能由服务Completed推断本机文件已验证落盘。旧实现对create/approve/bind幂等回复Completed会标本机completed；真实TLS/Ed25519/SQLite三个受控负例均出现 `state=completed, engine_receipt=false`。现只有已持久本机Completion才标本机完成；无该凭据则保留server grant Completed并置needs_attention，禁止Start/Resume/Authorize，不补造verified/committed或双向确认。UI显示“分享已结束，本机结果待核对”。

## 实际自动验证

主要日志根：`D:/apps/Osend/.artifacts/share-owner-draft/`。

- `owner-terminal-completion-before.log` 保留三类旧实现FAIL；`owner-terminal-completion-after-test.log` 为新负例及真实engine完成/完成响应丢失正例普通10轮，app18.844s、命令23256ms、exit0。
- `owner-terminal-completion-final-race.log` 为最终20项TemporaryShare顶层测试race3轮，app89.487s、命令106686ms、exit0；`owner-terminal-completion-final-vet.log` exit0。早期19项race不能代替这个最终范围。
- `owner-sync-shutdown-before.log` 保留退出五场景旧FAIL；`owner-sync-shutdown-after-test.log` 为普通10轮，app7.172s。最终20项race已覆盖退出回归。
- `owner-bound-restart-first.log` 为真实socket/V1引擎8MiB任务，4MiB已验证后暂停，双端退出/重开Service，再分别显式Resume；同原TaskRecord、grant/claim/generation/manifest/plan/目录，最终8MiB一致、无第二任务，5.00s。
- `owner-ledger-retirement-test.log` 覆盖自动终态回收、预算与CAS保护；`owner-engine-final-request-count.log` 记录同IP完整合法流程48次请求，其他完整样本48–50次。初版冗余proof实际RATE_LIMITED及开发期PlanChanged失败保留，未提高原全局burst60，也未换IP/延迟绕过。
- 四个helper回归 `TestShareDirectUsesPinnedQUICWithoutMembershipOrClipboard`、`TestSharePreviewBindsConfirmedPlanBeforeAnyFileStorage`、`TestShareDirectRequiresPositiveOwnerAuthorization`、`TestSharePreviewFramingRejectsAmbiguityAndUnboundedMetadata` 已由最终完整app矩阵实际运行通过；不虚构独立helper耗时。

最终源码回执 `owner-terminal-completion-source-hashes.json`：temporary_share_control.go SHA256 `ee0e144689e61483b685ae206c8a34ff7c027bbfa487b45df214254524e444af`；temporary_share_completion_test.go为 `f4894dba920bbfe25f4e913bfdf573b8ba588d7c2150d01569f1f6ce7446154d`。

完成门修复后的整树矩阵使用 `. D:/apps/Osend/.tools/use-desktop-toolchain.ps1`，分别在core与apps/desktop、workspace及GOWORK=off运行 `go test -count=1 ./...`、`go vet ./...`、`go build ./...`。准确日志根为 `.artifacts/crossnet-20261026/r5-final-owned-completion-155-{core|desktop}-{workspace|off}-{test|vet|build}.log`；12条均exit0，耗时毫秒：

| 模块/模式 | test | vet | build |
|---|---:|---:|---:|
| core workspace | 133863 | 1436 | 5105 |
| core GOWORK=off | 125610 | 1298 | 4710 |
| desktop workspace | 16532 | 1360 | 4119 |
| desktop GOWORK=off | 11166 | 965 | 4626 |

这份矩阵覆盖155项前端dist及本机Completion完成门修复，早于下节目录时间修复，不能移记后续源码通过。

## 目录准备阶段真实失败与修复

独立Windows driver首次小fixture在传输前真实失败：sender为preparing/SOURCE_CHANGED，receiver随后signaling_connect/DIRECT_FAILED，尚未发送文件。`.artifacts/share-crossnet-draft/stopped-fixture-readonly-comparison.json` 使用SQLite mode=ro读取原冻结manifest并重建原源，6条中只有一条directory.Modified增加521200ns；文件Size/Hash/Chunks及零字节条目保持，driver没有后续源写操作。另一个真实回归也见目录mtime约1ms延后更新，不能把后继receiver错误当独立跨网根因。

`prepareTemporaryShareSource` 继续完整遍历并哈希原源，仅在同ID、路径及类型均为directory时沿用原冻结Modified，再要求完整manifest摘要相同。目录增删/改名、文件大小/时间/内容/chunks变化仍拒绝；发出的正文wire仍是原冻结manifest，未改变transfer全局、V1或其他普通任务。实际Prepared.Revalidate仍检查所有文件与ReadChunk哈希，不把目录时间当文件内容证据。

`owner-directory-deterministic-before.log` 用确定的目录mtime-only变化复现旧实现FAIL，5个结构/文件负例仍PASS；`owner-directory-after-test.log` 为目录正例和5负例普通10轮app20.107s、exit0。`owner-directory-after-race.log` 的 `GOWORK=off go test -race -count=3 -run '^TestTemporaryShare(Directory|SourceChange|ExplicitConfirm|BoundRestart)' ./internal/app` exit0、app48.420s；没有记录命令总wall耗时，不补造毫秒。之后仅测试补齐6条精确路径/type断言，最终正例普通3轮app9.624s及app vet exit0（`owner-directory-final-{test,vet}.log`）；生产源码保持。新完整矩阵及准确CI随后单独核对。

修后新建的256KiB私有driver fixture双方输出PASS、应用正常退出，262175B、树hash一致、嵌套空目录1、TLS1.3/linksend/1、bilateral=true、relay=false、永久trust/剪贴板权限0及普通成员接口拒绝。Python wrapper因两端各两条Pion关闭UDP的stderr而exit1；必须分别记录driver结果与wrapper拒绝，不能称整个wrapper成功。原失败源/证据保留，详见 `.artifacts/share-crossnet-draft/after-fix-inspection.json`。这是Windows同机loopback，非准确干净提交或实网证据。

目录修复后 `r5-final-directory-155-{core|desktop}-{workspace|off}-{test|vet|build}.log` 的12命令均exit0：

| 模块/模式 | test ms | vet ms | build ms |
|---|---:|---:|---:|
| core workspace | 135385 | 1438 | 6342 |
| core GOWORK=off | 134782 | 1434 | 4709 |
| desktop workspace | 12689 | 1027 | 3857 |
| desktop GOWORK=off | 11544 | 733 | 3952 |

全部采用冻结的目录修复生产源码及同155项dist。首个core workspace test启动后，仅目录测试补齐精确路径/type断言；新增断言由最终目录普通3轮及后续core GOWORK=off完整测试验证，不把首个命令冒充该最后测试文本的运行。准确提交CI仍须逐项核对。

## 桌面、浏览器与原生边界

`apps/desktop/temporary_share.go`、原生复制适配、绑定及TemporarySharePanel/TransferPage已接当前owner DTO和显式操作。`.artifacts/share-ui-draft/state-frontend-{typecheck,lint,test,build}.log` 为准确最终来源：19files/155tests、3.34s，typecheck/lint/build通过；`final-*` 的145项和 `revision-*` 的146项是历史。dist为 `index-BDQxkLM-.js` 435.75kB（SHA256 `ba25ba3a0286a060748d65a7bc034f50ebc75e13228354225f5d5e6f9ff57b19`）及 `index-D-YRzknO.css` 50.49kB。

`.artifacts/share-ui-visual-draft/QA-RECEIPT.md` 记录实际TransferPage/TemporarySharePanel与项目CSS，独立Vite 127.0.0.1:41783、专用Playwright会话、Backend仅假metadata。960×640/1100×720已检查：零设备但有source时普通Send禁用、临时Create可用；打开/刷新/分页零自动Create/Approve/Start/Resume；sender显示完整64字符fingerprint，明确批准精确localID/claim/key/gen后才Start一次；receiver picker取消不Bind，长中文目录确认才Save；11分享每页5项、25清单每页10项、长文本无水平溢出；私有错误详情不入DOM、claim input清空。ready+pending标签初错已修，服务终态无本机receipt准确显示待核对且无继续按钮。RATE_LIMITED仍为generic提示，未扩为已解决项。

截图使用 `output/playwright/960-save-selected.png`、`1100-save-selected.png`、`1100-manifest-page2.png`、`960-source-error-visible.png`、`960-unverified-terminal-visible.png`；高surface整图有inner scroller裁剪，不能声称显示全部内容。`960-pending-confirm-viewport.png` 是修前，后续YAML/terminal及source-hashes-start/end记录修后范围。浏览器已CLI关闭，精确核对fixture命令行后停止Vite；`output/playwright/cleanup-final.json` 为browser_closed_by_cli=true、fixture_listener_remaining=0，未停无关Go或用户浏览器，原根 `.playwright-cli/` 未触碰。上述均不证明Wails RPC、native picker/clipboard、文件IO或P2P实机。

`native-clipboard-test-01.jsonl` 的 `TestDesktopTemporaryCodeNativeClipboardDoesNotSyncOrCache` 实际SKIP：原剪贴板含未证实可备份格式，未mutation。包级exit0/PASS不替代该目标通过。私有WindowStation尝试在 `.artifacts/share-ui-draft/private-station-02/final-receipt.json` 为 `NOT_RUN_API_ACCESS_DENIED`，CreateWindowStationW/Win32=5；未创建test child、native测试未跑、无默认station回退、parent station与sequence958→958保持。helper退出1并恢复原station。此为OS API权限拒绝，不是产品native失败或自动审批拒绝。测试exe SHA256 `90e23a01195923b2a5b04fcf6255ad7047d6c6ed488cdadbb1aa4132602fbf52`。

workflow已准备在Windows、GitHub Actions且runner context及环境均为github-hosted时才建立空剪贴板fixture，self-hosted/未知runner明确拒绝。设置 `LINKSEND_TEMP_SHARE_NATIVE_CLIPBOARD=1`，目标go test -json必须run=1/pass=1、skip=0/fail=0、exit0，并上传JSON/stderr/summary；fixture创建失败不能skip当通过。记录时准确新提交尚未推送/跑此检查，不能记GitHub native PASS。

## 未完成与后续

不同网络Go核心双向临时分享已补充[701准确实网证据](RELIABILITY-R5-CROSS-NETWORK-20261026.md)；准确候选CI/包来源、Windows↔Mac与原生保存/复制及最终完整旅程仍待真实证据。真实socket自动引擎、浏览器夹具、CI构建与物理跨网分别记录；不抵扣用户Windows双端公网连接现场、受限双NAT、R2完整缓存/键盘粘贴或其他R0–R6必做项。未做生产部署或宿主防火墙/路由/代理变更。

## 701e23b 准确CI失败与测试工具修复

`701e23b067c38747813b84fa9c93cee6d19e0901` 的[core40232117991](https://github.com/Wen5555/LinkSend/actions/runs/40232117991)及[desktop40232117989](https://github.com/Wen5555/LinkSend/actions/runs/40232117989)均completed/success；[packages40232117997](https://github.com/Wen5555/LinkSend/actions/runs/40232117997)为failure，两个Mac包job成功，Windows的native复制检查失败。证据artifact12614000098的三小日志已下载到 `.artifacts/share-ui-draft/ci-701e23b/`，summary为target_run=1/pass=0/skip=0、exit1；不能将目标运行等同通过。

真实原因是测试扫描运行中的profile时尝试读取Windows独占字节锁定的零字节根 `.profile.lock`，返回ERROR_LOCK_VIOLATION。失败发生在第一轮保护码和后续正常文字QUIC断言之后，paused整轮尚未验完，不标原生PASS。修复仅在Windows测试scanner中排除精确profile根、regular且Size0的该控制文件；其他同名嵌套文件、其他锁文件、非空/非regular控制锁、伪码及任何其他读取错误继续拒绝/传播，不改变生产代码、权限或原生备份门。

纯文件测试使用真实core.New持有该Windows锁，旧实现实际FAIL（`native-profile-scan-before.log`，exit1/8968ms）；修后正负例普通10轮PASS（app5.453s、命令9089ms、exit0，`native-profile-scan-after.log`）。optin显式unset的desktop workspace test/vet/build为9080/928/3924ms，off为9562/749/3772ms，六命令exit0；日志 `native-scan-desktop-{workspace|off}-{test|vet|build}.log`。这些测试未访问系统剪贴板，完整native正向仍须新提交托管CI证明。

`103ba61c13ba02a02a0037bd891c2a64777cec29` 的core40233307686与desktop40233307731均success，packages40233307711仍failure。Windows native目标run=1/pass=0/skip=0、exit1，报告real clipboard session did not become ready；同一等待closure用于初始和暂停恢复，本日志不能判定阶段，不能移记锁修复后的完整native通过。证据在 `.artifacts/share-ui-draft/ci-103ba61/`。Mac arm64在安装固定Wails beta.18时，sum.golang.org的IPv6无路由导致Go模块校验失败，未到产品测试；Intel包成功。保留两类失败，继续确定性回归，不以提高等待或重跑掩盖。

## 原生夹具就绪门的因果修复

新纯内存adapter、真实Core/Pion/QUIC和桌面copy/deliver桥接回归在原三秒门下真实FAIL：initial_leases为10ms就绪，紧接本机pause、保护码copy和resume，3005ms时A SendReady=false/ReceiveReady=true、B双向ready、无waiting/error且receiver仍online/listening（`native-readiness-before.log`，exit1/12708ms）。Reset清本机双向lease，本机wake只重发自己签发的receive lease；健康Ensure不重拨，对端仍按既有七秒周期发送原对端授权。这证明夹具的三秒双向门不足，不证明103未分阶段的那次CI一定发生于resume。

夹具现先等真实receiver SignalingConnected/Listening，再Ensure；initial_leases与resumed_leases分阶段记录安全boolean状态，并在原15秒父context下等真实租约。没有延长父期限、强迫对端Reset、改权限、改生产周期或制造远端证明。受控HTTP屏障还证明StartInbox已经返回但注册尚未完成，放行后实际就绪。恢复实测约6.97–6.99秒，WSS连接计数未增加；分享码generation不读/不转发，随后新文字实际经认证QUIC收取。

最终纯回归普通三轮app27.214s/命令31538ms、race三轮app30.249s/命令42913ms均exit0（`native-readiness-final-{test,race}.log`）；desktop workspace test/vet/build为15861/1065/3882ms，off为17465/832/3939ms，六条exit0（`readiness-desktop-{workspace|off}-{test|vet|build}.log`）。只修改两份Windows测试文件，optin显式unset、未调用本机系统clipboard API；准确新提交的托管native目标仍须实际run/pass/no-skip证据，不用本纯回归移记通过。
