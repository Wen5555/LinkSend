// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it, vi } from 'vitest';
import type { DeviceInfo } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { ClipboardDevice } from './ClipboardDevices';

const backend = vi.hoisted(() => ({ ClipboardGrants: vi.fn(), SetClipboardGrant: vi.fn() }));
vi.mock('../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => backend);
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

describe('simple clipboard device switch', () => {
  it('explicitly enables six scoped grants with current revisions and refreshes the result', async () => {
    backend.ClipboardGrants.mockResolvedValue([]);
    backend.SetClipboardGrant.mockResolvedValue({});
    const device = { id: 'peer', name: 'Mac', trusted: true, blocked: false, profile: {} } as DeviceInfo;
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
    const container = document.createElement('div'); document.body.append(container);
    const root = createRoot(container);
    try {
      await act(async () => { root.render(<QueryClientProvider client={client}><ClipboardDevice device={device} run={async (_key, action) => { await action(); return true; }} op="" available /></QueryClientProvider>); await new Promise(resolve => setTimeout(resolve, 20)); });
      await act(async () => { await new Promise(resolve => setTimeout(resolve, 20)); });
      const toggle = container.querySelector<HTMLInputElement>('input')!;
      expect(toggle.disabled).toBe(false);
      expect(backend.SetClipboardGrant).not.toHaveBeenCalled();
      await act(async () => { toggle.click(); await new Promise(resolve => setTimeout(resolve, 20)); });
      expect(backend.SetClipboardGrant).toHaveBeenCalledTimes(6);
      for (const direction of ['send', 'receive']) for (const kind of ['text', 'link', 'image']) expect(backend.SetClipboardGrant).toHaveBeenCalledWith({ peer_id: 'peer', direction, kind, enabled: true, expected_revision: 0 });
      expect(backend.ClipboardGrants.mock.calls.length).toBeGreaterThanOrEqual(3);
    } finally { act(() => root.unmount()); client.clear(); container.remove(); }
  });
});
