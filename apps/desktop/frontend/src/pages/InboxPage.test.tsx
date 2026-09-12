import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it, vi } from 'vitest';
import type { InboxItem, InboxPage as PageData } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { InboxPage, InboxResendLedger, cleanupNotice, inboxError, inboxQuery } from './InboxPage';

const backend = vi.hoisted(() => ({ Inbox: vi.fn(), InboxFiles: vi.fn(), ResendInbox: vi.fn(), ForgetInboxRecords: vi.fn(), CleanupInboxStaging: vi.fn(), RevealInboxFile: vi.fn() }));
vi.mock('../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => backend);

const filters = { search: '', peer: '', direction: '', state: '', after: '', before: '' };
const item: InboxItem = { task_id: 'opaque-task', peer_id: 'peer', direction: 'receive', state: 'completed', summary: '中文 报告.txt', file_count: 1,
  verified_bytes: 42, committed_bytes: 42, bilateral_confirmed: true, started_at: '2026-09-12T05:00:00Z', ended_at: '2026-09-12T05:00:01Z', revision: 4, can_resend: false, can_resume: false, can_forget: true };

function markup(data?: PageData, options: { available?: boolean; focused?: boolean; filesAvailable?: boolean; skipped?: boolean } = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  if (data) client.setQueryData(['inbox', 'history', inboxQuery(filters)], data);
  if (options.focused) client.setQueryData(['inbox', 'files', item.task_id, ''], { available: options.filesAvailable ?? true, files: [{ file_id: 0, name: '<script>unsafe</script>.txt', kind: 'file', size: 42, selected: !options.skipped }], next_cursor: '' });
  const result = renderToStaticMarkup(createElement(QueryClientProvider, { client }, createElement(InboxPage, { devices: [], run: async () => true, op: '', available: options.available ?? true, focusTaskID: options.focused ? item.task_id : undefined })));
  client.clear();
  return result;
}

describe('InboxPage bounded and safe surfaces', () => {
  it('does not offer a mutation while the native backend is unavailable', () => {
    const html = markup(undefined, { available: false });
    expect(html).toContain('本机记录尚未就绪');
    expect(html).toMatch(/<button class="secondary" disabled="">清理孤立暂存<\/button>/);
    expect(backend.CleanupInboxStaging).not.toHaveBeenCalled();
    expect(backend.ForgetInboxRecords).not.toHaveBeenCalled();
  });
  it('renders only the bounded page and leaves the next-page action available', () => {
    const html = markup({ items: [item], next_cursor: 'opaque-cursor' });
    expect(html).toContain('本页 1 条');
    expect(html).toContain('每页最多 25 条');
    expect(html).toMatch(/<button class="secondary">下一页<\/button>/);
    expect(html).not.toContain('共 1 条');
  });
  it('renders untrusted filenames as inert text and keeps skipped items unrevealable', () => {
    const html = markup({ items: [{ ...item, summary: '<img src=x onerror=alert(1)>' }], next_cursor: '' }, { focused: true, skipped: true });
    expect(html).toContain('&lt;img src=x onerror=alert(1)&gt;');
    expect(html).toContain('&lt;script&gt;unsafe&lt;/script&gt;.txt');
    expect(html).not.toContain('<script>');
    expect(html).toContain('未选择接收');
    expect(html).toMatch(/<button class="secondary" disabled="">定位文件<\/button>/);
    expect(backend.RevealInboxFile).not.toHaveBeenCalled();
  });
  it('does not imply that a historical task with no file index can be located', () => {
    expect(markup({ items: [], next_cursor: '' }, { focused: true, filesAvailable: false })).toContain('这条旧记录没有完整文件清单');
  });
  it('keeps protected history unselectable and distinguishes bilateral completion', () => {
    const html = markup({ items: [{ ...item, state: 'paused', bilateral_confirmed: false, can_forget: false }], next_cursor: '' });
    expect(html).toContain('已暂停');
    expect(html).toContain('尚未完成双方确认');
    expect(html).toMatch(/type="checkbox" disabled="" aria-label="选择记录/);
  });
});

describe('Inbox resend acknowledgment identity', () => {
  const storage = () => { const values = new Map<string, string>(); return { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value); } }; };
  it('retains an uncertain request across remount and creates a new ID only after acknowledgment', () => {
    const saved = storage(); let next = 0;
    const first = new InboxResendLedger(() => `request-${++next}`, saved);
    expect(first.begin('task-a', false)).toBe('request-1');
    const restored = new InboxResendLedger(() => `request-${++next}`, saved);
    expect(restored.begin('task-a', false)).toBe('request-1');
    expect(restored.begin('task-a', true)).toBe('request-2');
    expect(restored.confirmed('request-1')).toBe(true);
    expect(restored.begin('task-a', false)).toBe('request-3');
  });
  it('does not forget an acknowledgment identity when local receipt persistence fails', () => {
    const saved = storage(); const ledger = new InboxResendLedger(() => 'stable-id', saved);
    expect(ledger.begin('task-a', false)).toBe('stable-id');
    saved.setItem = () => { throw new Error('quota exceeded'); };
    expect(ledger.confirmed('stable-id')).toBe(false);
    expect(ledger.begin('task-a', false)).toBe('stable-id');
  });
  it('refuses a new command before backend mutation when request identity cannot be saved', () => {
    const ledger = new InboxResendLedger(() => 'id', { getItem: () => null, setItem: () => { throw new Error('storage unavailable'); } });
    expect(() => ledger.begin('task-a', false)).toThrow('storage unavailable');
  });
});

describe('Inbox filters and honest results', () => {
  it('treats the end date as inclusive and rejects invalid or oversized filters', () => {
    const query = inboxQuery({ ...filters, after: '2026-09-12', before: '2026-09-12', state: 'attention' });
    expect(new Date(query.before).getTime() - new Date(query.after).getTime()).toBe(24 * 60 * 60 * 1000);
    expect(query.states).toEqual(['paused', 'recovering', 'failed']);
    expect(() => inboxQuery({ ...filters, after: '2026-02-31' })).toThrow('有效日期');
    expect(() => inboxQuery({ ...filters, search: '中'.repeat(100) })).toThrow('关键词过长');
  });
  it('reports moved files and protected cleanup without implying deletion or success', () => {
    expect(inboxError(new Error('INBOX_FILE_MOVED_OR_DELETED'))).toContain('已被移动或删除');
    expect(cleanupNotice({ removed: [], protected: ['active'], issues: [{ task_id: 'other', code: 'error' }] })).toBe('本次没有清理暂存。 1 项仍被使用，已保留。 1 项需要核对，文件已保留。');
  });
});
