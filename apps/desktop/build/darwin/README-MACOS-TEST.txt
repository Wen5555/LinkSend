LinkSend Wails 3 测试版

安装：将 LinkSend.app 拖到 Applications（应用程序）后再启动。
本包未使用 Apple Developer ID 签名或公证，仅用于内部测试。

首次验证：
1. 确认应用设置中的服务地址为 https://linksend.oooai.de。
2. 在设备页输入 orion123（仅测试使用，未来可能关闭）并加入。
3. 与另一台已加入设备完成指纹核验后，再进行发送和接收确认。
4. 传输完成后核对目标目录中的文件哈希；失败时按界面阶段提示重试。

隐私边界：文件字节只经身份认证和加密的点对点 QUIC 数据面传输，不经过信令或 JavaScript IPC。
