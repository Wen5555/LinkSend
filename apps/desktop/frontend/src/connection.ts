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
    waiting_peer: '等待对方重新上线',
    connected: '已建立直连',
    awaiting_acceptance: '等待接收确认',
    transferring: '传输中',
    verifying: '校验文件',
	pausing: '正在暂停',
	paused: '已暂停',
    cancelling: '正在取消',
    recovering: '恢复传输',
	connection_interrupted: '连接已中断',
    completed: '已完成',
    failed: '失败',
  };
  return labels[phase.toLowerCase()] ?? phase;
}

export type RevisionedTask = { id: string; task_id?: string; revision: number };

// A delayed poll must never replace a newer action response or task snapshot.
// Missing entries are retained because history is append-only in V1.
export function mergeTaskSnapshots<T extends RevisionedTask>(current: T[], incoming: T[]): T[] {
  const merged = [...current];
  const indexes = new Map<string, number>();
  merged.forEach((task, index) => indexes.set(task.task_id || task.id, index));
  for (const task of incoming) {
    const key = task.task_id || task.id;
    const index = indexes.get(key);
    if (index == null) {
      indexes.set(key, merged.length);
      merged.push(task);
      continue;
    }
    if (task.revision >= merged[index].revision) merged[index] = task;
  }
  return merged;
}

export function humanizeBackendError(raw: unknown): string {
  const text = String(raw ?? '').replace(/^Error:\s*/, '');
  if (text.includes('request signature or membership invalid')) {
    return '当前设备尚未完成配对，或已被移除。请在另一台设备上生成新的配对码。';
  }
  if (text.includes('paired member required')) {
    return '当前设备尚未完成配对。请先使用已加入且未撤销的设备生成一次性配对码。';
  }
  if (text.includes('administrator required')) {
    return '服务端仍要求管理员生成邀请，可能与当前客户端权限模型不兼容；请升级服务端后重试。';
  }
  if (text.includes('invitation invalid, expired or used')) {
    return '配对码无效、已过期或已使用，请在另一台设备上重新生成。';
  }
  const code = text.match(/\b[A-Z][A-Z0-9_]{2,}\b/)?.[0];
  const byCode: Record<string, string> = {
    SIGNALING_UNREACHABLE: '无法连接信令服务，请检查服务地址和网络后重试。',
    SIGNALING_TIMEOUT: '信令服务响应超时，请检查网络后重试。',
    UNPAIRED: '设备尚未完成配对，请在设备页输入配对码。',
    AUTHENTICATION_FAILED: '设备认证失败，请重新生成配对码完成配对。',
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
    QUIC_HANDSHAKE_TIMEOUT: '安全连接握手超时，请确认双方在线后重试。',
    QUIC_HANDSHAKE_FAILED: '安全连接握手失败，请确认双方版本兼容；如设备密钥已变化请重新配对。',
    RECEIVE_REJECTED: '接收方拒绝了本次传输。',
    SOURCE_CHANGED: '源文件发生变化，请重新选择文件后重试。',
    INTEGRITY_FAILED: '文件完整性校验失败，请重试并检查磁盘或网络。',
    DISK_FULL: '接收磁盘空间不足，请清理空间后重试。',
    PERMISSION_DENIED: '接收目录没有写入权限，请选择可写目录。',
    FILE_CONFLICT: '目标文件已存在且不会覆盖，请选择空目录后重试。',
    TASK_INTERRUPTED: '上次任务因应用退出而中断，请核对源文件和接收目录后重新发起。',
    PEER_OFFLINE: '对端当前离线，请让对端保持 LinkSend 运行。',
    VERSION_INCOMPATIBLE: '双方版本或传输能力不兼容，请升级到兼容版本。',
    DIRECT_FAILED: '直连或传输未完成，请检查设备在线、网络和接收目录后重试。',
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
	TASK_NOT_PAUSABLE: '当前阶段不能暂停；校验或提交已开始时请等待结果。',
	TASK_NOT_RESUMABLE: '该任务没有可验证的恢复数据，请重新发起。',
	CONNECTION_INTERRUPTED: '连接已中断，已验证数据仍保留；请让双方确认后恢复。',
	RESUME_IDENTITY_MISMATCH: '恢复身份不匹配，已停止传输；请核对源文件、目录和已配对设备。',
	SESSION_CONFLICT: '双方同时发起了连接，正在合并为唯一会话；如未继续请重试。',
    INVALID_TASK_DIRECTORY: '只能打开已完成接收任务的目录。',
    INVALID_DIRECTORY: '接收目录不存在或不可访问，请重新选择目录。',
    INBOX_STOP_TIMEOUT: '后台接收切换超时，请重启 LinkSend 后重试。',
  };
  if (code && byCode[code]) return byCode[code];
  return text || '操作失败，请稍后重试。';
}
