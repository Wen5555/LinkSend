# R2 Windows v3 paginated handoff codec, 2026-10-26

本增量只增加未接线的私有 Go v3 codec / catalog 校验。没有修改 v2 controlled codec、launcher、helper main、native adapter、系统 clipboard 或 collector；不启动 helper、不读实际源 FD、不签发 quiescent receipt、不启用 GC。此前 [Shell primitives](RELIABILITY-R2-SHELL-PRIMITIVES.md) 的 generation 原子门缺口仍独立阻断发布，R2 不标记完成。

## 独立合同与预算

`internal/clipboardnativecontract/shell_catalog.go` 使用独立 `ShellPrepare`、`ShellCatalog`、`ShellCatalogPage` 和严格 version 3 / `windows-virtual-files-v1`。旧 `Prepare` v2 的内联 manifest / inventory / sources 和 256 KiB 帧、65 源、64 MiB 正文、controlled deadline 保持不变，旧入口不接受新首帧。新首帧只有准入绝对期限，不包含 post-export TTL 或 launcher 退出后停止指令；真正接入仍必须使用不可逆 Shell owner 政策，codec 本身不是寿命保证。

每页固定最多 64 KiB 原始元数据，使用原有有界帧读写。首帧固定总 bytes、pages、manifest entries、sources、完整 catalog SHA256、原 manifest digest 和新 inventory digest。接收先检查预算再分配，逐页严格核对版本、索引、固定长度和总摘要绑定，完整接收后再核对总摘要、严格 JSON 与 canonical 原字节。错页、遗漏 / 截断、重排 / 重复 / 未知 JSON 键、非规范数字或版本混用均不能返回部分已准入对象。额外页会留作下一阶段输入，未来阶段必须继续严格拒绝错序，不能忽略多余帧。

元数据总预算为 64 MiB，最多 10,000 manifest entries、20,003 nodes / sources 和总计 4,194,304 UTF-16 名字单元。这是元数据内存界限，不是文件正文总量上限；没有搬用 controlled 的 64 MiB 文件正文限制。root 固定首项，其他名字按字节排序，合法 `!note.bin` 不会因排在 `.` 之前而被排除。source 表与 node 表同序，不在每页增加独立索引或权限。

catalog 校验原 transfer manifest、相对名字、canonical Windows 文件身份、目录层级、唯一源句柄、stage / checkpoint / 每文件 part 和完整发布条目；提交文件必须与对应 part 的固定身份 / size / mtime 和声明 SHA256 一致。零 mtime 不增加新拒绝条件。目录源不能声明文件 hash，未知物理清单名和缺项拒绝。所有这些仅是签名元数据内部一致性；实际 FD 的类型、最小权限、真实文件身份、正文 hash 和可信 helper 镜像仍须在未来 handoff 中独立核验，不能由 metadata 自报替代。

## 用途签名与兼容

`ShellHelloMessage` 复用既有 challenge 用途域并固定签入 Version 3；`VerifyShellDelegation` 复用既有 delegation 用途域但明确只接受 Version 3 / 新 native contract，核对本次首帧 digest、challenge、inventory、实际签名字段与 publication / child / boot / source 摘要绑定。旧 verifier 仍只接受 delegation v1、prepare / hello v2 和 receipt-only-v1，双方不得互相降级或追认。这里没有扩大为任意签名服务，也没有新增 receipt / GC 权限；真实已固定 helper / child / kernel boot 证据须由 owner 另行核对。

## 实际最小验证

根目录加载 `.tools/use-desktop-toolchain.ps1` 后，均设置 `GOWORK=off`。日志和源码冻结在 `.artifacts/r2-shell-catalog/`：

- `go test ./internal/clipboardnativecontract -run '^TestShellCatalogV3RoundtripAndRejection$' -count=1 -timeout 60s -v`，首次 exit 0 / 2.507s，`catalog-tests.log`。之后源码审查指出合法 `!note` 的 root 排序问题及零 mtime 不必要限制，修正并合入同一 fixture 后仅重跑同一批，最终 exit 0 / 2.500s，`catalog-final-tests.log`。两次都保留，不扩大矩阵。
- 最终同一 roundtrip 包含 10,000 manifest files / 20,003 nodes 与 sources；元数据超过旧单帧 256 KiB，一个文件仅声明 65 MiB 大小，覆盖 `!note.bin` 与零 mtime。它使用合成身份和 hash，不读文件或声称这些正文已验证。固定用途 child / owner 签名与 v3 verifier 通过；错页、截断、总摘要篡改、分配前超限、重复键及双向版本混用必要负例均拒绝，旧 codec roundtrip 保持。
- `go vet ./internal/clipboardnativecontract` exit 0，`vet.log`；`gofmt` 和定向 `git diff --check` 通过。没有重复旧 controlled COM 回归、native 场景、full matrix 或 race。

## 尚未接线

本协议能够容纳完整原 manifest 的分页元数据，不表示 app 的 1,024-node 持久 inventory / 清理预算已升级，也不表示按需 FD pin / 真实 source hash、两阶段持久授权、签名发布 ack / echo、完整 launcher handoff 已实现。FILEDESCRIPTORW 的相对名字界限、global generation CAS、Explorer Ctrl+V、退出 / 重启 / 升级与自动删除均需独立后续证据。旧 CF_HDROP / file URL、receipt-only 和未知对象继续保护；不增加手动清理、容量或期限设置。
