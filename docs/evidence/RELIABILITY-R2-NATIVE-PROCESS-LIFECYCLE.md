# R2 real launcher exit and independent main, 2026-10-26

本次接续`f613e81c7151af5e709313d3909fd44f8e2ab1ed`的私有受控broker，仅证明实际launcher进程退出、已有真实COM consumer/stream继续读取、helper签原spool、独立新main消费原profile/同SQLite一次。系统clipboard/注册格式/GUI/Explorer、用户文件和Native删除均未接。打包/resolver为并行独立增量，不把其未验行为算进本记录。

## 固定授权和实际进程

私有launch使用v2 prepare/hello：原绝对deadline与wait-for-launcher-exit策略进入完整prepare摘要和child签名challenge；receipt/delegation仍v1，数据库schema12不变。旧v1/helper版本混用按严格codec/版本和准确image锚拒绝，不增加网络wire或JS能力。

helper原native owner在STA↔MTA标准COM marshalling后，已真实创建provider、stream/clone、async并释放原marshal table，才通过私有只读native Held门。Go只对固定Held摘要签名，入口不接受refs、trace、关闭布尔值或任意Sign参数。launcher确认Held后detach，helper用原继承稳定进程句柄等待真实kernel exit0，才advance原consumer继续读EOF、释放所有原生引用、逐句柄Close和固定receipt签发。期限在prepare时确定，helper只转换一次本机单调预算，各阶段和新main不续期；非零launcher exit与到期均保护且不签。

driver从launcher直接DuplicateHandle取得helper内核进程对象，先核对实际image SHA、仍存活与spool为0B，再放行原launcher。没有通过自报PID调用OpenProcess。新的main是另一个真正的test-role进程，重新读取原profile真实Ed25519身份、同SQLite和原run记录，核验原spool并消费一次，重放不能改变首次观察；压力collector和源SHA256断言均保留。

## 一次最小验证

现有Go1.27.1、MinGW g++16.2.0；没有安装依赖、改宿主DLL/PATH或做GUI/clipboard操作。证据目录`.artifacts/r2-native-process-lifecycle/`，旧冻结artifact及首增量EXE没有覆盖。

- desktop：`GOWORK=off CGO_ENABLED=1 go build -o ../../.artifacts/r2-native-process-lifecycle/clipboardfilehelper.exe ./cmd/clipboardfilehelper`，exit0，`helper-build.log`。保留`-std=c++17 -Wall -Wextra -Wpedantic -Werror`及GNU runtime静态链接。
- root：显式设置可信本地新EXE与其独立预期SHA后，`GOWORK=off go test ./internal/app -run '^TestClipboardCacheWindowsNativeProcessLifetime$' -count=1 -timeout 90s -v`，exit0，app8.380s/生命周期3.23s，`process-first.log`。
- 正例实际launcher PID64872 exit0、helper PID29500 exit0、独立main PID62292；原deadline保持，原run消费一次/重放拒绝，source protected。
- 必要负例实际launcher PID64288 exit9、helper PID63112 exit1、独立main PID11436；spool维持0B，新main拒绝消费、Native仍delegated且源SHA保持。
- 因公共commit实现增加detach分支，补一次`GOWORK=off go test ./internal/app -run '^TestClipboardCacheWindowsNativeControlled$' -count=1 -timeout 60s -v`验证原正常commit路径，exit0，app4.774s/功能0.21s，`normal-commit-compat.log`。
- `GOWORK=off go vet ./internal/app ./internal/clipboardnativecontract`及desktop的`GOWORK=off CGO_ENABLED=1 go vet ./cmd/clipboardfilehelper`均exit0，`core-vet.log`/`helper-vet.log`。未扩完整矩阵/race/压力多轮。

测试role各自使用driver自有临时目录；launcher正常关闭原Service与profile owner后实际`os.Exit`，helper不持DB/profile key继续原COM读取。验证新main的PID与原launcher不同，原public identity/OwnerRun与signed row保持。以上替代的是首增量中尚未证明的真实进程边界，没有把同进程Service reopen冒充实际重启。

## 安全审查和限制

独立只读review确认成功路径的Held绑定完整prepare/原期限，stage1实际持有streams/clones/async，原稳定launcher HANDLE真实exit0之后才advance，末尾仍需eligible、0x7f保护检查和实际源Close成功。review指出并明确保留的边界：commit已发但Held未确认时，当前受控helper可能在取消/失败中终止；不声称这个失败窗consumer仍可用。30秒硬期限只适用于当前自有受控消费者，不能直接用作未来系统Shell消费者寿命或退出策略。没有因为新receipt开启任何Native回收。

准确helper与六个构建源hash见`helper-freeze.json`，EXE SHA256 `46fe598c4c6207a497a3fa5d0a9f6a1013779d2dde05c35eb0e62de8ceaa7c0a`；最终本增量文件清单/PE审计与hash见`freeze.json`。R2完整系统发布、键盘粘贴、升级及自动回收仍未完成。
