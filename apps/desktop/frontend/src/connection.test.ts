import { describe, expect, it } from 'vitest';
import { connectionMethodLabel, humanizeBackendError, mergeTaskSnapshots, taskPhaseLabel } from './connection';

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
  it('localizes stable task phases and preserves unknown diagnostics', () => {
    expect(taskPhaseLabel('awaiting_acceptance')).toBe('等待接收确认');
    expect(taskPhaseLabel('CHECKING')).toBe('检查直连路径');
    expect(taskPhaseLabel('future_phase')).toBe('future_phase');
  });
  it('maps transport and task errors without exposing raw internals', () => {
    expect(humanizeBackendError('DISK_FULL: no space left on device')).toContain('空间不足');
    expect(humanizeBackendError('TASK_NOT_RETRYABLE')).toContain('无法重试');
    expect(humanizeBackendError('RELAY_NOT_IMPLEMENTED')).toContain('暂未实现中继');
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
