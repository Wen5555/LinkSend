// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, expect, it, vi } from 'vitest';
import type { TaskSnapshot, DiagnosticTask } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { DiagnosticNavigation, TaskDiagnostics } from './TaskDiagnostics';

const roots: Root[] = [];
afterEach(() => { for (const root of roots.splice(0)) act(() => root.unmount()); document.body.replaceChildren(); });
const diagnostic: DiagnosticTask = { product_version: '0.6.2', label: 'task', direction: 'send', state: 'failed', method: 'unknown', last_attempt: '', can_retry: false, can_resume: false,
  guidance: { code: 'CHECK_TIMEOUT', stage: 'ice', reason: '直连探测超时', next_step: '检查两端网络是否允许UDP。', action: 'open_network' } };

it('uses Go guidance and navigates to an actionable setting without bypassing retry controls', async () => {
  const element = document.createElement('div'); document.body.append(element);
  const root = createRoot(element); roots.push(root);
  const openSettings = vi.fn(); const openDevices = vi.fn();
  const task = { state: 'failed', error_message: 'C:\\Secret\\private-token', error_code: 'CHECK_TIMEOUT', diagnostic } as TaskSnapshot;
  await act(async () => root.render(<DiagnosticNavigation.Provider value={{ openSettings, openDevices }}><TaskDiagnostics task={task} /></DiagnosticNavigation.Provider>));
  expect(element.textContent).toContain('直连探测超时');
  expect(element.textContent).toContain('检查两端网络是否允许UDP');
  expect(element.textContent).toContain('未知');
  expect(element.textContent).not.toContain('Secret');
  expect(element.textContent).not.toContain('private-token');
  expect(element.textContent).not.toContain('重试');
  await act(async () => element.querySelector('button')!.click());
  expect(openSettings).toHaveBeenCalledWith('network');
  expect(openDevices).not.toHaveBeenCalled();
  await act(async () => root.render(<TaskDiagnostics task={{ ...task, state: 'completed', diagnostic: { ...diagnostic, guidance: { code: 'NONE', stage: 'completed', reason: '传输已完成', next_step: '', action: 'none' } } }} />));
  expect(element.textContent).not.toContain('直连探测超时');
});
