// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ClipboardGrant, DeviceInfo } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { DevicesPage } from './DevicesPage';

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const backend = vi.hoisted(() => ({ ClipboardGrants: vi.fn(), SetClipboardGrant: vi.fn(), RemoveDevice: vi.fn(), SaveDeviceProfile: vi.fn() }));
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
