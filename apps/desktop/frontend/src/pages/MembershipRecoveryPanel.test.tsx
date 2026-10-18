// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { CommandRunner } from '../hooks/useDesktop';
import { MembershipRecoveryPanel } from './MembershipRecoveryPanel';

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
const backend = vi.hoisted(() => ({ MembershipRecoveryStatus: vi.fn(), PrepareMembershipRecovery: vi.fn(), ContinueMembershipRecovery: vi.fn(), CancelMembershipRecovery: vi.fn() }));
vi.mock('../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => backend);
const run: CommandRunner = async (_key, action, _message, onError) => {
  try { await action(); return true; }
  catch (cause) { await onError?.(cause); return false; }
};
const roots: Root[] = [];
function mount(available = true) {
  const container = document.createElement('div'); document.body.append(container);
  const root = createRoot(container); roots.push(root);
  act(() => root.render(<MembershipRecoveryPanel available={available} membershipState="auth_failed" name="本机测试" run={run} op="" />));
  return container;
}
function button(container: HTMLElement, label: string) {
  const result = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === label);
  expect(result).toBeTruthy(); return result!;
}
function click(element: HTMLElement) { act(() => element.click()); }
async function settle() { await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); }); }
beforeEach(() => { vi.resetAllMocks(); backend.MembershipRecoveryStatus.mockResolvedValue({ state: 'idle' }); });
afterEach(() => { for (const root of roots.splice(0)) act(() => root.unmount()); document.body.replaceChildren(); });

describe('显式设备组恢复', () => {
  it.each([
    ['prepared', 'abcd efgh', 'ABCD-EFGH'],
    ['prepared', 'aBcDeFgH'.repeat(5) + '_-A', 'aBcDeFgH'.repeat(5) + '_-A'],
    ['started', 'aBcDeFgH'.repeat(5) + '_-A', 'aBcDeFgH'.repeat(5) + '_-A'],
  ])('preserves typed invitations and continues the same %s intent', async (state, typed, submitted) => {
    backend.MembershipRecoveryStatus.mockResolvedValue({ state, id: 'same-intent', mode: 'join' });
    backend.ContinueMembershipRecovery.mockRejectedValue(new Error('SIGNALING_UNREACHABLE'));
    const container = mount(); await settle();
    const input = container.querySelector<HTMLInputElement>('input.pairing-input')!;
    for (const character of typed) act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, input.value + character);
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    expect(input.value).toBe(typed);
    expect(backend.ContinueMembershipRecovery).not.toHaveBeenCalled();
    click(button(container, state === 'prepared' ? '确认连接' : '核对并继续恢复')); await settle();
    expect(backend.ContinueMembershipRecovery).toHaveBeenCalledExactlyOnceWith('same-intent', submitted, '本机测试');
    expect(input.value).toBe(typed);
    expect(backend.PrepareMembershipRecovery).not.toHaveBeenCalled();
  });

  it('requires explicit confirmation, then joins using the durable intent and clears the one-time code', async () => {
    const container = mount(); await settle();
    expect(backend.PrepareMembershipRecovery).not.toHaveBeenCalled();
    expect(backend.ContinueMembershipRecovery).not.toHaveBeenCalled();
    click(button(container, '服务重建后重新连接'));
    expect(button(container, '准备恢复').disabled).toBe(true);
    click(container.querySelector<HTMLInputElement>('input[type="checkbox"]')!);
    backend.PrepareMembershipRecovery.mockResolvedValue({ state: 'prepared', id: 'intent', mode: 'join', message: '旧任务已隔离' });
    click(button(container, '准备恢复')); await settle();
    expect(backend.PrepareMembershipRecovery).toHaveBeenCalledExactlyOnceWith('join', true);
    expect(button(container, '确认连接').disabled).toBe(true);
    const input = container.querySelector<HTMLInputElement>('input.pairing-input')!;
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'ABCD-EFGH');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    backend.ContinueMembershipRecovery.mockResolvedValue({ id: 'self' });
    backend.MembershipRecoveryStatus.mockResolvedValue({ state: 'completed', id: 'intent' });
    click(button(container, '确认连接')); await settle();
    expect(backend.ContinueMembershipRecovery).toHaveBeenCalledExactlyOnceWith('intent', 'ABCD-EFGH', '本机测试');
    expect(container.querySelector('input.pairing-input')).toBeNull();
    expect(container.textContent).toContain('上次已重新连接');
  });

  it('opens a prepared request after restart and cancels it without a remote registration', async () => {
    backend.MembershipRecoveryStatus.mockResolvedValue({ state: 'prepared', id: 'persisted', mode: 'initialize', message: '旧任务已隔离' });
    backend.CancelMembershipRecovery.mockResolvedValue({ state: 'idle' });
    const container = mount(); await settle();
    expect(container.textContent).toContain('在当前服务新建本机设备组');
    click(button(container, '取消准备')); await settle();
    expect(backend.CancelMembershipRecovery).toHaveBeenCalledExactlyOnceWith('persisted');
    expect(backend.ContinueMembershipRecovery).not.toHaveBeenCalled();
    expect(button(container, '准备恢复').disabled).toBe(true);
  });

  it('keeps a paused uncertain request and permits reconciliation without replaying a saved invitation', async () => {
    backend.MembershipRecoveryStatus.mockResolvedValue({ state: 'paused', id: 'uncertain', mode: 'join', message: '服务端结果待核对' });
    backend.ContinueMembershipRecovery.mockRejectedValue(new Error('SIGNALING_UNREACHABLE'));
    const container = mount(); await settle();
    expect(button(container, '核对并继续恢复').disabled).toBe(false);
    click(button(container, '核对并继续恢复')); await settle();
    expect(backend.ContinueMembershipRecovery).toHaveBeenCalledExactlyOnceWith('uncertain', '', '本机测试');
    expect(container.textContent).toContain('服务端结果待核对');
    expect(container.querySelector<HTMLInputElement>('input.pairing-input')!.value).toBe('');
    expect(backend.PrepareMembershipRecovery).not.toHaveBeenCalled();
  });

  it('does not prepare when status is unreadable or the native backend is unavailable', async () => {
    backend.MembershipRecoveryStatus.mockRejectedValue(new Error('policy unreadable'));
    const container = mount(); await settle();
    click(button(container, '服务重建后重新连接'));
    click(container.querySelector<HTMLInputElement>('input[type="checkbox"]')!);
    expect(container.querySelector('[role="alert"]')).toBeTruthy();
    expect(button(container, '准备恢复').disabled).toBe(true);
    expect(backend.PrepareMembershipRecovery).not.toHaveBeenCalled();
    backend.MembershipRecoveryStatus.mockClear();
    const offline = mount(false); await settle();
    click(button(offline, '服务重建后重新连接'));
    expect(button(offline, '准备恢复').disabled).toBe(true);
    expect(backend.MembershipRecoveryStatus).not.toHaveBeenCalled();
  });
});
