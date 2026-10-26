# R2 原生签名授权与单次回执基础（2026-10-26）

基线 `3d20ccb6d84a6c1edbeb9879e2fb721191f53f47`。本增量仅实现私有 `receipt-only-v1` 记录、签名核验、原 publication 跨 main 重启的单次 CAS 消费及保护门；没有原生 launcher/boot provider 的生产实现，没有接 WriteFiles 或系统剪贴板，不能标记 R2 完成或已发布缓存可回收。范围先冻结在[ADR0016](../adr/0016-automatic-clipboard-file-cache.md)。

## 实现与保护边界

同一 schema12 / SQLite metadata owner 的缓存记录增加可选 Native。nil 时旧字段顺序、JSON 字节及签名保持，完整旧可选字段 golden 逐字节验证；不新增数据库或迁移，不重签旧数据。当前 writer 原本输出 canonical typed JSON；decode 现在要求实际 raw 与该表示完全相同，重复/别名/未知/尾随或非 canonical 数据保护、不修复、不删除。

delegation 与 receipt 各最多4KiB，沿用 Go Ed25519、SHA256 和固定用途域。绑定 profile、原 OwnerRun/BootID、cache/root/object/stage、清单/计划/实际 inventory、publication/generation、nonce、helper image 及 child public key。实际接收句柄关闭观察得到的完整 inventory 先封存到原记录，再原 raw CAS 写 delegation。launch 与 boot 的强类型接口目前只有 `_test.go` 实现及调用，生产路径不能凭自报 hash/布尔值建立授权；CF_HDROP/legacy/unknown 不被追认。

独立 consume 按原 signed record/delegation 核对原 publication，允许同 profile 新 main 在真实身份密钥重载后消费原 OwnerRun 的合法完成回执；原 receipt.save 的 current-run 限制不变。重新核对 pinned roots/完整树、排除本机活动引用，同一共享连接事务按旧 raw CAS，事务内及提交后回读；重复消费不重置首次观察时间，不再发布原授权。helper 时钟不作为回收依据。

签名只证明出处和绑定。测试明确给有效 child key 签发“源已关闭”的自报消息，即使通过该层核验，Native 对象在正常/压力、老化或 boot 改变后仍保留且内容可用；任何 Native 都不进入当前未发布删除器。collector 扫描、逐对象、未发布 discard、legacy beginNative 和 forgetRemoved 均保持该边界。实际 helper 的固定可信 image、稳定 child 句柄/challenge、真实 native refs/源句柄结束后唯一签发、pathless publication 与系统消费者仍待后续实现，当前 C++ plain trace 不能授权生产删除。

## 因果缺陷与回归

独立 review 发现逐对象 collector 原先只比较 DB raw，却信任另传入的 record 副本。新自有 fixture 保留 DB 的真实 Native，但将调用副本 Native 清空、改成 reclaimable，旧实现确实删除了该测试源（`go-native-stripped-record-before.log`，exit1）。现 raw 相等后重新验签解析 DB 的实际 row，再按该 row 检查 Native、sealed 和合法删除态，之后只使用 authenticated row；新负例保留源与 hash（`go-native-stripped-record-after.log`，exit0）。未删除任何用户文件。

真实 key/SQLite/树回归覆盖：旧 main→新 owner 的12路并发只一 CAS；原 r.save 跨 run 拒绝；全部绑定/签名/重复未知 JSON、重放/代次替换；活动引用、真实根/条目身份替换；SQL abort、事务内回读篡改回滚与半写；完整 legacy/nil signing golden；有效签名自报关闭后仍零删除。没有使用 Go 验签测试冒充 native 生命周期证明。

## 实际验证

PowerShell7，锁定工具链。原始根 `.artifacts/r2-native-reader-draft/`：

- `GOWORK=off go test ./internal/app -run '^TestClipboardCache(Native|Ledger|Lifecycle)' -count=3 -timeout 180s`：exit0，app29.800s，`go-native-final-ordinary.log`。
- 同范围 `go test -race ... -count=3 -timeout 240s`：exit0，app60.999s，`go-native-final-race.log`。
- `go-native-final-verification.json` 保存完整六文件 SHA、命令及独立审查结果；new native.go SHA256 `5db20e7e0f3393d040ea015a27c1ba9b962b99b3ff6cb40dcc5538c5fb4db6b3`，native_test.go `307b51fa7e1c71a9c7de99ba7419eecc4396bc8ac65dc0a127ecd9984e747e3a`。

最终两模块 workspace / GOWORK=off 分别运行 `go test -count=1 ./...`、`go vet ./...`、`go build ./...`，12条均 exit0；日志 `.artifacts/crossnet-20261026/r2-native-auth-final-3d20-{core|desktop}-{workspace|off}-{test|vet|build}.log`。耗时毫秒：

| 模块/模式 | test | vet | build |
|---|---:|---:|---:|
| core workspace | 137610 | 2646 | 4892 |
| core off | 142279 | 1598 | 7043 |
| desktop workspace | 31437 | 1224 | 4238 |
| desktop off | 19993 | 866 | 4162 |

没有前端/dist、正文 wire、权限默认值或缓存用户设置变化。准确提交 `52f9bf20c3fb2ac71bffa0ee71521e9af40048dc` 的 core40240526707、desktop40240526725、packages40240526706 已再次查询均 attempt1/completed/success，无重试，回执 `.artifacts/share-native-ui-draft/current-exact-ci-status.json`。准确3d20候选包含此前 R5 产品源码，不能将本增量倒记到该包。生产 native/helper、已发布缓存安全回收与双平台键盘粘贴仍为必做缺项。
