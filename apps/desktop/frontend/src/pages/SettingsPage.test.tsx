// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { DesktopPreferences } from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { SettingsPage } from './SettingsPage';

const backend = vi.hoisted(() => ({
  Preferences: vi.fn(),
  SavePreferencesSection: vi.fn(),
  PickDirectory: vi.fn(),
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

function renderSettings() {
  const container = document.createElement('div');
  document.body.append(container);
  const root = createRoot(container);
  act(() => root.render(<SettingsPage preferences={basePreferences} interfaces={[]} run={run} controlRun={run} op="" available />));
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
});
