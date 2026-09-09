// Only the backend's evidenced connection_method is eligible for a LAN label.
// ICE candidate type (including "host") is deliberately not an accepted method.
export function connectionMethodLabel(method: string): string {
  switch (method) {
    case 'lan_direct': return '局域网直连';
    case 'internet_p2p': return '互联网 P2P';
    case 'relay': return '尚未实现';
    default: return '直连 · 路径待确认';
  }
}

export function humanizeBackendError(raw: unknown): string {
  const text = String(raw ?? '').replace(/^Error:\s*/, '');
  if (text.includes('request signature or membership invalid')) {
    return '当前身份尚未加入这个设备组。请先使用已加入设备生成的一次性邀请，在“设备”页完成加入；只有已加入且具备管理员权限的设备才能生成邀请。';
  }
  if (text.includes('administrator required')) {
    return '当前设备已加入，但没有管理员权限，无法生成邀请。请让设备组管理员生成一次性邀请。';
  }
  if (text.includes('invitation invalid, expired or used')) {
    return '邀请无效、已过期或已使用，请让管理员重新生成邀请。';
  }
  return text || '操作失败，请稍后重试。';
}
