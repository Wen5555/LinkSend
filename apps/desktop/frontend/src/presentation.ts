import type { DeviceInfo, QueueItem } from '../bindings/github.com/Wen5555/LinkSend/internal/app/models';

export const formatBytes = (n?: number | null) => n == null ? '未知' : n < 1024 ? `${Math.round(n)} B` : n < 1048576 ? `${(n / 1024).toFixed(1)} KB` : n < 1073741824 ? `${(n / 1048576).toFixed(1)} MB` : `${(n / 1073741824).toFixed(1)} GB`;
export const deviceName = (device?: DeviceInfo) => device?.profile.alias || device?.name || '未命名设备';
export function deviceConnectionLabel(device: DeviceInfo): string {
  if (device.blocked || device.relationship === 'removed' || device.connection_state === 'blocked') return '已阻止连接';
  if (device.connection_state === 'connected') return '已连接';
  // A signed announcement and server presence are hints for a connection attempt,
  // not proof of a working LAN control or authenticated data connection.
  if (device.lan_control_state === 'discovered_unverified' || device.nearby) return '已发现 · 连接待验证';
  if (device.service_state === 'membership_synced') return device.online ? '在线 · 直连待检查' : '未见在线记录';
  return '连接状态待检查';
}
export const fileName = (path: string) => path.split(/[\\/]/).pop() || path;
export const taskLabels: Record<string, string> = {
  preparing: '准备中', awaiting_acceptance: '等待确认', transferring: '传输中', verifying: '校验中',
  recovering: '待恢复', paused: '已暂停', pause_requested: '正在暂停', rejected: '已拒绝', completed: '已完成',
  failed: '失败', cancelled: '已取消', cancel_requested: '正在取消', shutdown_requested: '正在保存',
  no_content: '未接收内容',
};
export const queueLabels: Record<string, string> = {
  queued: '等待发送', waiting_peer: '等待设备上线', needs_attention: '需要确认', running: '正在执行',
  completed: '已完成', cancelled: '已取消', expired: '已过期', failed: '发送失败',
};
export const canOrderQueue = (item: QueueItem) => ['queued', 'waiting_peer', 'needs_attention'].includes(item.state);
export function movedQueueIDs(items: QueueItem[], id: string, direction: -1 | 1): string[] {
  const ids = items.filter(canOrderQueue).map(item => item.id);
  const from = ids.indexOf(id), to = from + direction;
  if (from < 0 || to < 0 || to >= ids.length) return ids;
  [ids[from], ids[to]] = [ids[to], ids[from]];
  return ids;
}
