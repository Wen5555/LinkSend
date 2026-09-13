// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ClipboardGrant, DeviceInfo } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { DevicesPage } from './DevicesPage';

const backend = vi.hoisted(() => ({
  ClipboardGrants: vi.fn(),
  SetClipboardGrant: vi.fn(),
}));
vi.mock('../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => backend);

const run: CommandRunner = async (_key, action, _message, onError) => {
  try {
    await action();
    return true;
  } catch (cause) {
    await onError?.(cause);
    return false;
  }
};
const device: DeviceInfo = {
  id: 'peer', group_id: '', name: '另一台设备', public_key_hex: '', admin: false, online: true, trusted: true,
  always_accept: false, nearby: false, blocked: false, relationship: 'group_paired', service_state: 'membership_synced',
  lan_control_state: 'unavailable', connection_state: 'not_connected',
  profile: { peer_id: 'peer', alias: '', my_device: false, pinned: false, position: 0, receive_directory: '', conflict_policy: '', last_used_at: '', revision: 1 },
};
const roots: Root[] = [];

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount());
  document.body.replaceChildren();
  vi.clearAllMocks();
});

describe('device clipboard grants', () => {
  it('keeps all six grants off by default and saves one direction and kind with revision CAS', async () => {
    backend.ClipboardGrants.mockResolvedValue([]);
    const saved: ClipboardGrant = { peer_id: 'peer', direction: 'send', kind: 'text', enabled: true, revision: 1, updated_at: '', authorization_generation: 3 };
    backend.SetClipboardGrant.mockResolvedValue(saved);
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    roots.push(root);
    act(() => root.render(<DevicesPage devices={[device]} identityID="self" membership={{ state: 'member', role: 'member', message: '' }} name="本机" run={run} op="" available />));
    act(() => ([...container.querySelectorAll('button')].find(button => button.textContent === '设置') as HTMLButtonElement).click());
    await act(async () => { await Promise.resolve(); });
    const checkboxes = [...container.querySelectorAll('.clipboard-grant-grid input')] as HTMLInputElement[];
    expect(checkboxes).toHaveLength(6);
    expect(checkboxes.every(input => !input.checked)).toBe(true);
    expect(container.textContent).toContain('离线、锁屏或暂停期间的内容不会补发');
    await act(async () => { checkboxes[0].click(); await Promise.resolve(); });
    expect(backend.SetClipboardGrant).toHaveBeenCalledWith({ peer_id: 'peer', direction: 'send', kind: 'text', enabled: true, expected_revision: 0 });
    expect(checkboxes[0].checked).toBe(true);
  });
});
