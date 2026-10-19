# R5 临时分享授权状态与持久核心（2026-10-19）

基线 `1caddbdc93f1c55b7a5579dcdc5effc4d46dfbc9`。先冻结[ADR0017](../adr/0017-one-time-share-grant.md)，本增量只实现Go授权状态机和既有Control连接上的事务存储。当前服务器没有调用激活函数、没有临时分享路由，也不广告能力；不是可用的两端分享产品，R5仍未完成。

## 已实现边界

- 20字节crypto/rand分享码，32字符Base32及固定分组/ASCII校验，创建API仅接收带用途域SHA256摘要；领取API只以明文派生验证器，控制库/WAL不存明文。客户端生成的ID/摘要使创建可幂等重试，不依靠服务端保存或回读明文；码丢失需新授权。
- offered/pending/approved/bound/completed/revoked/expired，发送方明确批准与接收方保存确认两道门。领取2分钟、总领取10分钟，绑定后原任务最多24小时恢复且重试不续期。拒绝、超时重新领取推进generation，晚到批准不能绑定新接收人；撤销优先后不接受完成/恢复回调。
- 严格绑定两端公钥、分享ID、清单、claim、task、计划、generation/revision。已完成仅核对原任务终态，不再授权文件操作。私有记录不是UI或诊断DTO，未知字段/版本/非法值/索引不一致拒绝，记录读取在SQL侧限制8KiB。
- 事务读取、校验、行revision CAS后提交，单连接借用Control owner；没有新数据库owner或文件正文通路。最多4096条记录、每发送身份8个未终态、每领取身份4个未过期待确认；分页对账每次1–64条，过期处理与7天后终态回收合计不超过该预算，不因压力删活动授权。
- 显式EnableShareGrants进行控制库4→5迁移：VACUUM INTO一致性备份包含已提交WAL，唯一文件不覆盖，0600创建；备份integrity/schema/成员回读后事务建表、更新版本，最后quick_check/列回读。重复激活不重新备份。普通Server.New仍用schema4，不自动升级；新代码可重开已显式激活的schema5，缺表/未来版本拒绝。任务库schema12、V1文件wire、成员与永久信任保持。

## 已确认验证

Windows Go1.27.1；仅自有临时目录与测试身份，没有生产库或用户文件改动。

- 状态机定向测试验证两道确认前无正文授权、错误身份/清单/计划/task/generation拒绝、领取占位到期释放、旧确认隔离、拒绝幂等、原任务有限恢复、完成确认丢失可核对但不能重开、撤销后晚回调及未知存储保护。
- 真实SQLite 32路并发领取只有1个成功，其他31个冲突，回读winner/revision保持；devices/groups仍0条。实际两个独立组的身份领取后，各组ID与membership revision保持。
- 控制库关闭/重开保留原领取与bound授权，不延长恢复期限；错误公钥、源摘要替换、期限边界拒绝且行不变。撤销与完成实际并发，只有1个终态成功，最终revision只前进一步且无新文件授权。
- 并发/发送限额失败不消费原offer，单次对账预算1只处理1条，活动记录保持；新终态保留，满7天后才进入有界回收。取消迁移仍schema4，伪schema5缺表拒绝；备份保留成员/WAL且不覆盖。
- 首轮备份“不覆盖”测试FAIL：夹具用普通OpenControl打开备份触发journal_mode变更。已改用query_only连接核验备份；生产迁移的核验原本就query_only，失败与修正边界保留，不把夹具错误记产品数据丢失。
- 最终`go test ./internal/sharegrant ./internal/store -count=3`退出0（sharegrant1.825s/store3.716s）；定向`go test -race ... -count=3`退出0（2.488s/6.834s），见r5-grant-core-final-{test,race}.log。两模块workspace/off的test -count=1 ./...、vet ./...、build ./...完整12命令全部退出0，日志r5-grant-core-{core,desktop}-{workspace,off}-{test,vet,build}.log。无前端改动，复用1caddbd已验dist；本提交CI推送后核对，不预记成功。

## 仍必做

本层API只接受Go调用者已认证的actor，不把公钥参数本身视为网络身份认证。HTTP/WS签名、防重放、IP/身份猜码限频、能力协商、临时公钥与认证P2P作用域、任务取消/源变化/重启生命周期、双端UI及真实文件/跨网分享尚未接入，不能据此宣称这些测试通过。SQLite重开是存储重启，不是完整服务/桌面重启。

旧服务源码明确拒绝schema5；准确旧二进制对升级库的启动拒绝尚未实测。回退程序与恢复数据分开，不能以schema4备份覆盖升级后的新成员写入。R1真实网络、R2完整缓存与键盘粘贴、R3 Mac原生、R4双平台完整旅程与R6同候选收尾继续。
