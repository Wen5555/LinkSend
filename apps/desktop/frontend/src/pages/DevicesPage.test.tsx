// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ClipboardGrant, DeviceInfo } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { DevicesPage } from './DevicesPage';

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const backend = vi.hoisted(() => ({ ClipboardGrants: vi.fn(), SetClipboardGrant: vi.fn(), RemoveDevice: vi.fn(), UnblockDevice: vi.fn(), SaveDeviceProfile: vi.fn() }));
vi.mock('../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => backend);

const run: CommandRunner = async (_key, action, _message, onError) => {
  try { await action(); return true; }
  catch (cause) { await onError?.(cause); return false; }
};
const makeDevice = (id = 'peer', overrides: Partial<DeviceInfo> = {}): DeviceInfo => ({
  id, group_id: '', name: '另一台设备', public_key_hex: '', admin: false, online: true, trusted: true,
  always_accept: false, nearby: false, blocked: false, relationship: 'group_paired', service_state: 'membership_synced',
  lan_control_state: 'unavailable', connection_state: 'not_connected',
  profile: { peer_id: id, alias: '', my_device: false, pinned: false, position: 0, receive_directory: '', conflict_policy: '', last_used_at: '', revision: 1 },
  ...overrides,
});
const roots: Root[] = [];
function renderDevices(container: HTMLDivElement, devices: DeviceInfo[]) {
  const root = createRoot(container);
  roots.push(root);
  act(() => root.render(<DevicesPage devices={devices} identityID="self" membership={{ state: 'member', role: 'member', message: '' }} name="本机" run={run} op="" available />));
  return root;
}
function click(button: HTMLButtonElement | null) { expect(button).toBeTruthy(); act(() => button?.click()); }
async function settle() { await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); }); }
function setInputValue(input: HTMLInputElement, value: string) {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(input, value);
  input.dispatchEvent(new Event('input', { bubbles: true }));
}
function searchInput(container: HTMLElement) { return container.querySelector<HTMLInputElement>('input[aria-label="按名称、别名或身份查找设备"]')!; }
function openButton(container: HTMLElement) { return container.querySelector<HTMLButtonElement>('button[aria-label^="打开 "]'); }

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount());
  document.body.replaceChildren();
  vi.clearAllMocks();
});

describe('设备目录和详情', () => {
  it('keeps discovery, server presence and verified connections distinct as evidence changes', () => {
    const container = document.createElement('div'); document.body.append(container);
    const snapshot = makeDevice('status-peer', { online: false, nearby: true, service_state: 'unavailable', lan_control_state: 'discovered_unverified' });
    const root = renderDevices(container, [snapshot]);
    const status = () => container.querySelector('article[data-device-id="status-peer"]')!.textContent!;
    const update = (patch: Partial<DeviceInfo>) => act(() => root.render(<DevicesPage devices={[{ ...snapshot, ...patch }]} identityID="self" name="本机" run={run} op="" available />));
    expect(status()).toContain('已发现 · 连接待验证');
    expect(status()).not.toMatch(/局域网可达|已连接|离线/);
    update({ nearby: false, lan_control_state: 'not_seen' });
    expect(status()).toContain('连接状态待检查');
    expect(status()).not.toContain('离线');
    update({ nearby: false, lan_control_state: 'not_seen', service_state: 'membership_synced' });
    expect(status()).toContain('未见在线记录');
    update({ nearby: false, lan_control_state: 'not_seen', online: true, service_state: 'membership_synced' });
    expect(status()).toContain('在线 · 直连待检查');
    update({ connection_state: 'connected' });
    expect(status()).toContain('已连接');
    expect(status()).not.toContain('连接待验证');
  });

  it('does not turn a removed device into a reachable peer from stale discovery or session evidence', () => {
    const container = document.createElement('div'); document.body.append(container);
    renderDevices(container, [makeDevice('removed-peer', { trusted: false, blocked: true, relationship: 'removed', service_state: 'pending_revoke_sync', nearby: true, online: true, connection_state: 'connected' })]);
    const row = container.querySelector('article[data-device-id="removed-peer"]')!.textContent!;
    expect(row).toContain('待同步撤销');
    expect(row).toContain('已阻止连接');
    expect(row).not.toMatch(/局域网可达|已连接|直连待检查/);
  });

  it('confirms a pending revocation explicitly without allowing the device to rejoin', async () => {
    backend.RemoveDevice.mockResolvedValue(undefined);
    const pending = makeDevice('pending-target', { blocked: true, trusted: false, relationship: 'removed', service_state: 'pending_revoke_sync' });
    const container = document.createElement('div'); document.body.append(container);
    const root = renderDevices(container, [pending]);
    expect(container.textContent).toContain('待同步撤销');
    click(container.querySelector<HTMLButtonElement>('button[aria-label^="继续撤销 "]'));
    expect(backend.RemoveDevice).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(container.querySelector('button[aria-label^="取消继续撤销 "]'));
    click(container.querySelector<HTMLButtonElement>('button[aria-label^="取消继续撤销 "]'));
    expect(backend.RemoveDevice).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(container.querySelector('button[aria-label^="继续撤销 "]'));
    click(container.querySelector<HTMLButtonElement>('button[aria-label^="继续撤销 "]'));
    click(container.querySelector<HTMLButtonElement>('button[aria-label^="确认继续撤销 "]'));
    await settle();
    expect(backend.RemoveDevice).toHaveBeenCalledExactlyOnceWith(pending.id);
    expect(backend.UnblockDevice).not.toHaveBeenCalled();
    const synced = { ...pending, service_state: 'revoked' };
    act(() => root.render(<DevicesPage devices={[synced]} identityID="self" name="本机" run={run} op="" available />));
    expect(container.querySelector('button[aria-label^="继续撤销 "]')).toBeNull();
    expect(container.textContent).not.toContain('待同步撤销');
    expect(container.querySelector('button[aria-label^="允许重新添加 "]')).toBeTruthy();
  });

  it('keeps a failed revocation pending and clears stale confirmation when the status changes', async () => {
    backend.RemoveDevice.mockRejectedValue(new Error('SIGNALING_UNREACHABLE'));
    const pending = makeDevice('pending-target', { blocked: true, trusted: false, relationship: 'removed', service_state: 'pending_revoke_sync' });
    const container = document.createElement('div'); document.body.append(container);
    const root = renderDevices(container, [pending]);
    click(container.querySelector<HTMLButtonElement>('button[aria-label^="继续撤销 "]'));
    click(container.querySelector<HTMLButtonElement>('button[aria-label^="确认继续撤销 "]'));
    await settle();
    expect(backend.RemoveDevice).toHaveBeenCalledTimes(1);
    expect(backend.UnblockDevice).not.toHaveBeenCalled();
    expect(container.querySelector('button[aria-label^="确认继续撤销 "]')).toBeTruthy();
    act(() => root.render(<DevicesPage devices={[{ ...pending, service_state: 'revoked' }]} identityID="self" name="本机" run={run} op="" available />));
    act(() => root.render(<DevicesPage devices={[pending]} identityID="self" name="本机" run={run} op="" available />));
    expect(container.querySelector('button[aria-label^="确认继续撤销 "]')).toBeNull();
    expect(container.querySelector('button[aria-label^="继续撤销 "]')).toBeTruthy();
  });

  it.each([{ available: false, op: '' }, { available: true, op: 'another-operation' }])('disables pending revocation when the backend is unavailable or busy: %j', state => {
    const pending = makeDevice('pending-target', { blocked: true, trusted: false, relationship: 'removed', service_state: 'pending_revoke_sync' });
    const container = document.createElement('div'); document.body.append(container);
    const root = renderDevices(container, [pending]);
    act(() => root.render(<DevicesPage devices={[pending]} identityID="self" name="本机" run={run} {...state} />));
    const retry = container.querySelector<HTMLButtonElement>('button[aria-label^="继续撤销 "]');
    expect(retry?.disabled).toBe(true);
    click(retry);
    expect(container.querySelector('button[aria-label^="确认继续撤销 "]')).toBeNull();
    act(() => root.render(<DevicesPage devices={[pending]} identityID="self" name="本机" run={run} op="" available />));
    click(container.querySelector<HTMLButtonElement>('button[aria-label^="继续撤销 "]'));
    act(() => root.render(<DevicesPage devices={[pending]} identityID="self" name="本机" run={run} {...state} />));
    const confirm = container.querySelector<HTMLButtonElement>('button[aria-label^="确认继续撤销 "]');
    expect(confirm?.disabled).toBe(true);
    click(confirm);
    expect(backend.RemoveDevice).not.toHaveBeenCalled();
  });

  it.each([{ group_id: 'new-group' }, { incarnation: 'new-incarnation' }])('discards confirmation when the same identity changes its membership: %j', membership => {
    const pending = makeDevice('pending-target', { blocked: true, trusted: false, relationship: 'removed', service_state: 'pending_revoke_sync' });
    const container = document.createElement('div'); document.body.append(container);
    const root = renderDevices(container, [pending]);
    click(container.querySelector<HTMLButtonElement>('button[aria-label^="继续撤销 "]'));
    expect(container.querySelector('button[aria-label^="确认继续撤销 "]')).toBeTruthy();
    act(() => root.render(<DevicesPage devices={[{ ...pending, ...membership }]} identityID="self" name="本机" run={run} op="" available />));
    expect(container.querySelector('button[aria-label^="确认继续撤销 "]')).toBeNull();
    expect(container.querySelector('button[aria-label^="继续撤销 "]')).toBeTruthy();
    expect(backend.RemoveDevice).not.toHaveBeenCalled();
  });

  it('keeps a large paired directory to ten DOM rows and removes the exact searched identity', async () => {
    backend.ClipboardGrants.mockResolvedValue([]);
    backend.RemoveDevice.mockResolvedValue(undefined);
    const devices = Array.from({ length: 25 }, (_, index) => makeDevice(`peer-${index.toString().padStart(2, '0')}`, { name: '同名设备' }));
    const target = devices[24];
    const container = document.createElement('div'); document.body.append(container); renderDevices(container, devices);
    expect(container.querySelectorAll('article[data-device-id]')).toHaveLength(10);
    const input = searchInput(container);
    act(() => setInputValue(input, target.id));
    expect(container.querySelectorAll('article[data-device-id]')).toHaveLength(1);
    expect(container.querySelector('article[data-device-id="peer-24"]')).toBeTruthy();
    click(openButton(container)); await settle();
    click(container.querySelector<HTMLButtonElement>('button[aria-label^="删除 同名设备"]'));
    click(container.querySelector<HTMLButtonElement>('button[aria-label^="确认删除 同名设备"]'));
    await settle();
    expect(backend.RemoveDevice).toHaveBeenCalledWith(target.id);
  });

  it('clamps a disappearing final page and returns focus to search after the selected device vanishes', async () => {
    backend.ClipboardGrants.mockResolvedValue([]);
    const devices = Array.from({ length: 11 }, (_, index) => makeDevice(`peer-${index}`));
    const container = document.createElement('div'); document.body.append(container);
    const root = renderDevices(container, devices);
    click(container.querySelector<HTMLButtonElement>('button[aria-label="已配对设备下一页"]'));
    expect(container.querySelectorAll('article[data-device-id]')).toHaveLength(1);
    click(openButton(container)); await settle();
    act(() => root.render(<DevicesPage devices={devices.slice(0, 10)} identityID="self" membership={{ state: 'member', role: 'member', message: '' }} name="本机" run={run} op="" available />));
    await settle();
    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(container.textContent).toContain('第 1 / 1 页');
    expect(document.activeElement).toBe(searchInput(container));
  });

  it('keeps dirty input for the same identity and never reuses it after opening another device', async () => {
    backend.ClipboardGrants.mockResolvedValue([]);
    const first = makeDevice('first', { profile: { ...makeDevice('first').profile, alias: '初始别名' } });
    const second = makeDevice('second', { profile: { ...makeDevice('second').profile, alias: '第二台设备' } });
    const container = document.createElement('div'); document.body.append(container);
    const root = renderDevices(container, [first]);
    click(openButton(container)); await settle();
    const alias = container.querySelector<HTMLInputElement>('input[placeholder="另一台设备"]')!;
    act(() => setInputValue(alias, '尚未保存的别名'));
    const updatedFirst = { ...first, profile: { ...first.profile, alias: '后台新别名', revision: 2 } };
    act(() => root.render(<DevicesPage devices={[updatedFirst]} identityID="self" membership={{ state: 'member', role: 'member', message: '' }} name="本机" run={run} op="" available />));
    expect(container.querySelector<HTMLInputElement>('input[placeholder="另一台设备"]')?.value).toBe('尚未保存的别名');
    act(() => root.render(<DevicesPage devices={[second]} identityID="self" membership={{ state: 'member', role: 'member', message: '' }} name="本机" run={run} op="" available />));
    await settle(); click(openButton(container)); await settle();
    expect(container.querySelector<HTMLInputElement>('input[placeholder="另一台设备"]')?.value).toBe('第二台设备');
    expect(backend.ClipboardGrants).toHaveBeenCalledWith('second');
  });

  it('closes on Escape and on the close button without invoking a device command, restoring trigger focus', async () => {
    backend.ClipboardGrants.mockResolvedValue([]);
    const container = document.createElement('div'); document.body.append(container); renderDevices(container, [makeDevice()]);
    const trigger = openButton(container)!;
    click(trigger); await settle();
    act(() => container.querySelector<HTMLElement>('[role="dialog"]')?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })));
    await settle();
    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe(trigger);
    expect(backend.RemoveDevice).not.toHaveBeenCalled();
    click(trigger); await settle(); click(container.querySelector<HTMLButtonElement>('button[aria-label^="关闭 "]')); await settle();
    expect(document.activeElement).toBe(trigger);
  });

  it('keeps Tab inside the modal drawer in both directions', async () => {
    backend.ClipboardGrants.mockResolvedValue([]);
    const container = document.createElement('div'); document.body.append(container); renderDevices(container, [makeDevice()]);
    click(openButton(container)); await settle();
    const dialog = container.querySelector<HTMLElement>('[role="dialog"]')!;
    const controls = [...dialog.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])')];
    const first = controls[0], last = controls.at(-1)!;
    last.focus();
    act(() => last.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true })));
    expect(document.activeElement).toBe(first);
    first.focus();
    act(() => first.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true })));
    expect(document.activeElement).toBe(last);
  });

  it('disables profile fields while saving and preserves a newer saved revision over an old snapshot', async () => {
    backend.ClipboardGrants.mockResolvedValue([]);
    let resolveSave: (profile: DeviceInfo['profile']) => void = () => undefined;
    backend.SaveDeviceProfile.mockImplementation(() => new Promise<DeviceInfo['profile']>(resolve => { resolveSave = resolve; }));
    const initial = makeDevice('saved');
    const container = document.createElement('div'); document.body.append(container);
    const root = renderDevices(container, [initial]);
    click(openButton(container)); await settle();
    const alias = container.querySelector<HTMLInputElement>('input[placeholder="另一台设备"]')!;
    act(() => setInputValue(alias, '准备保存'));
    const save = [...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === '保存设备偏好') ?? null;
    expect(save?.disabled).toBe(false);
    click(save);
    await settle();
    expect(backend.SaveDeviceProfile).toHaveBeenCalledOnce();
    expect(container.querySelector<HTMLFieldSetElement>('.device-profile-fields')?.disabled).toBe(true);
    await act(async () => { resolveSave({ ...initial.profile, alias: '已保存别名', revision: 2 }); await Promise.resolve(); });
    await settle();
    act(() => root.render(<DevicesPage devices={[initial]} identityID="self" membership={{ state: 'member', role: 'member', message: '' }} name="本机" run={run} op="" available />));
    expect(container.querySelector<HTMLInputElement>('input[placeholder="另一台设备"]')?.value).toBe('已保存别名');
  });
});

describe('device clipboard grants', () => {
  it('keeps all six grants off by default and saves one direction and kind with revision CAS', async () => {
    backend.ClipboardGrants.mockResolvedValue([]);
    const saved: ClipboardGrant = { peer_id: 'peer', direction: 'send', kind: 'text', enabled: true, revision: 1, updated_at: '', authorization_generation: 3 };
    backend.SetClipboardGrant.mockResolvedValue(saved);
    const container = document.createElement('div'); document.body.append(container); renderDevices(container, [makeDevice()]);
    click(openButton(container)); await settle();
    const checkboxes = [...container.querySelectorAll('.clipboard-grant-grid input')] as HTMLInputElement[];
    expect(checkboxes).toHaveLength(6);
    expect(checkboxes.every(input => !input.checked)).toBe(true);
    expect(container.textContent).toContain('离线、锁屏或暂停期间的内容不会补发');
    await act(async () => { checkboxes[0].click(); await Promise.resolve(); });
    expect(backend.SetClipboardGrant).toHaveBeenCalledWith({ peer_id: 'peer', direction: 'send', kind: 'text', enabled: true, expected_revision: 0 });
    expect(checkboxes[0].checked).toBe(true);
  });
});
