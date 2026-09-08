# LinkSend v0.1.0 Windows Preview

发布日期：2026-09-09（Asia/Shanghai）

## 交付内容

- Wails 2.15.0 Windows/amd64 生产构建：`apps/desktop/build/bin/LinkSend.exe`
- 预览交付目录：`.artifacts/windows-preview/20260909-0405/`
- ZIP 包、SHA256、中文启动说明、验证记录和脱敏浏览器截图

## 用户流程

1. 双击程序进入传输页；首次使用可在“设置与诊断”填写服务地址、绑定地址、STUN、设备名和接收目录。
2. 在“设备”页生成邀请或粘贴邀请加入设备组。
3. 加入后核对完整设备指纹，明确点击“验证并信任”。
4. 发送方选择可信设备和文件/文件夹后发送。
5. 接收方选择目录并开始等待；收到请求后查看认证发送者、内容摘要、条目数量和总大小，再接受或拒绝。
6. 任务区显示阶段、进度、速率、错误、取消和失败重试；完成后可打开接收目录。

## 主要改进

- 新增本地偏好原子写入，环境变量覆盖本地设置，保留既有身份和信任文件。
- 新增真实网卡枚举与绑定地址配置，避免把 Mihomo/TUN 地址当作普通 LAN 地址。
- 新增安全目录打开桥接，只允许已完成接收任务对应的目录。
- 接收任务快照补充真实 `peer_id`、manifest 摘要和文件条目数量。
- 桌面接收等待时限固定为 10 分钟，ICE 检查时限为 30 秒。
- 前端刷新拆分为任务高频刷新和远端状态低频刷新，并防止请求重叠。
- 浅色视觉系统采用语义 tokens、侧栏导航、状态条、分层表面、键盘焦点和减少动效规则。

## 验证证据

- 根模块 `go test ./...`、`go vet ./...`：通过。
- 桌面模块 `GOWORK=off go test ./...`、`go build ./...`：通过。
- 前端 `pnpm run typecheck`、`lint`、`test`、`build`：通过。
- Wails 2.15.0 production build：通过。
- 隔离 rendezvous + 两个随机 profile：bootstrap、邀请、加入、双向 trust、16 MiB 随机文件 QUIC 传输和双端 SHA256：通过。
- 香港主站 `https://linksend.oooai.de/healthz`：HTTP 200，`protocol_version=1`、`transport=quic`、`relay=false`。
- 香港主站 443 rendezvous listener、3478 coturn STUN-only listener：通过只读检查。
- Playwright 实际渲染检查传输、设备、设置三页，截图已随产物提供。

## 明确边界

- `relay=false`，文件字节只经过已认证 QUIC 直连。
- Wails 原生窗口人工点击和桌面截图受当前自动化环境限制，未完成。
- 新版本 Windows↔macOS、跨 NAT、IPv6、网络迁移和睡眠唤醒未运行。
- 拖放、深色模式、托盘、安装器、签名、任务持久化、重启恢复和并发队列未包含在本预览版。

## 源码状态

- 分支：`main`
- 发布提交：`2f2bde15c5b3f862f372c4f725f2c4acc8603bd1`
- Annotated tag：`v0.1.0-windows-preview`
- 工作树在发布前已清理；没有自动部署或生产配置变更。
