// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, expect, it, vi } from 'vitest';
import { DiagnosticExport } from './DiagnosticExport';

const backend = vi.hoisted(() => ({ ExportDiagnostics: vi.fn() }));
vi.mock('../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => backend);
const roots: Root[] = [];
afterEach(() => { for (const root of roots.splice(0)) act(() => root.unmount()); document.body.replaceChildren(); vi.resetAllMocks(); });
async function render(available = true) {
  const element = document.createElement('div'); document.body.append(element);
  const root = createRoot(element); roots.push(root);
  await act(async () => root.render(<DiagnosticExport available={available} />));
  return element;
}

it('exports only after a user action, coalesces clicks, and reports actual saved/cancelled states', async () => {
  const element = await render();
  expect(backend.ExportDiagnostics).not.toHaveBeenCalled();
  let finish: (state: string) => void = () => {};
  backend.ExportDiagnostics.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
  const button = element.querySelector('button')!;
  await act(async () => { button.click(); button.click(); });
  expect(backend.ExportDiagnostics).toHaveBeenCalledOnce();
  expect(button.disabled).toBe(true);
  await act(async () => finish('cancelled'));
  expect(element.textContent).toContain('已取消导出');
  expect(element.textContent).not.toContain('诊断已保存');
  backend.ExportDiagnostics.mockResolvedValue('saved');
  await act(async () => button.click());
  expect(element.textContent).toContain('诊断已保存到所选位置');
});

it('never displays a raw native exception or enables exports without a desktop bridge', async () => {
  const element = await render();
  backend.ExportDiagnostics.mockRejectedValue(new Error('fake private C:\\Secret\\token.txt api-key-should-not-render'));
  await act(async () => element.querySelector('button')!.click());
  expect(element.textContent).toContain('无法导出诊断');
  expect(element.textContent).not.toContain('Secret');
  expect(element.textContent).not.toContain('api-key');
  const offline = await render(false);
  expect(offline.querySelector('button')!.disabled).toBe(true);
});
