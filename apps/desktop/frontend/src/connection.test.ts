import { describe, expect, it } from 'vitest';
import { connectionMethodLabel, formatPairingCodeInput, humanizeBackendError, mergeTaskSnapshots, shouldClearPairingCode, taskPhaseLabel } from './connection';

describe('connection evidence labels', () => {
  it('does not infer LAN from an ICE host candidate or online presence', () => {
    for (const unproven of ['host', 'online', 'direct_unknown', '', 'unexpected']) {
      expect(connectionMethodLabel(unproven)).toBe('直连 · 路径待确认');
    }
  });
  it('does not advertise a relay implementation', () => {
    expect(connectionMethodLabel('relay')).toBe('尚未实现');
  });
  it('uses a proven connection method when available', () => {
    expect(connectionMethodLabel('lan_direct')).toBe('局域网直连');
    expect(connectionMethodLabel('internet_p2p')).toBe('互联网 P2P');
  });
  it('explains membership errors in user-facing language', () => {
    expect(humanizeBackendError('signaling HTTP 401: AUTHENTICATION_FAILED: request signature or membership invalid')).toContain('尚未完成配对');
    expect(humanizeBackendError('AUTHENTICATION_FAILED: paired member required')).toContain('尚未完成配对');
    expect(humanizeBackendError('AUTHENTICATION_FAILED: administrator required')).toContain('权限模型不兼容');
  });
  it('formats current pairing codes without changing legacy invitations', () => {
    expect(formatPairingCodeInput('abcd efgh')).toBe('ABCD-EFGH');
    expect(formatPairingCodeInput('abcd–efgh')).toBe('ABCD-EFGH');
    const legacy = 'AbCd_legacy-token-with-43-characters-123456';
    expect(formatPairingCodeInput(legacy)).toBe(legacy.slice(0, 43));
  });
  it('distinguishes pairing failures and clears only terminal codes', () => {
    expect(humanizeBackendError('PAIRING_CODE_EXPIRED')).toContain('已过期');
    expect(humanizeBackendError('PAIRING_CODE_USED')).toContain('已被其他设备使用');
    expect(humanizeBackendError('PAIRING_CODE_INVALID')).toContain('格式不正确');
    expect(shouldClearPairingCode('PAIRING_CODE_EXPIRED')).toBe(true);
    expect(shouldClearPairingCode('PAIRING_CODE_INVALID')).toBe(false);
  });
  it('localizes stable task phases and preserves unknown diagnostics', () => {
    expect(taskPhaseLabel('awaiting_acceptance')).toBe('等待接收确认');
    expect(taskPhaseLabel('CHECKING')).toBe('检查直连路径');
	expect(taskPhaseLabel('endpoint_setup')).toBe('自动选择本机网络');
	expect(taskPhaseLabel('quic_handshake')).toBe('建立加密传输通道');
    expect(taskPhaseLabel('lan_control_connect')).toBe('连接局域网设备');
    expect(taskPhaseLabel('future_phase')).toBe('future_phase');
  });
  it('maps transport and task errors without exposing raw internals', () => {
    expect(humanizeBackendError('DISK_FULL: no space left on device')).toContain('空间不足');
    expect(humanizeBackendError('TASK_NOT_RETRYABLE')).toContain('无法重试');
    expect(humanizeBackendError('RELAY_NOT_IMPLEMENTED')).toContain('暂未实现中继');
  });
  it('localizes content failures in the shared command error banner', () => {
    const unavailable = new Error('CLIPBOARD_IMAGE_NOT_AVAILABLE');
    unavailable.name = 'RuntimeError';
    expect(humanizeBackendError(unavailable)).toBe('剪贴板中没有可用图片，请先复制图片再读取。');
    expect(humanizeBackendError('RuntimeError: CLIPBOARD_WRITE_FAILED')).toContain('未能写入剪贴板');
    expect(humanizeBackendError('RuntimeError: CONTENT_LIMIT_EXCEEDED')).toContain('尺寸限制');
    expect(humanizeBackendError('RuntimeError: CONTENT_CAPABILITY_REQUIRED')).toContain('按文件发送');
    expect(humanizeBackendError('RuntimeError: CONTENT_SNAPSHOT_CHANGED: private snapshot path')).not.toContain('private');
  });
  it('provides actionable LAN discovery diagnostics', () => {
    expect(humanizeBackendError('LAN_CONTROL_UNREACHABLE')).toContain('防火墙');
    expect(humanizeBackendError('LAN_DISCOVERY_UNAVAILABLE')).toContain('UDP 53318');
    expect(humanizeBackendError('LAN_PROBE_INVALID')).toContain('同一局域网');
  });
  it('distinguishes permission, conflict, interruption and capacity failures', () => {
    expect(humanizeBackendError('PERMISSION_DENIED: /private/receive')).toContain('写入权限');
    expect(humanizeBackendError('FILE_CONFLICT: /private/receive')).toContain('不会覆盖');
    expect(humanizeBackendError('TASK_INTERRUPTED')).toContain('重新发起');
    expect(humanizeBackendError('DISK_FULL')).not.toContain('权限');
    expect(humanizeBackendError('PERMISSION_DENIED: /private/receive')).not.toContain('/private');
  });
  it('keeps the newest revision when an older poll arrives late', () => {
    const current = [{ id: 'task-1', revision: 8, state: 'paused' }];
    const stale = [{ id: 'task-1', revision: 7, state: 'transferring' }];
    expect(mergeTaskSnapshots(current, stale)).toEqual(current);
    expect(mergeTaskSnapshots(current, [{ id: 'task-1', revision: 9, state: 'recovering' }]))
      .toEqual([{ id: 'task-1', revision: 9, state: 'recovering' }]);
  });
});
