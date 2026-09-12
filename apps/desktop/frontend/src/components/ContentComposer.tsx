import { useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { ContentDraft, DeviceInfo, EnqueueContentRequest } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { Kind as ContentKind } from '../../bindings/github.com/Wen5555/LinkSend/internal/content/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { humanizeBackendError } from '../connection';
import { deviceName, formatBytes } from '../presentation';
import './ContentComposer.css';

type Kind = 'text' | 'url' | 'image';
type Props = { devices: DeviceInfo[]; identityID?: string; initialPeerID?: string; run: CommandRunner; op: string; available: boolean };
type StorageLike = Pick<Storage, 'getItem' | 'setItem'>;
type Creation = { type: 'create'; key: string; requestID: string };
type Enqueue = { type: 'enqueue'; request: EnqueueContentRequest };
type Entry = Creation | Enqueue;
const storageKey = 'linksend.content.pending.v1';
export const contentDraftKey = ['content', 'drafts'] as const;
export const contentKindLabel = (kind: string) => ({ text: '文字', url: '链接', image: '图片' })[kind] ?? '内容';

/** Persist intent identity and queue metadata, never plaintext, image bytes or paths. */
export class ContentRequestLedger {
  private entries: Entry[] = [];
  constructor(private readonly makeID: () => string, private readonly storage?: StorageLike) {}
  private restore() {
    const encoded = this.storage?.getItem(storageKey);
    if (!encoded) return;
    if (encoded.length > 65536) throw new Error('待核对的内容请求过多，请先处理发送队列。');
    const parsed: unknown = JSON.parse(encoded);
    const bounded = (value: unknown) => typeof value === 'string' && value.length > 0 && value.length <= 256;
    if (!Array.isArray(parsed) || parsed.length > 64 || !parsed.every((entry: unknown) => {
      if (!entry || typeof entry !== 'object') return false;
      const item = entry as Partial<Entry>;
      if (item.type === 'create') return bounded(item.key) && bounded(item.requestID);
      if (item.type !== 'enqueue' || !item.request) return false;
      const request = item.request;
      return bounded(request.request_id) && bounded(request.draft_id) && bounded(request.peer_id) &&
        Number.isSafeInteger(request.draft_revision) && request.draft_revision > 0 &&
        typeof request.allow_file_fallback === 'boolean' && typeof request.wait_for_peer === 'boolean' && request.expires_at === '';
    })) throw new Error('待核对的内容请求无法读取，请先核对发送队列。');
    this.entries = parsed;
  }
  private save(next: Entry[]) { this.storage?.setItem(storageKey, JSON.stringify(next)); this.entries = next; }
  private append(entry: Entry) {
    if (this.entries.length >= 64) throw new Error('有多条内容请求尚未确认，请先核对发送队列。');
    this.save([...this.entries, entry]);
  }
  creation(key: string): string {
    this.restore();
    const previous = this.entries.find((entry): entry is Creation => entry.type === 'create' && entry.key === key);
    if (previous) return previous.requestID;
    const requestID = this.makeID();
    this.append({ type: 'create', key, requestID });
    return requestID;
  }
  hasPendingCreation(key: string): boolean {
    this.restore();
    return this.entries.some(entry => entry.type === 'create' && entry.key === key);
  }
  enqueue(intent: Omit<EnqueueContentRequest, 'request_id'>): EnqueueContentRequest {
    this.restore();
    const previous = this.entries.find((entry): entry is Enqueue => entry.type === 'enqueue' && entry.request.draft_id === intent.draft_id);
    if (previous) {
      if (Object.entries(intent).some(([key, value]) => previous.request[key as keyof EnqueueContentRequest] !== value)) {
        throw new Error('这份草稿已有待核对的入队请求，请先按原目标核对结果。');
      }
      return { ...previous.request };
    }
    const request = { ...intent, request_id: this.makeID() };
    this.append({ type: 'enqueue', request });
    return { ...request };
  }
  pending(): EnqueueContentRequest[] {
    this.restore();
    return this.entries.filter((entry): entry is Enqueue => entry.type === 'enqueue').map(entry => ({ ...entry.request }));
  }
  confirmed(requestID: string): boolean {
    try { this.restore(); this.save(this.entries.filter(entry => (entry.type === 'create' ? entry.requestID : entry.request.request_id) !== requestID)); }
    catch { return false; }
    return true;
  }
  releaseRejected(requestID: string, operation: 'create' | 'enqueue', cause: unknown): boolean {
    return contentRequestWasRejected(operation, cause) && this.confirmed(requestID);
  }
}

/** These backend codes are returned before content creation/enqueue commits.
 * Unknown failures (including lost IPC responses) must keep the original ID. */
export function contentRequestWasRejected(operation: 'create' | 'enqueue', cause: unknown): boolean {
  const text = cause instanceof Error ? cause.message : String(cause ?? '');
  const code = text.replace(/^(?:Error|RuntimeError):\s*/, '').match(/^([A-Z][A-Z0-9_]+)(?=$|[:\s])/)?.[1];
  const codes = operation === 'enqueue'
    ? ['PEER_OFFLINE', 'UNPAIRED', 'AUTHENTICATION_FAILED', 'INVALID_ARGUMENT', 'RESOURCE_LIMIT', 'CONTENT_SNAPSHOT_CHANGED', 'CONTENT_OWNERSHIP_MISMATCH']
    : ['CLIPBOARD_IMAGE_NOT_AVAILABLE', 'CLIPBOARD_IMAGE_FORMAT_UNSUPPORTED', 'CLIPBOARD_CHANGED_DURING_CAPTURE', 'CLIPBOARD_UNAVAILABLE', 'INVALID_CONTENT_TEXT', 'INVALID_CONTENT_URL', 'INVALID_CONTENT_IMAGE', 'CONTENT_LIMIT_EXCEEDED', 'CONTENT_DRAFT_LIMIT'];
  return !!code && codes.includes(code);
}

export const clipboardCaptureLabel = (pending: boolean) => pending ? '核对上次图片读取' : '读取图片并保存草稿';

export function contentInputError(kind: Kind, value: string): string {
  if (kind === 'image') return '';
  if (!value.trim()) return '请输入要发送的内容。';
  if (value.includes('\0')) return '内容不能包含空字符。';
  if (new TextEncoder().encode(value).length > 64 * 1024) return '文字或链接最多为 64 KiB，请缩短后发送。';
  if (kind === 'url') {
    try { const url = new URL(value); if (!['http:', 'https:'].includes(url.protocol) || !url.hostname) return '仅支持完整的 http 或 https 链接。'; }
    catch { return '请输入完整的 http 或 https 链接。'; }
  }
  return '';
}

export function contentError(cause: unknown): string {
  const raw = String(cause ?? '');
  const known: Record<string, string> = {
    CONTENT_CAPABILITY_REQUIRED: '对方暂不支持此内容类型。可以更新对方应用，或明确选择按文件发送。',
    CONTENT_DRAFT_CONSUMED: '这份草稿已经处理，请刷新草稿和发送队列。',
    CONTENT_DRAFT_LIMIT: '已保存的内容草稿过多，请先发送或丢弃一些草稿。',
    CONTENT_LIMIT: '内容超过大小或图片尺寸限制。',
    INVALID_CONTENT_URL: '仅支持完整的 http 或 https 链接。',
    INVALID_CONTENT_TEXT: '文字为空或格式无效，请检查输入。',
    INVALID_CONTENT_IMAGE: '图片格式无效，请重新复制图片。',
    CLIPBOARD_IMAGE_NOT_AVAILABLE: '剪贴板中没有可用图片，请先复制图片再读取。',
    CLIPBOARD_IMAGE_FORMAT_UNSUPPORTED: '暂不支持剪贴板中的图片格式，请重新复制为 PNG 图片。',
    CLIPBOARD_CHANGED_DURING_CAPTURE: '读取时剪贴板发生变化，请重新复制图片再试。',
    CLIPBOARD_UNAVAILABLE: '当前无法读取系统剪贴板，请稍后重试。',
    CONTENT_IMAGE_NOT_AVAILABLE: '这张图片尚不可保存，请核对接收结果。',
    CONTENT_NOT_AVAILABLE: '内容尚未完成接收，或原文件已不可用。',
    CONTENT_SNAPSHOT_CHANGED: '已保存的发送快照发生变化，请重新创建草稿。',
    INBOX_FILE_CHANGED: '保存的内容已变化，请在接收目录中核对。',
    INBOX_FILE_MOVED_OR_DELETED: '收到的内容已被移动或删除，请在接收目录中核对。',
    INBOX_FILE_NOT_AVAILABLE: '此内容尚未完成接收，或旧记录没有保存位置。',
    CONTENT_URL_SCHEME: '该内容不能作为网页链接打开，可按文字查看。',
    METADATA_REVISION_CONFLICT: '草稿已经更新，请刷新后重新选择。',
    IDEMPOTENCY_CONFLICT: '请求与之前的操作不一致，请先核对发送队列。',
    NATIVE_RUNTIME_UNAVAILABLE: '请在桌面应用中执行此操作。',
    CONTENT_NATIVE_ACTIONS_UNAVAILABLE: '当前桌面尚未启用原生内容操作。',
  };
  for (const [code, message] of Object.entries(known)) if (raw.includes(code)) return message;
  return humanizeBackendError(cause);
}

async function creationKey(kind: Kind, value: string): Promise<string> {
  if (kind === 'image') return 'native-clipboard-image';
  const digest = await window.crypto.subtle.digest('SHA-256', new TextEncoder().encode(`${kind}\0${value}`));
  return `${kind}:${Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('')}`;
}

export function ContentComposer({ devices, identityID, initialPeerID, run, op, available }: Props) {
  const client = useQueryClient();
  const [kind, setKind] = useState<Kind>('text');
  const [value, setValue] = useState('');
  const [peerID, setPeerID] = useState(initialPeerID ?? '');
  const [draft, setDraft] = useState<ContentDraft>();
  const [allowFallback, setAllowFallback] = useState(false);
  const [waitForPeer, setWaitForPeer] = useState(false);
  const [pending, setPending] = useState<EnqueueContentRequest[]>([]);
  const [capturePending, setCapturePending] = useState(false);
  const [discard, setDiscard] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false);
  const active = useRef(false);
  const ledger = useRef<ContentRequestLedger | null>(null);
  const disabled = !available || !!op || busy;
  const drafts = useQuery({ queryKey: contentDraftKey, enabled: available, retry: false, refetchOnWindowFocus: 'always',
    queryFn: ({ signal }) => Backend.ContentDrafts().cancelOn(signal) });
  const peers = devices.filter(device => device.id !== identityID && !device.blocked && (device.trusted || device.nearby));
  const selected = peers.find(device => device.id === peerID);
  const pendingDraft = pending.some(request => request.draft_id === draft?.id);
  const inputProblem = contentInputError(kind, value);
  const getLedger = () => {
    if (!ledger.current) ledger.current = new ContentRequestLedger(() => window.crypto.randomUUID(), window.sessionStorage);
    return ledger.current;
  };
  useEffect(() => {
    try { setPending(getLedger().pending()); setCapturePending(getLedger().hasPendingCreation('native-clipboard-image')); }
    catch (cause) { setError(contentError(cause)); }
  }, []);
  const refresh = () => { void client.invalidateQueries({ queryKey: contentDraftKey }); };
  const execute = async (key: string, work: () => Promise<void>) => {
    if (disabled || active.current) return;
    active.current = true; setBusy(true); setError(''); setNotice('');
    try { await run(key, work, undefined, cause => setError(contentError(cause))); }
    finally { active.current = false; setBusy(false); try { setPending(getLedger().pending()); setCapturePending(getLedger().hasPendingCreation('native-clipboard-image')); } catch (cause) { setError(contentError(cause)); } }
  };
  const create = () => void execute('content-create', async () => {
    if (inputProblem) throw new Error(inputProblem);
    const requestID = getLedger().creation(await creationKey(kind, value));
    let result: ContentDraft;
    try { result = kind === 'image' ? await Backend.CaptureClipboardImage(requestID) : await Backend.CreateContentText({ request_id: requestID, kind: kind === 'url' ? ContentKind.URL : ContentKind.Text, text: value }); }
    catch (cause) { getLedger().releaseRejected(requestID, 'create', cause); throw cause; }
    const marked = getLedger().confirmed(requestID);
    if (result.state === 'draft') { setDraft(result); setDiscard(false); setAllowFallback(false); setNotice(marked ? '草稿已保存，请确认目标和发送方式。' : '草稿已保存；请求标记尚未更新，重试会核对同一份草稿。'); }
    else { setDraft(undefined); setNotice('这份内容已处理，请在发送队列或收件箱中核对。'); }
    setValue(''); refresh();
  });
  const enqueue = (retry?: EnqueueContentRequest) => void execute('content-enqueue', async () => {
    const request = retry ?? (draft && getLedger().enqueue({ draft_id: draft.id, draft_revision: draft.revision, peer_id: peerID,
      allow_file_fallback: allowFallback, wait_for_peer: waitForPeer, expires_at: '' }));
    if (!request) return;
    try { await Backend.EnqueueContent(request); }
    catch (cause) { getLedger().releaseRejected(request.request_id, 'enqueue', cause); throw cause; }
    const marked = getLedger().confirmed(request.request_id);
    setDraft(current => current?.id === request.draft_id ? undefined : current); setDiscard(false);
    setAllowFallback(false); setWaitForPeer(false);
    setNotice(marked ? '已加入发送队列。' : '已加入发送队列；本机标记尚未更新，核对请求不会重复发送。'); refresh();
  });
  const chooseDraft = (item: ContentDraft) => { setDraft(item); setDiscard(false); setAllowFallback(false); setWaitForPeer(false); setError(''); setNotice(''); };
  const discardDraft = () => void execute('content-discard', async () => {
    if (!draft || pendingDraft) return;
    await Backend.DiscardContentDraft(draft.id, draft.revision);
    setDraft(undefined); setDiscard(false); setNotice('草稿已丢弃。未使用的快照可在设置中清理。'); refresh();
  });

  return <section className="surface content-composer" aria-label="发送文字、链接或图片">
    <div className="section-heading"><div><span className="section-kicker">内容发送</span><h2>文字、链接与图片</h2></div></div>
    <div className="content-kind-switch" role="group" aria-label="内容类型">{(['text', 'url', 'image'] as const).map(item => <button key={item} className={kind === item ? 'secondary selected' : 'ghost'} aria-pressed={kind === item} disabled={disabled} onClick={() => { setKind(item); setError(''); }}>{contentKindLabel(item)}</button>)}</div>
    {kind === 'image' ? <div className="content-image-input"><strong>{capturePending ? '核对上一次图片读取的结果' : '读取当前剪贴板中的图片'}</strong><p>{capturePending ? '上次结果尚未确认。核对会沿用同一请求；若图片已经保存，将取回当时的草稿。核对完成后可再次读取新图片。' : '点击后保存这一刻的图片，发送前确认尺寸和大小。'}</p><small>图片不超过 32 MiB、4000 万像素，单边不超过 32768 像素。</small></div> : <label className="field-label">{kind === 'url' ? '网页链接' : '要发送的文字'}<textarea value={value} disabled={disabled} rows={kind === 'url' ? 3 : 5} maxLength={65536} spellCheck={kind !== 'url'} placeholder={kind === 'url' ? 'https://example.com' : '输入文字，先保存草稿再确认发送'} onChange={event => setValue(event.target.value)} /><span className="content-input-size">{formatBytes(new TextEncoder().encode(value).length)} / 64 KiB</span></label>}
    <button className="secondary" disabled={disabled || !!inputProblem} onClick={create}>{busy && op === 'content-create' ? '正在保存草稿…' : kind === 'image' ? clipboardCaptureLabel(capturePending) : '保存内容草稿'}</button>
    {inputProblem && value && <p className="content-note">{inputProblem}</p>}
    {!!drafts.data?.length && <div className="content-draft-list"><label className="field-label">继续已保存的草稿<select value={draft?.id ?? ''} disabled={disabled} onChange={event => { const item = drafts.data?.find(entry => entry.id === event.target.value); if (item) chooseDraft(item); }}><option value="">选择草稿</option>{drafts.data.map(item => <option key={item.id} value={item.id}>{contentKindLabel(item.snapshot.kind)} · {formatBytes(item.snapshot.size)} · {new Date(item.created_at).toLocaleString()}</option>)}</select></label></div>}
    {drafts.isError && <p className="content-error" role="alert">{contentError(drafts.error)} <button className="ghost" disabled={disabled} onClick={refresh}>重新读取草稿</button></p>}
    {draft && <div className="content-confirm" aria-label="确认内容草稿">
      <h3>确认发送</h3><dl className="content-metadata"><div><dt>内容</dt><dd>{contentKindLabel(draft.snapshot.kind)}</dd></div><div><dt>大小</dt><dd>{formatBytes(draft.snapshot.size)}</dd></div>{draft.snapshot.kind === 'image' && <div><dt>尺寸</dt><dd>{draft.snapshot.width} × {draft.snapshot.height}</dd></div>}</dl>
      <label className="field-label">发送给<select value={peerID} disabled={disabled || pendingDraft} onChange={event => { setPeerID(event.target.value); setAllowFallback(false); }}><option value="">选择设备</option>{peerID && !peers.some(peer => peer.id === peerID) && <option value={peerID}>已选目标 · 当前不可发送</option>}{peers.map(device => <option key={device.id} value={device.id}>{deviceName(device)} · {device.online ? '在线' : '离线'}</option>)}</select></label>
      <label className="queue-wait"><input type="checkbox" checked={waitForPeer} disabled={disabled || pendingDraft} onChange={event => setWaitForPeer(event.target.checked)} /><span>对方离线时，留在队列中等待</span></label>
      <label className="queue-wait"><input type="checkbox" checked={allowFallback} disabled={disabled || pendingDraft || !selected} onChange={event => setAllowFallback(event.target.checked)} /><span>如果{selected ? `“${deviceName(selected)}”` : '对方'}不支持此类型，允许按普通文件发送</span></label>
      <p className="content-note">{allowFallback ? '只在对方不支持时按文件发送，对方仍需确认接收。' : '对方不支持此内容类型时停止发送。'}</p>
      {selected && !selected.online && !waitForPeer && <p className="content-note">对方暂时离线，勾选等待后可加入队列。</p>}
      <div className="content-buttons"><button className="primary" disabled={disabled || pendingDraft || !selected || (!selected.online && !waitForPeer)} onClick={() => enqueue()}>确认并加入发送队列</button><button className="ghost danger" disabled={disabled || pendingDraft} onClick={() => setDiscard(true)}>丢弃草稿</button></div>
      {discard && <div className="content-discard" role="group" aria-label="确认丢弃草稿"><p>丢弃这份{contentKindLabel(draft.snapshot.kind)}草稿？</p><button className="secondary" disabled={disabled} onClick={discardDraft}>确认丢弃</button><button className="ghost" disabled={disabled} onClick={() => setDiscard(false)}>保留草稿</button></div>}
    </div>}
    {!!pending.length && <div className="content-pending"><h3>核对尚未确认的入队请求</h3><p className="content-note">请求可能已经送达。核对会沿用原目标和发送方式，不会重复入队。</p>{pending.map(request => <div key={request.request_id}><span>{deviceName(devices.find(device => device.id === request.peer_id))} · {request.allow_file_fallback ? '允许按文件发送' : '仅发送原内容类型'}{request.wait_for_peer ? ' · 等待上线' : ''}</span><button className="secondary" disabled={disabled} onClick={() => enqueue(request)}>核对原请求</button></div>)}</div>}
    {error && <p className="content-error" role="alert">{error}</p>}{notice && <p className="content-notice" role="status">{notice}</p>}
  </section>;
}
