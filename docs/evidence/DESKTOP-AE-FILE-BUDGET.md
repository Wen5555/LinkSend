# 2026-10-18 N3 文件正文限速

契约见[ADR0011](../adr/0011-process-file-body-budget.md)，当前状态由[唯一TODO](../DESKTOP-EXPERIENCE-TODO.md)维护。它是Go正文预算，不是精确UDP线上带宽控制；没有改变ICE/QUIC、身份pin、hash、安全落盘或relay=false。

## 已验证行为

进程共享token bucket覆盖所有文件发送、队列、批次与恢复；默认0不限速、有效上限1KiB/s–1GiB/s。100ms突发、最少1KiB/最多64KiB，闲置不累计无限额度；非零动态修改不新造预算，改0唤醒等待。预算等待只持内部短锁，可被task context立即取消。应用级重复文件块再计费，控制与自动剪贴板不经过该预算。

假时钟覆盖第二发送者共享余额、50/100ms补充、长时间闲置容量、重复/动态保存不充值、改不限速唤醒、取消与timer停止、0/负数/过小/最大int溢出边界。帧夹具核对长度头和正文编码完全相同，真实Send/Receive中PreviouslySent所有块仍与总正文等量扣费，控制未计入。

桌面偏好分类sending只保存本字段，不覆盖名称/剪贴板；CAS冲突、非法值磁盘与运行时均不变、重开保持与旧偏好缺字段默认0通过。format1可选字段由旧程序忽略，回退旧版不支持限速，不能称仍限速；当前任务库11、wire不变。

## 实际命令与结果

| 范围 | 命令/本地日志 | 终态 |
|---|---|---|
| 核心独立模块 | `GOWORK=off go test -count=1 -timeout 6m ./...`及vet/build；`n3-verified-core.log` | PASS，app79.594s/transfer10.650s |
| 完整并发 | `go test -race -count=1 -timeout 6m ./internal/app ./internal/transfer`；`n3-verified-race.log` | PASS，app195.242s/transfer10.617s |
| 桌面两模式 | workspace test、off-race/vet/build；`n3-verified-desktop.log` | PASS，workspace5.820s/off-race8.625s |
| 前端 | 固定Node24.21.0/pnpm12.4.1 test/typecheck/lint/build | PASS，11 files/101 tests；最后CSS增量重build |
| 真实绑定 | Wails beta.18/Go1.27生成；`n3-bindings.log` | PASS，95methods/66models，无前轮旧toolchain警告 |
| 真实QUIC/独立剪贴板 | `go test -race -v -count=1 -timeout 2m ./internal/app ./internal/transfer -run '^TestFileBody'`；`n3-real-first.log` | PASS，app11.637s/transfer2.200s |
| 首offer前连接过期与限速组合 | race五轮；`pool-expiry-and-budget-green.log` | PASS，58.504s；原代次固定后定向三轮9.410s |
| 最终集成增量 | `go test -race -count=1 -timeout 2m ./internal/app -run '^(TestPooledPeerRemoteIdleCloseBeforeOfferReconnectsOnce|TestFileBodyLimit|TestBatchCancel)'`；`n3-final-combined.log` | PASS，15.894s |

完整core/race先于连接过期修复，后两行覆盖该新增路径与取消/限速组合；不把旧二进制测试当新路径执行。最新源码CI/准确包另外核对。

1MiB文件、64KiB块、256KiB/s：16块首块后至末块为3.7633651s，理论余下960KiB约3.75s；接收hash与实际正文大小一致。另一次活跃发送在首块后改0，剩余正文及时结束。1KiB/s文件处于正文等待时，真实认证QUIC上的内存剪贴板adapter在7.1292ms写入；随后Pause、Resume、Cancel各自完成，源文件保持。均是同机小样本，未触碰OS剪贴板，不报告p95或物理两机结论。

## 界面与资源

Playwright独立session ae-budget、生产SettingsPage/FileSendLimit/CSS与明确mock偏好；960×640浅色、1100×720深色无水平溢出，保存按钮在视口内。空值显示错误并禁保存；Tab/字母/Enter切换单位保留精确值，MiB↔KiB换算核对。初次深色焦点select背景白、低对比，已以局部主题token修复并重新截图查看；原始失败截图保留，最终截图在`output/playwright/ae-budget-20261018/*-fixed.png`，不入Git。

0 errors/1 warning，warning为Wails浏览器模式提示；未运行原生保存RPC。owned浏览器/Vite及两份临时入口收尾后核对无监听；原生Windows/Mac、物理限速/剪贴板和A–E其他缺项仍继续。日志/profile/原始正文全部位于忽略的本地artifact，不提交。
