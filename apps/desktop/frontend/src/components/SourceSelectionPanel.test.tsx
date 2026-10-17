// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { DraftPreview, SendDraft } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { SourceSelectionPanel } from './SourceSelectionPanel';

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
const roots: Root[] = [];
afterEach(() => { for (const root of roots.splice(0)) act(() => root.unmount()); document.body.replaceChildren(); });
const draft: SendDraft = { id: 'main', paths: ['/project'], peer_id: 'peer', revision: 3, updated_at: '' };
const preview: DraftPreview = { revision: 3, files: 2, directories: 1, bytes: 1024, complete: true, excluded_files: 3, excluded_directories: 1, excluded_bytes: 2048, entries: [{ path: 'project/main.go', bytes: 1024, directory: false, excluded: false }, { path: 'project/.git', bytes: 0, directory: true, excluded: true }] };

function mount(value = draft) {
  const container = document.createElement('div'); document.body.append(container);
  const root = createRoot(container); roots.push(root);
  const save = vi.fn();
  const render = (next: SendDraft, stats = preview) => act(() => root.render(<SourceSelectionPanel draft={next} preview={stats} disabled={false} save={save} />));
  render(value);
  return { container, save, render };
}

describe('目录发送范围', () => {
  it('keeps custom exclusions when removing only the preset', () => {
    const { container, save } = mount({ ...draft, source_selection: { version: 1, preset: 'source', exclude: ['private/'] } });
    act(() => { const select = container.querySelector('select')!; select.value = 'all'; select.dispatchEvent(new Event('change', { bubbles: true })); });
    expect(save).toHaveBeenCalledWith({ source_selection: { version: 1, preset: 'all', exclude: ['private/'] } });
  });
  it('defaults to complete contents and exposes actual include/exclude counts and entries', () => {
    const { container, save } = mount();
    expect(container.textContent).toContain('默认发送完整内容');
    expect(container.textContent).toContain('排除 3 个文件');
    expect(container.textContent).toContain('project/.git/');
    expect(save).not.toHaveBeenCalled();
    act(() => { const select = container.querySelector('select')!; select.value = 'source'; select.dispatchEvent(new Event('change', { bubbles: true })); });
    expect(save).toHaveBeenCalledWith({ source_selection: { version: 1, preset: 'source', exclude: [] } });
  });

  it('applies custom rules explicitly, preserves them across unrelated reads and supports reverting', () => {
    const value: SendDraft = { ...draft, source_selection: { version: 1, preset: 'source', exclude: ['*.tmp'] } };
    const { container, save, render } = mount(value);
    const editor = container.querySelector('textarea')!;
    act(() => { Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(editor, '缓存/\nsrc/**/*.tmp'); editor.dispatchEvent(new Event('input', { bubbles: true })); });
    expect(save).not.toHaveBeenCalled();
    render({ ...value, revision: 4 });
    expect(editor.value).toBe('缓存/\nsrc/**/*.tmp');
    expect(container.textContent).not.toContain('排除 3 个文件'); // Older preview is not evidence for a new revision.
    act(() => Array.from(container.querySelectorAll('button')).find(button => button.textContent === '应用排除规则')!.click());
    expect(save).toHaveBeenLastCalledWith({ source_selection: { version: 1, preset: 'source', exclude: ['缓存/', 'src/**/*.tmp'] } });
    act(() => Array.from(container.querySelectorAll('button')).find(button => button.textContent === '恢复完整发送')!.click());
    expect(save).toHaveBeenLastCalledWith({ source_selection: undefined });
  });
});
