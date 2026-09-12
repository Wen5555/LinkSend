import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it, vi } from 'vitest';
import { ContentComposer, ContentRequestLedger, clipboardCaptureLabel, contentError, contentInputError, contentRequestWasRejected } from './components/ContentComposer';
import { ReceivedContentActions, contentActionNotice } from './components/ContentActions';
import { ContentSettings } from './components/ContentSettings';
import { createQueryClient } from './workspace-cache';
import type { CommandRunner } from './hooks/useDesktop';

vi.mock('../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => ({
  PreviewReceivedText: vi.fn(), CopyReceivedText: vi.fn(), OpenReceivedURL: vi.fn(), SaveReceivedImage: vi.fn(),
}));
const run: CommandRunner = async () => true;
const intent = { draft_id: 'draft.a', draft_revision: 1, peer_id: 'peer.a', allow_file_fallback: false, wait_for_peer: false, expires_at: '' };
function memoryStorage() {
  let stored = '';
  return { getItem: () => stored, setItem: (_key: string, value: string) => { stored = value; } };
}

describe('content intent receipts', () => {
  it('allows an offline-before-enqueue rejection to be changed to waiting or another peer', () => {
    let next = 0;
    const ledger = new ContentRequestLedger(() => `request-${++next}`, memoryStorage());
    const rejected = ledger.enqueue(intent);
    expect(ledger.releaseRejected(rejected.request_id, 'enqueue', new Error('PEER_OFFLINE: explicit waiting choice required'))).toBe(true);
    expect(ledger.pending()).toEqual([]);
    const replacement = ledger.enqueue({ ...intent, peer_id: 'another', wait_for_peer: true });
    expect(replacement.request_id).not.toBe(rejected.request_id);
    expect(replacement).toMatchObject({ peer_id: 'another', wait_for_peer: true });
  });
  it('keeps the original enqueue intent after an uncertain response or an unrelated error detail', () => {
    const ledger = new ContentRequestLedger(() => 'enqueue', memoryStorage());
    const request = ledger.enqueue(intent);
    for (const cause of [new Error('IPC response lost'), new Error('database commit failed'), new Error('read PEER_OFFLINE.txt: access denied'), new Error('APP_CLOSING')]) {
      expect(ledger.releaseRejected(request.request_id, 'enqueue', cause)).toBe(false);
      expect(ledger.pending()).toEqual([request]);
      expect(() => ledger.enqueue({ ...intent, wait_for_peer: true })).toThrow('原目标');
    }
  });
  it('distinguishes an uncertain image read from a definitive empty clipboard rejection', () => {
    let next = 0;
    const storage = memoryStorage();
    const ledger = new ContentRequestLedger(() => `request-${++next}`, storage);
    const requestID = ledger.creation('native-clipboard-image');
    expect(ledger.releaseRejected(requestID, 'create', new Error('IPC disconnected'))).toBe(false);
    const remounted = new ContentRequestLedger(() => `request-${++next}`, storage);
    expect(clipboardCaptureLabel(remounted.hasPendingCreation('native-clipboard-image'))).toBe('核对上次图片读取');
    expect(remounted.creation('native-clipboard-image')).toBe(requestID);
    expect(remounted.releaseRejected(requestID, 'create', new Error('CLIPBOARD_IMAGE_NOT_AVAILABLE'))).toBe(true);
    expect(clipboardCaptureLabel(remounted.hasPendingCreation('native-clipboard-image'))).toBe('读取图片并保存草稿');
    expect(remounted.creation('native-clipboard-image')).not.toBe(requestID);
  });
  it('recognizes stable runtime rejection codes only in the relevant operation', () => {
    expect(contentRequestWasRejected('enqueue', 'RuntimeError: PEER_OFFLINE: explicit waiting choice required')).toBe(true);
    expect(contentRequestWasRejected('create', 'RuntimeError: CONTENT_LIMIT_EXCEEDED')).toBe(true);
    expect(contentRequestWasRejected('enqueue', 'RuntimeError: CLIPBOARD_IMAGE_NOT_AVAILABLE')).toBe(false);
    expect(contentRequestWasRejected('enqueue', 'RuntimeError: IDEMPOTENCY_CONFLICT')).toBe(false);
  });
  it('keeps a lost creation receipt across remount and separates creation from enqueue IDs', () => {
    const storage = memoryStorage();
    let next = 0;
    const ids = () => `request-${++next}`;
    const first = new ContentRequestLedger(ids, storage);
    const create = first.creation('text:sha256-of-input');
    const restored = new ContentRequestLedger(ids, storage);
    expect(restored.creation('text:sha256-of-input')).toBe(create);
    expect(restored.enqueue(intent).request_id).not.toBe(create);
    expect(restored.enqueue(intent)).toEqual(restored.enqueue(intent));
    expect(storage.getItem()).not.toContain('hello private content');
  });
  it('keeps the same image request after an uncertain capture, then permits an explicit new capture', () => {
    let next = 0;
    const ledger = new ContentRequestLedger(() => `request-${++next}`);
    const first = ledger.creation('native-clipboard-image');
    expect(ledger.creation('native-clipboard-image')).toBe(first);
    expect(ledger.confirmed(first)).toBe(true);
    expect(ledger.creation('native-clipboard-image')).not.toBe(first);
  });
  it('freezes target, revision, waiting and explicit fallback while an enqueue is uncertain', () => {
    const storage = memoryStorage();
    const ledger = new ContentRequestLedger(() => 'enqueue', storage);
    const request = ledger.enqueue(intent);
    for (const patch of [{ peer_id: 'another' }, { draft_revision: 2 }, { allow_file_fallback: true }, { wait_for_peer: true }]) {
      expect(() => ledger.enqueue({ ...intent, ...patch })).toThrow('原目标');
    }
    const restored = new ContentRequestLedger(() => 'wrong', storage);
    expect(restored.pending()).toEqual([request]);
    expect(restored.enqueue(intent).request_id).toBe('enqueue');
  });
  it('does not allow callers to mutate the stored retry payload', () => {
    const ledger = new ContentRequestLedger(() => 'enqueue');
    const initial = ledger.enqueue(intent);
    initial.allow_file_fallback = true;
    ledger.pending()[0].peer_id = 'another';
    expect(ledger.enqueue(intent)).toMatchObject({ peer_id: 'peer.a', allow_file_fallback: false });
  });
  it('retains the request if receipt persistence fails after backend success', () => {
    const backing = memoryStorage();
    let fail = false;
    const storage = { getItem: backing.getItem, setItem: (key: string, value: string) => { if (fail) throw new Error('storage unavailable'); backing.setItem(key, value); } };
    const ledger = new ContentRequestLedger(() => 'enqueue', storage);
    const request = ledger.enqueue(intent);
    fail = true;
    expect(ledger.confirmed(request.request_id)).toBe(false);
    expect(ledger.enqueue(intent)).toEqual(request);
  });
  it('fails before generating an actionable request if local persistence is unavailable or corrupted', () => {
    const storage = { getItem: () => '', setItem: () => { throw new Error('storage unavailable'); } };
    expect(() => new ContentRequestLedger(() => 'request', storage).creation('image')).toThrow('storage unavailable');
    const corrupted = { getItem: () => '[{"type":"enqueue","request":{"draft_id":"d"}}]', setItem: () => {} };
    expect(() => new ContentRequestLedger(() => 'request', corrupted).pending()).toThrow('无法读取');
    expect(() => new ContentRequestLedger(() => 'request', { ...corrupted, getItem: () => 'x'.repeat(65537) }).pending()).toThrow('过多');
  });
  it('bounds uncertain intents without silently dropping an old request', () => {
    let next = 0;
    const ledger = new ContentRequestLedger(() => `request-${++next}`);
    for (let index = 0; index < 64; index++) ledger.creation(`text:${index}`);
    expect(() => ledger.creation('another')).toThrow('尚未确认');
    expect(ledger.creation('text:0')).toBe('request-1');
  });
});

describe('content validation and native boundary presentation', () => {
  it('checks UTF-8 bytes rather than JavaScript character count', () => {
    expect(contentInputError('text', '文'.repeat(21845))).toBe('');
    expect(contentInputError('text', '文'.repeat(21846))).toContain('64 KiB');
    expect(contentInputError('text', 'a\0b')).toContain('空字符');
    expect(contentInputError('text', '  ')).toContain('请输入');
  });
  it('accepts only web URL schemes while allowing literal text to remain text', () => {
    for (const value of ['javascript:alert(1)', 'file:///etc/passwd', 'data:text/html,x', 'example.com']) expect(contentInputError('url', value)).not.toBe('');
    expect(contentInputError('url', 'https://example.com/path?q=中文')).toBe('');
    expect(contentInputError('text', 'javascript:alert(1)')).toBe('');
  });
  it('distinguishes submitted native actions, completed copy/save and cancelled save', () => {
    expect(contentActionNotice({ task_id: 'task', action: 'preview', state: 'submitted' })).toBe('已请求打开文字预览。');
    expect(contentActionNotice({ task_id: 'task', action: 'open_url', state: 'submitted' })).toContain('已请求');
    expect(contentActionNotice({ task_id: 'task', action: 'copy', state: 'completed' })).toContain('已复制');
    expect(contentActionNotice({ task_id: 'task', action: 'save_image', state: 'completed' })).toContain('已保存');
    expect(contentActionNotice({ task_id: 'task', action: 'save_image', state: 'cancelled' })).toBe('已取消保存。');
    expect(contentActionNotice({ task_id: 'task', action: 'open_url', state: 'unknown' })).not.toContain('已请求');
  });
  it('uses actual backend error codes for absent clipboard images and changed received files', () => {
    expect(contentError(new Error('CLIPBOARD_IMAGE_NOT_AVAILABLE'))).toContain('剪贴板中没有');
    expect(contentError(new Error('INVALID_CONTENT_URL'))).toContain('http');
    expect(contentError(new Error('INBOX_FILE_CHANGED'))).toContain('已变化');
    expect(contentError(new Error('CONTENT_NATIVE_ACTIONS_UNAVAILABLE'))).toContain('尚未启用');
  });
  it('renders compose controls without file/image byte IPC inputs or automatic enqueue', () => {
    const markup = renderToStaticMarkup(createElement(QueryClientProvider, { client: createQueryClient() }, createElement(ContentComposer, { devices: [], run, op: '', available: false })));
    expect(markup).toContain('保存内容草稿');
    expect(markup).toContain('内容类型');
    expect(markup).not.toContain('type="file"');
    expect(markup).not.toContain('<img');
    expect(markup).not.toContain('确认并加入发送队列');
  });
  it('keeps content inspection collapsed until the user asks, with no preview/copy/open on mount', () => {
    const markup = renderToStaticMarkup(createElement(QueryClientProvider, { client: createQueryClient() }, createElement(ReceivedContentActions, { taskID: 'received', revision: 3, run, op: '', available: true })));
    expect(markup).toContain('aria-expanded="false"');
    expect(markup).toContain('查看内容操作');
    expect(markup).not.toContain('复制文字');
    expect(markup).not.toContain('确认打开');
  });
  it('defaults snapshot retention off and explains preservation of received user files', () => {
    const client = createQueryClient();
    client.setQueryData(['content', 'settings'], { retain_sent_snapshots: false });
    const markup = renderToStaticMarkup(createElement(QueryClientProvider, { client }, createElement(ContentSettings, { run, op: '', available: true })));
    expect(markup).not.toContain('checked=""');
    expect(markup).toContain('发送快照');
    expect(markup).toContain('已经清理的内容不能从历史重发');
    expect(markup).not.toContain('删除收到的文件');
  });
});
