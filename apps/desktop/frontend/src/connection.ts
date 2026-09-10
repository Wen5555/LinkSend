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

/** Stable task phases are intentionally mapped here instead of exposing the
 * internal enum names returned by the Go coordinator. Unknown future phases
 * remain readable and are kept visible for diagnostics. */
export function taskPhaseLabel(phase: string): string {
  const labels: Record<string, string> = {
    preparing: '准备中',
    gathering: '发现网络地址',
    signaling: '交换连接信息',
    checking: '检查直连路径',
    nominating: '选择直连路径',
    authenticating: '验证设备身份',
    waiting: '等待接收请求',
    connecting: '建立安全直连',
    connected: '已建立直连',
    awaiting_acceptance: '等待接收确认',
    transferring: '传输中',
    verifying: '校验文件',
    cancelling: '正在取消',
    recovering: '恢复传输',
    completed: '已完成',
    failed: '失败',
  };
  return labels[phase.toLowerCase()] ?? phase;
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
    INVALID_ARGUMENT: '输入不完整，请选择设备、文件和接收目录后重试。',
    DESKTOP_NOT_READY: '桌面窗口尚未准备好，请稍后重试。',
    BACKEND_UNAVAILABLE: '桌面服务尚未启动完成，请稍后重试。',
    NO_CANDIDATES: '没有发现可用的直连地址，请检查网卡和绑定地址。',
    NO_VIABLE_CANDIDATE: '没有找到可用的直连路径，请检查 UDP 防火墙和网络。',
    CHECK_TIMEOUT: '直连检查超时，请确认双方在线并允许 UDP 通信。',
    ICE_FAILED: '直连检查失败，请检查绑定地址和网络后重试。',
    CANDIDATE_EXCHANGE_TIMEOUT: '连接信息交换超时，请检查信令服务和网络。',
    QUIC_HANDSHAKE_TIMEOUT: '安全连接握手超时，请确认双方指纹一致后重试。',
    QUIC_HANDSHAKE_FAILED: '安全连接握手失败，请确认双方版本和指纹一致。',
    RECEIVE_REJECTED: '接收方拒绝了本次传输。',
    SOURCE_CHANGED: '源文件发生变化，请重新选择文件后重试。',
    INTEGRITY_FAILED: '文件完整性校验失败，请重试并检查磁盘或网络。',
    DISK_FULL: '接收目录空间不足或不可写，请选择其他目录。',
    UNSAFE_PATH: '目标路径不安全，请选择其他接收目录。',
    CANCELLED: '传输已取消。',
    RELAY_NOT_IMPLEMENTED: '当前网络需要中继，但 LinkSend 暂未实现中继。',
    INVALID_MESSAGE: '收到无效的连接信息，请刷新设备状态后重试。',
    REPLAY: '连接信息已过期，请重新发起传输。',
    RATE_LIMITED: '请求过于频繁，请稍后重试。',
    TASK_NOT_RETRYABLE: '该任务无法重试，请重新选择文件发起任务。',
    TASK_NOT_AWAITING_ACCEPTANCE: '该接收请求已处理或已过期。',
    TASK_DECISION_ALREADY_SET: '接收决定已提交，请等待任务更新。',
    TASK_CANCEL_ALREADY_REQUESTED: '取消请求已提交，请等待任务结束。',
    INVALID_TASK_DIRECTORY: '只能打开已完成接收任务的目录。',
    INVALID_DIRECTORY: '接收目录不存在或不可访问，请重新选择目录。',
  };
  if (code && byCode[code]) return byCode[code];
  return text || '操作失败，请稍后重试。';
}
