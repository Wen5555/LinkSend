# ADR0018：跨网候选收集的解析与默认服务容错

日期：2026-10-26。状态：实施并验证，准确来源及实网范围见[验证记录](../evidence/CROSS-NETWORK-20261026.md)。不改变文件wire、身份校验、relay=false、任务库或用户配置格式。

## 故障与处理

设备在线只证明信令在线。现场默认STUN域名被系统解析为198.18/15 Fake-IP，绑定物理网卡的UDP不能依靠HTTPS代理获得公网候选；默认服务真实地址也无UDP响应。单个默认STUN因此成为跨网可用性的故障点。两端是否能打洞仍取决于实际拓扑，STUN成功不证明NAT类型或认证文件连接成功。

桌面自动默认值采用原服务与stun.cloudflare.com:3478两路，仍只有UDP STUN。实际连接与EffectiveConfig使用同一策略；只扩展空配置及曾保存的单个内置默认值，显式环境变量和自定义列表保留。读取不写回偏好，无新用户设置。空环境列表不再造成界面显示默认、实际却没有STUN的分歧。

## 应用内解析

复用锁定Pion v4.4.2的ice.WithNet及transport/v4 stdnet，只覆盖两个内置公开STUN主机的ResolveUDPAddr。显式IP、其他主机及所有socket操作沿用原实现；不改系统DNS、代理、路由、防火墙或UDP mux，不增加简化ICE或文件中继。

内置主机先走有期限的系统解析。Fake-IP、无结果或错误时，查询固定HTTPS DNS JSON端点cloudflare-dns.com/dns-query和dns.alidns.com/resolve；不向公共解析器发送任意私有STUN主机名。不在Endpoint.New发起DNS/HTTPS，在Pion srflx收集goroutine中解析，host/LAN收集并行且不依赖外部响应。总预算2.5秒，第一提供方不能耗尽备用机会；Endpoint.Close先取消解析，再关闭既有owner。

HTTPS保留证书验证、拒绝重定向，仅接受DNS JSON/JSON、16KiB响应、准确Question、最多64回答及8轮CNAME。检查名称链、家族和地址，拒绝Fake-IP、私有/本地/保留目标的外部答复；仅受控loopback测试可放行本地答复。TTL取链中最短值并限制缓存寿命，失败短暂退避。同键合并，最多4个缓存键，整个进程最多4个并发HTTPS查询；不保存文件内容、身份或凭据。

## 失败取证

ICE失败也发布本次实际socket、网卡/地址族、STUN/信令计数、ICE时间线和耗时。未选中候选时不补造路径/对端地址/连接方法，未握手时TLS=0、ALPN空；错误仍按实际阶段分类。STUN收到了回复与已建立文件连接是独立事实。

## 回退与验收

这是代码回退：回退程序即可恢复原策略，无需用旧库覆盖数据。现有显式网络偏好、身份、配对和文件保持。原STUN的宿主UDP3478规则/DNS切换不在本修复授权内；采用已实测的备用服务，不借修改宿主网络掩盖问题。

验收包括旧默认回归失败→修复通过、真实Pion同socket收集、host优先/关闭取消、TLS/重定向/响应/名称/TTL/并发边界、双角色ICE失败证据、两模块workspace/off检查及准确候选CI。Windows↔Linux不同网络认证文件/hash证据与Windows↔Windows双端现场证据分别记录；loopback、单边NAT或公共主机不转记双NAT全面可用。
