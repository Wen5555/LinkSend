// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ClipboardWatchStatus, DesktopPreferences } from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { SettingsPage, type SettingsCategoryRequest } from './SettingsPage';

const backend = vi.hoisted(() => ({
  Preferences: vi.fn(),
  SavePreferencesSection: vi.fn(),
  PickDirectory: vi.fn(),
  SetClipboardPaused: vi.fn(),
}));
vi.mock('../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => backend);

const basePreferences: DesktopPreferences = {
  format_version: 1,
  revision: 2,
  device_name: '原名称',
  receive_directory: 'C:\\receive',
  conflict_policy: 'keep_both',
  server_url: 'https://signal.example',
  bind_address: '',
  interface_priority: [],
  excluded_interfaces: [],
  stun_urls: [],
  background: { close_mode: '', notifications: false, prevent_sleep: false },
  clipboard_enabled: false,
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

const run: CommandRunner = async (_key, action, _message, onError) => {
  try {
    await action();
    return true;
  } catch (error) {
    await onError?.(error);
    return false;
  }
};

function renderSettings(clipboard?: ClipboardWatchStatus, categoryRequest?: SettingsCategoryRequest) {
  const container = document.createElement('div');
  document.body.append(container);
  const root = createRoot(container);
  act(() => root.render(<SettingsPage preferences={basePreferences} clipboard={clipboard} interfaces={[]} run={run} controlRun={run} op="" available initialCategory={categoryRequest?.category} categoryRequest={categoryRequest} />));
  return { container, root };
}

function change(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

function button(container: HTMLElement, label: string) {
  const match = [...container.querySelectorAll('button')].find(item => item.textContent?.includes(label));
  if (!match) throw new Error(`missing button: ${label}`);
  return match;
}

const roots: Root[] = [];
afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount());
  document.body.replaceChildren();
  vi.clearAllMocks();
});

describe('SettingsPage save lifecycle', () => {
  it('locks the submitted section until a deferred save resolves', async () => {
    const pending = deferred<DesktopPreferences>();
    backend.SavePreferencesSection.mockReturnValueOnce(pending.promise);
    const { container, root } = renderSettings();
    roots.push(root);
    const name = container.querySelector('input') as HTMLInputElement;
    act(() => change(name, '保存中的名称'));

    act(() => button(container, '保存通用设置').click());
    expect(name.disabled).toBe(true);
    expect(button(container, '文件接收').disabled).toBe(true);

    await act(async () => pending.resolve({ ...basePreferences, revision: 3, device_name: '保存中的名称' }));
    expect(name.disabled).toBe(false);
    expect(name.value).toBe('保存中的名称');
  });

  it('rebases a revision conflict before allowing an explicit retry', async () => {
    backend.SavePreferencesSection
      .mockRejectedValueOnce(new Error('PREFERENCES_REVISION_CONFLICT: stale revision'))
      .mockResolvedValueOnce({ ...basePreferences, revision: 6, device_name: '我的草稿', receive_directory: 'D:\\latest' });
    backend.Preferences.mockResolvedValueOnce({ ...basePreferences, revision: 5, device_name: '其他位置名称', receive_directory: 'D:\\latest' });
    const { container, root } = renderSettings();
    roots.push(root);
    const name = container.querySelector('input') as HTMLInputElement;
    act(() => change(name, '我的草稿'));

    await act(async () => button(container, '保存通用设置').click());
    expect(backend.Preferences).toHaveBeenCalledOnce();
    expect(name.value).toBe('我的草稿');
    expect(container.textContent).toContain('设置已在其他位置更新');

    act(() => button(container, '保留输入并合并最新设置').click());
    await act(async () => button(container, '保存通用设置').click());
    expect(backend.SavePreferencesSection).toHaveBeenLastCalledWith('general', 5, expect.objectContaining({ device_name: '我的草稿', receive_directory: 'D:\\latest' }));
  });

  it('keeps ordinary save errors out of the revision-merge flow', async () => {
    backend.SavePreferencesSection.mockRejectedValueOnce(new Error('DISK_IO: write failed'));
    const { container, root } = renderSettings();
    roots.push(root);
    act(() => button(container, '保存通用设置').click());
    await act(async () => Promise.resolve());
    expect(backend.Preferences).not.toHaveBeenCalled();
    expect(container.textContent).not.toContain('设置已在其他位置更新');
  });

  it('saves the clipboard master switch and renders peer capability state', async () => {
    backend.SavePreferencesSection.mockResolvedValueOnce({ ...basePreferences, revision: 3, clipboard_enabled: true });
	const { container, root } = renderSettings({ enabled: false, master_enabled: false, active: false, paused: true, pause_reason: 'master_disabled', last: { sequence: 0, text: false, link: false, image: false }, peers: [{ peer_id: 'peer-a', state: 'unsupported', send_ready: false, receive_ready: false }, { peer_id: 'peer-b', state: 'ready', send_ready: true, receive_ready: true, error: 'write_failed' }] });
    roots.push(root);
    act(() => button(container, '自动剪贴板').click());
    const toggle = container.querySelector('input[type="checkbox"]') as HTMLInputElement;
    act(() => toggle.click());
    expect(toggle.checked).toBe(true);
	expect(container.textContent).toContain('对端不支持');
	expect(container.textContent).toContain('写入系统剪贴板失败');
    await act(async () => button(container, '保存自动剪贴板设置').click());
    expect(backend.SavePreferencesSection).toHaveBeenCalledWith('clipboard', 2, expect.objectContaining({ clipboard_enabled: true }));
  });
});

describe('SettingsPage navigation requests', () => {
  it('opens receive settings for every explicit request without remounting local preferences state', () => {
    const { container, root } = renderSettings(undefined, { category: 'general', revision: 0 });
    roots.push(root);
    act(() => button(container, '网络与发现').click());
    expect(container.textContent).toContain('连接设置');
    act(() => root.render(<SettingsPage preferences={basePreferences} interfaces={[]} run={run} controlRun={run} op="" available initialCategory="receive" categoryRequest={{ category: 'receive', revision: 1 }} />));
    expect(container.textContent).toContain('目录与冲突');
    act(() => button(container, '网络与发现').click());
    expect(container.textContent).toContain('连接设置');
    act(() => root.render(<SettingsPage preferences={basePreferences} interfaces={[]} run={run} controlRun={run} op="" available initialCategory="receive" categoryRequest={{ category: 'receive', revision: 2 }} />));
    expect(container.textContent).toContain('目录与冲突');
    expect((container.querySelector('input[placeholder="请选择目录"]') as HTMLInputElement).value).toBe('C:\\receive');
  });
});
