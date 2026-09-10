import { describe, expect, it } from 'vitest';
import { connectionMethodLabel, humanizeBackendError, taskPhaseLabel } from './connection';

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
    expect(humanizeBackendError('signaling HTTP 401: AUTHENTICATION_FAILED: request signature or membership invalid')).toContain('尚未加入');
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
});
