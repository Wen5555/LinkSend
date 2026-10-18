// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Status } from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/updater/models';
import { SoftwareUpdates } from './SoftwareUpdates';

const backend = vi.hoisted(() => ({ UpdateStatus: vi.fn(), CheckForUpdates: vi.fn(), DownloadUpdate: vi.fn(), CancelUpdate: vi.fn(), OpenUpdatePackage: vi.fn(), OpenUpdateReleasePage: vi.fn(), QuitApplication: vi.fn() }));
vi.mock('../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => backend);

const idle: Status = { current_version: '0.6.1', latest_version: '', platform: 'windows-amd64', stage: 'idle', message: '点击检查 GitHub 上的最新正式版', error: '', release_url: '', notes: '', package_name: '', package_bytes: 1000, downloaded_bytes: 0, last_checked: '', update_available: false, busy: false, can_download: false, can_open: false };
const roots: Root[] = [];
beforeEach(() => { vi.useFakeTimers(); backend.UpdateStatus.mockResolvedValue(idle); });
afterEach(() => { for (const root of roots.splice(0)) act(() => root.unmount()); document.body.replaceChildren(); vi.useRealTimers(); vi.resetAllMocks(); });

async function render(available = true) {
  const container = document.createElement('div'); document.body.append(container);
  const root = createRoot(container); roots.push(root);
  await act(async () => root.render(<SoftwareUpdates available={available} />));
  return container;
}
function button(container: HTMLElement, label: string) {
  const match = [...container.querySelectorAll('button')].find(item => item.textContent?.includes(label));
  if (!match) throw new Error(`missing button: ${label}`);
  return match;
}
async function poll() { await act(async () => vi.advanceTimersByTimeAsync(600)); }

describe('manual GitHub updater', () => {
  it('checks only on demand and downloads before offering a native installer', async () => {
    const container = await render();
    expect(backend.CheckForUpdates).not.toHaveBeenCalled();
    expect(container.textContent).toContain('0.6.1');
    backend.CheckForUpdates.mockResolvedValue({ ...idle, stage: 'checking', busy: true });
    backend.UpdateStatus.mockResolvedValue({ ...idle, stage: 'available', latest_version: '0.6.2', update_available: true, can_download: true, notes: '<script>plain text only</script>' });
    await act(async () => button(container, '检查更新').click());
    expect(button(container, '检查更新').disabled).toBe(true);
    await poll();
    expect(container.querySelector('script')).toBeNull();
    expect(container.textContent).toContain('<script>plain text only</script>');
    expect(backend.DownloadUpdate).not.toHaveBeenCalled();
    backend.DownloadUpdate.mockResolvedValue({ ...idle, stage: 'downloading', busy: true, downloaded_bytes: 430 });
    backend.UpdateStatus.mockResolvedValue({ ...idle, stage: 'ready', can_open: true });
    await act(async () => button(container, '下载更新').click());
    expect(container.querySelector('progress')?.value).toBe(430);
    expect(container.textContent).toContain('43%');
    await poll();
    expect(backend.OpenUpdatePackage).not.toHaveBeenCalled();
    backend.OpenUpdatePackage.mockResolvedValue({ ...idle, stage: 'opened', can_open: true });
    await act(async () => button(container, '打开安装包').click());
    expect(container.textContent).toContain('包括托盘后台');
    backend.QuitApplication.mockResolvedValue(undefined);
    await act(async () => button(container, '退出 LinkSend').click());
    expect(backend.QuitApplication).toHaveBeenCalledOnce();
  });

  it('recovers an ongoing download when returning to the page and supports cancellation', async () => {
    backend.UpdateStatus.mockResolvedValue({ ...idle, stage: 'downloading', busy: true, downloaded_bytes: 500 });
    const container = await render();
    expect(container.textContent).toContain('50%');
    backend.CancelUpdate.mockResolvedValue({ ...idle, stage: 'cancelling', busy: true });
    backend.UpdateStatus.mockResolvedValue({ ...idle, stage: 'cancelled', can_download: true, message: '操作已取消，可以重试' });
    await act(async () => button(container, '取消').click());
    expect(button(container, '取消').disabled).toBe(true);
    await poll();
    expect(button(container, '下载更新').disabled).toBe(false);
    expect(backend.CheckForUpdates).not.toHaveBeenCalled();
  });

  it('never offers to open a package that failed verification', async () => {
    backend.UpdateStatus.mockResolvedValue({ ...idle, stage: 'error', can_download: true, error: '安装包 SHA256 校验失败，请重新下载' });
    const container = await render();
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('SHA256');
    expect([...container.querySelectorAll('button')].some(item => item.textContent === '打开安装包')).toBe(false);
    expect(button(container, '下载更新').disabled).toBe(false);
  });

  it('does not call the desktop bridge in a plain browser', async () => {
    const container = await render(false);
    expect(backend.UpdateStatus).not.toHaveBeenCalled();
    expect(button(container, '检查更新').disabled).toBe(true);
  });
});
