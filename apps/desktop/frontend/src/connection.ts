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
    return '当前身份尚未加入或未通过这个设备组的成员验证。请先使用已加入且未撤销的设备生成一次性邀请，在“设备”页完成加入。';
  }
  if (text.includes('paired member required')) {
    return '当前设备尚未完成配对。请先使用已加入且未撤销的设备生成一次性配对码。';
  }
  if (text.includes('administrator required')) {
    return '服务端仍要求管理员生成邀请，可能与当前客户端权限模型不兼容；请升级服务端后重试。';
  }
  if (text.includes('invitation invalid, expired or used')) {
    return '邀请无效、已过期或已使用，请让管理员重新生成邀请。';
  }
  const code = text.match(/\b[A-Z][A-Z0-9_]{2,}\b/)?.[0];
  const byCode: Record<string, string> = {
    SIGNALING_UNREACHABLE: '无法连接信令服务，请检查服务地址和网络后重试。',
    SIGNALING_TIMEOUT: '信令服务响应超时，请检查网络后重试。',
    UNPAIRED: '设备尚未完成指纹信任，请在设备页核对完整指纹。',
    AUTHENTICATION_FAILED: '身份验证失败，请确认设备组成员资格和已保存指纹。',
    INVALID_CONFIG: '设置无效，请检查地址格式后重试。',
    CONFIG_BLOCKED: '本地偏好文件异常，请在设置页保存修复后的配置。',
    TASK_NOT_FOUND: '任务已不存在，请刷新任务列表。',
    TASK_TERMINAL: '任务已经结束，不能再执行该操作。',
    BUSY: '已有任务正在运行，请等待完成或先取消当前任务。',
  };
  if (code && byCode[code]) return byCode[code];
  return text || '操作失败，请稍后重试。';
}
