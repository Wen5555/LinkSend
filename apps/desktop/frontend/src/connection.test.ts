import { describe, expect, it } from 'vitest';
import { connectionMethodLabel } from './connection';

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
});
