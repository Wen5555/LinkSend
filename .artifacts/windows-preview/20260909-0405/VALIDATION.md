# LinkSend Windows 初版验证记录

## 已实现
- Wails 2.15.0 Windows/amd64 生产构建，三页应用壳（传输、设备、设置与诊断）。
- 本地偏好原子保存（服务地址、绑定地址、STUN、设备名、接收目录），环境变量覆盖本地设置并保留现有 profile。
- 真实文件/目录选择、可信设备筛选、邀请生成、加入设备组、完整指纹信任。
- 任务发送、接收等待、接收端确认/拒绝、取消、失败重试与完成后的目录打开。
- 任务与健康轮询分离，浏览器桥未注入时显示明确不可用状态。
- 接收 manifest 摘要、条目数和真实 peer ID 进入任务快照；完成接收任务的目录打开按任务 ID 校验。
- 接收等待时限由桌面层固定为 10 分钟，任务/远端轮询防止重叠请求。

## 命令与结果
- `GOWORK=off go test ./...`（apps/desktop）：PASS
- `GOWORK=off go build ./...`（apps/desktop）：PASS
- `go vet ./...`（apps/desktop）：PASS
- `pnpm run typecheck`：PASS
- `pnpm run lint`：PASS
- `pnpm run test`：PASS（现有 3 项连接方式测试）
- `pnpm run build`：PASS
- `wails.exe build`：PASS，生成 LinkSend.exe
- 直接运行 LinkSend.exe：进程启动并正常退出（当前无可见桌面捕获环境，未完成点击式 Wails 验收）
- 使用隔离 `LINKSEND_DATA_DIR` 启动 LinkSend.exe：进程保持运行超过 3 秒后由验证脚本结束；WebView2 Runtime 152.0.4191.66 已检测到。
- Playwright 实际浏览器渲染：传输、设备、设置三页截图已保存；检查空状态、错误条、设置表单和 1280×720 视口。
- 隔离 rendezvous 双 profile E2E：bootstrap、邀请、加入、双向 trust、16 MiB 真实 QUIC 传输与独立 SHA256：PASS。测试 profile、密钥、数据库和大文件已清理。
- `git diff --check`：PASS（仅 Git 的 LF/CRLF 转换提示）。

## 未运行 / 待验收
- 新版本 Windows↔macOS 实机互通、跨 NAT、IPv6、睡眠唤醒/网络迁移：NOT_RUN / BLOCKED_BY_EXTERNAL_ENV。
- Wails 原生窗口截图和人工点击验收：当前环境无可用可见桌面捕获通道，NOT_RUN。
- 签名、安装器、持久化任务历史、并发队列：本轮未实现。

截图来源：`screenshots/*-browser.png` 是本地 Vite + Playwright 渲染；没有伪造 Wails 桌面截图。
