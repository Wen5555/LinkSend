# E0-A 中间审查交接

2026-09-13。本检查点依据E0-dispatch-v1允许的“较大独立闭环中间提交”，**不是整批E0验收结束**。
执行树208a，分支codex/desktop-experience-upgrade；无产品Go/React实现修改，E1未启动。

| TODO | 本检查点结果 | 尚缺 |
|---|---|---|
| E0-01 | 准确四包重hash/GitHub digest、工具链、真实Windows payload问题复现 | 准确NSIS/Mac安装身份、完整七项矩阵 |
| E0-02 | 三主机当前审计、香港health/WSS认证/在线列表、隔离namespace能力 | 物理完整桌面双向链路、真实双NAT实验 |
| E0-03 | 真Share Target源码/MSIX/开发签名与安装失败收据 | 系统信任门槛、激活/授权/冷启动/卸载矩阵 |
| E0-04 | 真.appex可执行包/注册/系统共享服务窗口、注销复核 | 自定义controller/附件回调未取得成功回执；文件交接、冷启动、Finder实操 |
| E0-05 | ADR0007提案与产品规范链接 | 总控判定后冻结，后续协议/安全兼容实现测试 |

证据分别位于DESKTOP-E0-BASELINE/NETWORK/WINDOWS-SHARE-PROTOTYPE/MAC-SHARE-PROTOTYPE/DECISIONS.md。
检查点没有PASS U1–U7；Mac回调问题属于待修原型缺陷，不能笼统称缺签名导致。AX=false与0签名身份是独立实测限制。

## 验证与清理

根基线go test/workspace/独立vet：总控固定3bcb730实际退出0（缓存边界保留）；desktop独立test/vet/build退出0；新增control-inspect编译/vet与真实控制面运行退出0。
Windows修正版dotnet publish/makeappx/sign退出0，安装退出1；Mac最终swiftc/codesign/注册/激活wrapper退出0，不代表附件回调成功。
普通git diff --cached --check只剩**原始方案第3/4行**的Markdown硬换行双空格。两份用户材料保留原始hash；排除此原始方案的新改动检查退出0，脚本精确核对原方案仅该两行有尾空白，未关闭全局检查。
临时Windows证书已从两个CurrentUser存储精确清理；没有安装包残留。Mac测试host终止，最终pluginkit精确IDno matches，无原型进程；bundle归档仍保留。Windows隔离控制身份副本已删除。
所有原始日志和包在ignored `.artifacts/desktop-experience/e0/`，不携带用户身份、私钥、正文或无关监听原始日志进入提交。

## 审查后下一批

优先修Mac自定义controller生命周期并取得真实file representation/URL授权回执；在可审查Windows安装步骤准备好后解决管理员信任最后一步。
同时继续E0-01/02准确两平台包、发现/配对/控制/双向文件hash；不能用指定地址探针代替桌面。nl保持专用隔离namespace，不动宿主网络。
ADR中的成员代际、组删除vs本机屏蔽、旧端硬拒绝、LAN provisional互认和剪贴板冲突规则需要总控审查；未经审查不推送、不启动E1。
当前任务工具不提供跨任务消息接口，工程文件和本地提交是可恢复交接入口；未宣称消息已送达或总控已验收。
