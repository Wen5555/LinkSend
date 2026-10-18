# 同一局域网自动剪贴板连接修复

日期：2026-10-19。基线 `3aa51486340ef2ad3eda30c063701ffd3199f2b3`，当前分支 `codex/github-desktop-updates`。用户反馈两端同一局域网，设置仍显示“连接失败”。保留既有GitHub更新功能及全部其他工作树改动，本次未提交、推送或发布。

## 原因与修改

1. `receiveLANSession`在认证之后直接等待文件的双向QUIC流，没有像WSS inbox那样把支持复用的会话交给pool。因此纯剪贴板会话不能启动单向流接收与lease交换。现立即采用既有pool，启动两类流owner；LAN接收控制TLS通道独占归属也明确关闭，旧端不支持复用时仍走原文件路径。
2. 桌面与核心剪贴板连接各有一层10秒截止时间，早于桌面配置的30秒分阶段超时。移除两个额外截断，保留DirectConfig分阶段超时、应用/workCtx关闭取消和Ensure singleflight。
3. `connection_failed`/`connection_lost`与`connecting`/`waiting_lease`可能在重连及lease已就绪之后残留。新尝试和当前会话就绪会更新对应连接状态；保留`write_failed`等独立剪贴板I/O错误；release不再以“等待授权”覆盖已经就绪的lease。

文件和剪贴板协议、数据库schema、身份固定校验、权限代次与默认开关均未改变。正文继续通过已认证QUIC，不经JS、信令或中继。

## 失败复现

- 新增`TestClipboardLANBootstrapsWithoutFile`，通过显式loopback发现路由进入LAN TLS控制接口，使用真实Pion ICE和QUIC，拒绝悄悄退回WSS。单独还原旧LAN接收owner后，3.25秒内实际FAIL：`LAN clipboard did not become ready without a file transfer`。该场景没有预置旧错误，失败与状态残留独立。日志：`.artifacts/clipboard-fix-20261019/lan-owner-before.log`。
- 新增`TestClipboardConnectionUsesDirectPhaseBudget`：连接准备受控延迟10.1秒，仍在15秒测试阶段预算内；旧剪贴板截止时间导致实际FAIL `SIGNALING_TIMEOUT: signaling read timed out`；修复后真实连接和双向lease就绪。日志：`timeout-before.log`、`fix-targeted.log`。
- 最初带旧错误的LAN回归失败另保留在`before.log`；复原旧owner仅用于验证回归有效，随后恢复最终修复源码。

## 验证

工具使用仓库`.tools/use-desktop-toolchain.ps1`：Go1.27.1、Node24.21.0、pnpm12.4.1、固定Wails3 beta.18；race使用可用MinGW。

实际命令：

```powershell
$env:GOWORK = 'off'
go test ./... -timeout 240s
go vet ./...
go build ./...
go test -race ./internal/app -run 'TestClipboard|TestCancelledClipboardLease|TestRemoteCancelledStream|TestLANPairing' -count=1 -timeout 180s
go test -race ./internal/app -run '^TestClipboardLAN' -count=1 -v -timeout 60s
# 在 apps/desktop
go test ./... -timeout 180s
go vet ./...
go build ./...
go test -race ./... -timeout 120s
# 在 apps/desktop/frontend
pnpm run typecheck
pnpm run lint
pnpm test
pnpm run build
```

上述检查通过；核心完整app包94.484秒，核心定向race38.288秒。前端15个文件/123项通过。最终新增LAN回归race通过，两个场景约4.12/4.26秒；覆盖无文件先行的双向文本、两端同时发起连接、两端pool保留相同SessionID、超过3秒文件空闲TTL仍可用、随后复用同一会话完成小文件及实际落盘内容一致。错误恢复测试也断言原生`write_failed`不能因连接恢复而被清掉。

日志在`.artifacts/clipboard-fix-20261019/`，包含`core-test.log`、`core-race.log`、`desktop-test.log`、`desktop-race.log`及`lan-final-race.log`。测试收尾的Pion `use of closed network connection`属于保留的socket关闭诊断，命令exit0；未为隐藏日志关闭身份或网络检查。

## 交付与边界

workspace模式的核心与桌面完整测试也通过（`core-workspace-test.log`、`desktop-workspace-test.log`），前端production build通过。前端完成后顺序生成Windows资源，以`GOWORK=off go build -tags production -trimpath -buildvcs=false -ldflags '-w -s -H windowsgui'`构建本地Windows试用程序，未启动。

源码保留本地未提交，既有更新功能继续保留。试用程序为`.artifacts/clipboard-fix-20261019/LinkSend.exe`，24,334,336 bytes，内嵌产品版本仍为0.6.1；SHA256为`cf28b0f4afdfa89d1b64b1d349c1f3baf1ea9e7a0be10900ba8860a2a9e90095`。来源与验证摘要在同目录`VERIFICATION.json`，不是GitHub已发布的0.6.1原包，也未覆盖已安装文件。构建生成的Windows资源移至artifact保留，不混入待审阅源码改动。

以上是首轮本地验证的范围；下节记录用户随后授权的SSH实机检查，不将后续结果移记到首轮。

## SSH Mac实机继续（2026-10-19）

用户明确授权SSH Mac测试及临时修改两端系统剪贴板；完整内容只在控制进程内存中备份，不进入日志或工作区。Mac别名`mac-test-102342413`，实际macOS26.5/arm64，192.168.1.243；Windows为192.168.1.122。Mac日常`/Applications/LinkSend.app`实际版本0.6.0，PID65291，旧设置页原生AX确实显示“连接失败”。

当前源码归档SHA256为`7750caf899259dca8fa623ad2e493e943090632362d20e038505a4877ca9fa2c`。独立目录`/Users/wen/.codex/supervision/linksend-clipboard-20261019`的Go1.27.1/cgo构建及定向核心/完整桌面test、vet通过，保留SDK26与部署目标的linker warnings。原生测试app二进制SHA256为`37b20856ec8ee1d0af4e25240c536c093a3dead455baf7db851db2e9834fd0ea`；本地测试ZIP`LinkSend-Clipboard-Test-mac-arm64.zip`的SHA256为`a5cb3f94e54784fce053bd838e6719dcd2b2b6a604ca5e9c81c6b4531f0736c0`。

Windows测试应用启动两次均被自动审批在CreateProcess前拒绝，理由仅`blocked by policy`；没有改入口绕过。用户随后自己运行已审阅的隔离启动脚本，得到准确测试EXE PID169292。Mac测试PID66643，日常app只通过已有AX临时暂停剪贴板，测试后恢复；没有改日常信任或网络。两个隔离profile由测试工具生成独立identity并固定对方公钥，六项文字/链接/图片grant显式开启，不使用信令正文或relay。

两端原生设置页实际显示监听器“运行中”、peer“就绪”。使用系统general clipboard的独特测试文字完成Windows→Mac及Mac→Windows读回；双向URL也通过。最终文字回执job为`/tmp/codex-ssh/job-ls-clipboard-clean-text-20261019-v3`，Mac原生设置回执为`job-ls-clipboard-verified-status-20261019-v1`。

图片尚未通过：`job-ls-clipboard-link-image-20261019-v1`的Mac回执为Windows→Mac PNG `false`，因此脚本没有执行反向图片复制；Windows图片验证曾读到自己刚复制的相同样本，不能算反向通过。助手已即时纠正先前过早的“图片双向通过”报告。下一轮必须采用不同方向的不同像素样本，并核对Mac颜色空间转换对验证器的影响；这只是待排查的测试工具因素，不作为产品根因。

控制夹具首次命令文件替换失败与短暂重复controller、PowerShell动态编译缺少Drawing/GDI引用，均单列为工具问题，后续切换单controller及直接写入受限命令文件。Windows最终原生剪贴板还原返回true。Mac最终还原因changeCount不匹配返回false，没有强行覆盖当前内容；测试agent已停止，不能报告Mac原内容已还原。两个测试应用均已按准确PID/path停止，旧Mac PID65291的临时暂停已恢复。没有覆盖安装、修改防火墙/路由/代理或发布。

关键作业：build=`/tmp/codex-ssh/job-ls-clipboard-build-20261019-v1`，最后收尾=`/tmp/codex-ssh/job-ls-clipboard-finalize-cleanup-20261019-v1`。新Mac测试包构建和实际启动已验，但系统图片与文件复制仍未交付；双NAT仍未运行。

## 用户新增文件复制与进度要求（实施中）

用户要求不能忘记复制文件和图片，进一步要求大文件复制时像隔空投送一样在屏幕中央显示“正在复制”和真实进度条；少量文字/图片不弹窗。当前自动协议只有text/link/image，复制文件尚无生产路径。不能用普通文件发送通过冒充文件剪贴板成功。

后续实现沿用现有认证QUIC与V1文件传输，新增能力协商和显式文件方向授权；大文件准备阶段使用不确定进度，传输阶段使用真实确认字节，支持取消并保持用户新复制优先。文件正文不进入JS、信令或relay，旧端能力不足明确提示。大小阈值暂按32MiB实施；该数值是本轮实现选择，非用户指定数值。协议/迁移与兼容安全回归必须一并记录。


## 文件复制候选本地回归与首轮实机（2026-10-19，图片仍排查）

新增能力 clipboard_files_v1、独立 files 收发授权、schema11→12备份迁移、Windows CF_HDROP/Mac file URL 原生元数据、10秒内预约/最长24h文件传输、V1文件正文及32MiB居中进度窗。新增测试覆盖真实认证QUIC文件落盘与本机路径、取消/撤权/暂停/新复制、旧端无能力/无授权不读元数据、异常绑定、迁移保留原授权且不静默授权files，以及Mac独立命名pasteboard读写/CAS。旧测试把files作为未知类型的两处负例已改成unsupported，原断言保持。

两Go模块workspace/off完整test、vet、build通过；核心相关race（app46.707秒、clipboardsync1.652秒）和桌面完整race通过。前端16文件/125项、typecheck/lint/build通过。浏览器440×208有效内容区无溢出，准备/实际进度/失败及取消按钮可达；截图 output/playwright/clipboard-progress-440.png、clipboard-preparing-440.png、clipboard-failed-440.png，仅作布局证据。

首个文件候选Windows为LinkSend-Files.exe，24,432,640B，SHA256 c0d377e0b4ba5839cdaaae9db2c566fec7bd2df29d727371a4ab0b49b239c42f。Mac源码归档files-source-v2.tar.gz SHA256 cccdb92abe5a200443af21380b2697cc1bfce32eafe1dd6a8f3da41f03f09156；arm64二进制8eddefd961a789feabf7ca570b386ee65d102d2efe7f97a4655659b78aee1f62，ZIP 09bcad1824214357e80e5658885d77f6152bf2c7405eadbeba1c3989d880543f。远程新目录/Users/wen/.codex/supervision/linksend-clipboard-files-20261019，build job /tmp/codex-ssh/job-ls-clipboard-files-build-20261019-v2；完整核心/桌面test、vet、production编译、签名验证通过，保留系统SDK链接警告；命名pasteboard文件/CAS专项另验通过。

新一轮Windows启动准备命令被自动审批在执行前拒绝，理由仍仅blocked by policy。没有换入口绕过。用户手动启动初次因复制时环境变量名/换行丢失未实际执行；改用带分号且无$env变量的单行命令后准确PID158804。Mac测试PID68634，日常PID65291仅临时暂停。两端新profile固定互信公钥，八项grant显式开启，测试限速2MiB/s只写隔离profile。

新包文字双向PASS，原生设置实际运行中/就绪，job-ls-clipboard-files-text-20261019-v2。小文件Win→Mac与Mac→Win均经原生文件条目读取、验证本机缓存路径和SHA256，并由原生剪贴板消费者复制到独立pasted目录；job-ls-clipboard-files-small-20261019-v1 exit0。该证据是原生条目消费，未把自动文件拷贝表述成Finder/Explorer键盘Ctrl+V实测。

64MiB Windows→Mac复制超过10秒lease，Windows原生窗550×300物理像素（125%缩放对应440×240逻辑像素），中心(1280,735)与主窗口一致；观察到0%、56%，Mac原生AX读到“正在复制”“正在接收并验证文件…”及12%。最终Mac原生文件条目和64MiB文件SHA256 42c4aea64f3ee9470ec5a9828ff92bf0860d44008a5470f210c56499f2f113c9一致，并复制到pasted目录，job-ls-clipboard-files-large-20261019-v1 exit0。随后第二次复制64MiB并点击原生“取消复制”，进度窗消失；Mac观察到新临时缓存出现后删除，当前剪贴板文件路径保持，job-ls-clipboard-files-cancel-20261019-v1 exit0。

图片仍未通过：使用红(180,60,30)/蓝(20,100,220)不同方向样本；Windows原生ReadPayload及真实窗口消息监听的只读探针确认2×2 PNG可读、Image=true。Mac尚未确认红图，故未将反向读回计为成功。期间还出现样本之外的787×882 PNG及剪贴板变更，按安全保护保留新内容而不强行恢复，重新备份后继续；是否外部复制或产品事件时序尚未确认，不能归因给用户。Windows新增只读探针默认跳过，不在普通go test读取general剪贴板。

首轮控制器因用户复制启动命令导致基线变化，未覆盖；后续完整内存备份支持CF_BITMAP副本并保留所有可读格式。当前为诊断阶段，图片问题未交付；候选未发布，日常安装/宿主网络未修改。

## 最终图片双向闭环、状态修复与恢复（2026-10-19）

本节更新上方按时间保留的“图片仍排查”状态。用户明确允许两端约一分钟无新复制后，Windows→Mac独立红图2×2、原始像素(180,60,30)与Mac→Windows独立蓝图2×2、原始像素(20,100,220)均经系统剪贴板独立读取验证通过。此前Swift校验器使用usingColorSpace转换，把正确原始红值转为(195,80,39)，引起假失败；job-ls-clipboard-files-image-diag-20261019-v4对比原始colorAt/bitmapData与转换值确认原因。最终夹具改为直接读取原始colorAt；job-ls-clipboard-files-raw-images-20261019-v1和Windows原生蓝图回执完成双向闭环，不把自己发出的同一图片当成反向成功。

最终一轮两端内存备份恢复均为true；Windows收据为隔离控制目录的clipboard-agent-final.json，Mac收尾为/tmp/codex-ssh/job-ls-clipboard-files-finalize-20261019-v1。Windows测试PID158804/诊断PID166584、Mac测试PID68634/诊断PID71337以及两端控制器均已停止；Mac日常PID65291恢复同步。早期因新复制而restore=false的轮次保持原记录，未覆盖新内容，不能据最终恢复推断首轮最初内容全部还原。生产源码中的临时trace instrumentation已移除，诊断源副本只在忽略的.artifacts中保存。

文件正文与原生路径验证后，新回归又复现完成时接收端仍显示“正在接收”。为运行态绑定私有FileID，结束仅清理自身文件等待态，成功清除旧file_copy_failed并保留独立原生错误；旧任务结束不能清掉新任务状态，取消不被晚到进度/finish恢复。TestClipboardFileCompletionCannotClearNewerCopyStatus及文件完成回归、相关race通过（files-status-before.log记录修复前失败，files-final-v3-race.log记录修复后结果）。设置新增copying_files中文映射。这一最终状态修复未重新运行物理测试，前述物理证据属于记录中的候选包。

发布准备将产品元数据统一为0.6.2。最终本地检查两模块workspace/off的go test ./...、go vet ./...、go build ./...全部通过，日志为release062-core-*.log与release062-desktop-*.log。前端125项、typecheck/lint/build及相关race已通过。准确提交、三平台CI、程序包hash与正式发布另行记录，不把本地候选实机结果移记为GitHub发行包已运行。
