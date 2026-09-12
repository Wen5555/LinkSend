import { useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DeviceInfo, InboxCleanupResult, InboxItem, InboxQuery, InboxRecordRef } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { humanizeBackendError } from '../connection';
import { deviceName, formatBytes, taskLabels } from '../presentation';
import './InboxPage.css';
import { ReceivedContentActions } from '../components/ContentActions';

type Props = { devices: DeviceInfo[]; run: CommandRunner; op: string; available: boolean; focusTaskID?: string; navigationRevision?: number };
type Filters = { search: string; peer: string; direction: string; state: string; after: string; before: string };
type Action = { kind: 'forget'; records: InboxRecordRef[] } | { kind: 'cleanup' } | { kind: 'resend'; item: InboxItem };
type StorageLike = Pick<Storage, 'getItem' | 'setItem'>;
type PendingResend = { taskID: string; wait: boolean; requestID: string };

const emptyFilters: Filters = { search: '', peer: '', direction: '', state: '', after: '', before: '' };
const stateGroups: Record<string, string[]> = {
  completed: ['completed'], active: ['preparing', 'awaiting_acceptance', 'transferring', 'verifying'],
  attention: ['paused', 'recovering', 'failed'], stopped: ['cancelled', 'rejected'], skipped: ['no_content'],
};
const ledgerKey = 'linksend.inbox.resend.pending.v1';

/** Retain uncertain commands across navigation and reload. Only opaque task and
 * request IDs are stored, never paths, file names or content. */
export class InboxResendLedger {
  private entries: PendingResend[] = [];
  constructor(private readonly makeID: () => string, private readonly storage?: StorageLike) {}
  begin(taskID: string, wait: boolean): string {
    this.restore();
    const found = this.entries.find(entry => entry.taskID === taskID && entry.wait === wait);
    if (found) return found.requestID;
    if (this.entries.length >= 64) throw new Error('有多条重发请求尚未确认，请先在发送队列核对结果。');
    const requestID = this.makeID();
    const next = [...this.entries, { taskID, wait, requestID }];
    this.storage?.setItem(ledgerKey, JSON.stringify(next));
    this.entries = next;
    return requestID;
  }
  confirmed(requestID: string): boolean {
    const next = this.entries.filter(entry => entry.requestID !== requestID);
    try { this.storage?.setItem(ledgerKey, JSON.stringify(next)); }
    catch { return false; } // keep the same ID if receipt persistence is uncertain
    this.entries = next;
    return true;
  }
  private restore() {
    const encoded = this.storage?.getItem(ledgerKey);
    if (!encoded) return;
    if (encoded.length > 24576) throw new Error('本机重发记录超出限制，请先核对发送队列。');
    const parsed: unknown = JSON.parse(encoded);
    if (!Array.isArray(parsed) || parsed.length > 64 || !parsed.every((entry: unknown) => {
      if (!entry || typeof entry !== 'object') return false;
      const value = entry as Partial<PendingResend>;
      return typeof value.taskID === 'string' && value.taskID.length > 0 && value.taskID.length <= 128 &&
        typeof value.requestID === 'string' && value.requestID.length > 0 && value.requestID.length <= 128 && typeof value.wait === 'boolean';
    })) throw new Error('本机重发记录无法读取，请先核对发送队列。');
    this.entries = parsed;
  }
}

export function inboxQuery(filters: Filters): InboxQuery {
  const boundary = (day: string, end: boolean) => {
    if (!day) return '';
    if (!/^\d{4}-\d{2}-\d{2}$/.test(day)) throw new Error('请选择有效日期。');
    const date = new Date(`${day}T00:00:00`);
    const parts = day.split('-').map(Number);
    if (!Number.isFinite(date.getTime()) || date.getFullYear() !== parts[0] || date.getMonth() + 1 !== parts[1] || date.getDate() !== parts[2]) throw new Error('请选择有效日期。');
    if (end) date.setDate(date.getDate() + 1);
    return date.toISOString();
  };
  const after = boundary(filters.after, false), before = boundary(filters.before, true);
  if (after && before && after >= before) throw new Error('结束日期不能早于开始日期。');
  if (new TextEncoder().encode(filters.search.trim()).length > 256) throw new Error('关键词过长，请缩短后重试。');
  return { peer_id: filters.peer, direction: filters.direction, states: stateGroups[filters.state] ?? [],
    after, before, search: filters.search.trim(), cursor: '', limit: 25 };
}

export function inboxError(cause: unknown): string {
  const raw = String(cause ?? '');
  const known: Record<string, string> = {
    INBOX_FILE_MOVED_OR_DELETED: '文件已被移动或删除，原位置已不可用。',
    INBOX_FILE_CHANGED: '原位置的文件已经变化，请在接收目录中核对。',
    INBOX_FILE_NOT_AVAILABLE: '此文件尚未完成接收，或这条旧记录没有保存位置。',
    INBOX_RECORD_IN_USE: '记录仍被活动、暂停或可恢复任务使用，已保留。',
    INBOX_STAGING_OWNERSHIP_MISMATCH: '有暂存文件已变化，已保留以供核对。',
    METADATA_REVISION_CONFLICT: '记录已更新，请刷新后重新选择。',
    METADATA_NOT_FOUND: '这条记录已不可用，请刷新列表。',
    SOURCE_CHANGED: '源文件已变化，请重新选择文件后发送。',
    FILE_NOT_FOUND: '源文件已被移动或删除，请重新选择。',
    NATIVE_REVEAL_UNSUPPORTED: '当前系统暂不支持在文件管理器中定位。',
    WORKSPACE_STORE_UNAVAILABLE: '本机记录暂时无法读取，请先检查应用状态。',
  };
  for (const [code, message] of Object.entries(known)) if (raw.includes(code)) return message;
  return humanizeBackendError(cause);
}

export function cleanupNotice(result: InboxCleanupResult): string {
  const removed = result.removed?.length ?? 0, protectedCount = result.protected?.length ?? 0, issues = result.issues?.length ?? 0;
  return `${removed ? `已清理 ${removed} 项孤立暂存。` : '本次没有清理暂存。'}${protectedCount ? ` ${protectedCount} 项仍被使用，已保留。` : ''}${issues ? ` ${issues} 项需要核对，文件已保留。` : ''}`;
}

export function InboxPage({ devices, run, op, available, focusTaskID, navigationRevision }: Props) {
  const client = useQueryClient();
  const [filters, setFilters] = useState<Filters>({ ...emptyFilters });
  const [applied, setApplied] = useState<InboxQuery>(() => inboxQuery(emptyFilters));
  const [cursors, setCursors] = useState(['']);
  const [selected, setSelected] = useState<InboxRecordRef[]>([]);
  const [detailID, setDetailID] = useState(focusTaskID ?? '');
  const [fileCursors, setFileCursors] = useState(['']);
  const [action, setAction] = useState<Action>();
  const [waitForPeer, setWaitForPeer] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const ledger = useRef<InboxResendLedger | null>(null);
  const localBusy = useRef(false);
  const panel = useRef<HTMLElement>(null);
  const blocked = !!op || !available;
  const query = { ...applied, cursor: cursors[cursors.length - 1] };
  const history = useQuery({ queryKey: ['inbox', 'history', query], enabled: available, retry: false, refetchOnWindowFocus: 'always', refetchOnReconnect: 'always', refetchInterval: 10000,
    queryFn: ({ signal }) => Backend.Inbox(query).cancelOn(signal) });
  const files = useQuery({ queryKey: ['inbox', 'files', detailID, fileCursors[fileCursors.length - 1]], enabled: available && !!detailID, retry: false,
    queryFn: ({ signal }) => Backend.InboxFiles(detailID, fileCursors[fileCursors.length - 1], 25).cancelOn(signal) });
  const items = history.data?.items ?? [];
  const detail = items.find(item => item.task_id === detailID);

  useEffect(() => {
    if (!focusTaskID) return;
    setDetailID(focusTaskID); setFileCursors(['']); setAction(undefined); setError('');
  }, [focusTaskID, navigationRevision]);
  useEffect(() => {
    if (action) panel.current?.querySelector<HTMLButtonElement>('button')?.focus();
  }, [action]);
  useEffect(() => {
    if (!history.data) return;
    setSelected(previous => previous.filter(record => history.data.items?.some(item => item.task_id === record.task_id && item.revision === record.revision && item.can_forget)));
    setAction(current => current?.kind === 'forget' && current.records.some(record => !history.data.items?.some(item => item.task_id === record.task_id && item.revision === record.revision && item.can_forget)) ? undefined : current);
  }, [history.data]);

  const refresh = () => { void client.invalidateQueries({ queryKey: ['inbox'] }); };
  const changePage = (next: string[] | ((previous: string[]) => string[])) => { setCursors(next); setSelected([]); setAction(undefined); setError(''); };
  const openFiles = (id: string) => { setDetailID(id); setFileCursors(['']); setError(''); };
  const execute = async (key: string, work: () => Promise<unknown>) => {
    if (blocked || localBusy.current) return;
    localBusy.current = true; setError(''); setNotice('');
    try { await run(key, work, undefined, cause => setError(inboxError(cause))); }
    finally { localBusy.current = false; }
  };
  const submitAction = () => {
    if (!action) return;
    const current = action;
    void execute(`inbox-${current.kind}`, async () => {
      if (current.kind === 'forget') {
        await Backend.ForgetInboxRecords(current.records);
        setSelected([]); setNotice(`已删除 ${current.records.length} 条记录，文件仍保留。`);
        if (current.records.some(record => record.task_id === detailID)) setDetailID('');
        setCursors(['']);
      } else if (current.kind === 'cleanup') {
        setNotice(cleanupNotice(await Backend.CleanupInboxStaging(100)));
      } else {
        if (!ledger.current) ledger.current = new InboxResendLedger(() => window.crypto.randomUUID(), window.sessionStorage);
        const requestID = ledger.current.begin(current.item.task_id, waitForPeer);
        await Backend.ResendInbox({ task_id: current.item.task_id, request_id: requestID, wait_for_peer: waitForPeer });
        setNotice(ledger.current.confirmed(requestID) ? '已作为新任务加入发送队列。' : '已加入队列；本机标记更新失败，重试会核对同一请求。');
      }
      setAction(undefined); refresh();
    });
  };

  return <div className="page-stack inbox-page">
    <div className="page-intro"><div><p className="eyebrow">本机传输记录</p><h2>收件箱</h2><p>查找文件、查看保存结果，或从历史重新发送。</p></div><button className="secondary" disabled={blocked} onClick={() => { setAction({ kind: 'cleanup' }); setError(''); }}>清理孤立暂存</button></div>
    <form className="inbox-filters" onSubmit={event => { event.preventDefault(); try { setApplied(inboxQuery(filters)); changePage(['']); setNotice(''); } catch (cause) { setError(inboxError(cause)); } }}>
      <label className="field-label inbox-search">文件名<input type="search" value={filters.search} maxLength={128} placeholder="搜索文件名，支持中文" onChange={event => setFilters(previous => ({ ...previous, search: event.target.value }))} /></label>
      <label className="field-label">设备<select value={filters.peer} onChange={event => setFilters(previous => ({ ...previous, peer: event.target.value }))}><option value="">所有设备</option>{devices.map(device => <option key={device.id} value={device.id}>{deviceName(device)}</option>)}</select></label>
      <label className="field-label">方向<select value={filters.direction} onChange={event => setFilters(previous => ({ ...previous, direction: event.target.value }))}><option value="">全部方向</option><option value="receive">接收</option><option value="send">发送</option></select></label>
      <label className="field-label">状态<select value={filters.state} onChange={event => setFilters(previous => ({ ...previous, state: event.target.value }))}><option value="">全部状态</option><option value="completed">已完成</option><option value="active">进行中</option><option value="attention">需要处理</option><option value="stopped">已取消或拒绝</option><option value="skipped">全部跳过</option></select></label>
      <label className="field-label">开始日期<input type="date" value={filters.after} onChange={event => setFilters(previous => ({ ...previous, after: event.target.value }))} /></label>
      <label className="field-label">截止日期<input type="date" value={filters.before} onChange={event => setFilters(previous => ({ ...previous, before: event.target.value }))} /></label>
      <div className="inbox-filter-actions"><button className="primary" disabled={!available}>应用筛选</button><button type="button" className="ghost" onClick={() => { setFilters({ ...emptyFilters }); setApplied(inboxQuery(emptyFilters)); changePage(['']); }}>重置</button></div>
    </form>

    {notice && <p className="inbox-notice" role="status">{notice}</p>}
    {error && <p className="inbox-error" role="alert">{error}</p>}
    {action && <section ref={panel} className="inbox-confirm" role="region" aria-label={action.kind === 'forget' ? '删除记录确认' : action.kind === 'cleanup' ? '清理暂存确认' : '重新发送确认'}>
      <div><h3>{action.kind === 'forget' ? `删除 ${action.records.length} 条记录？` : action.kind === 'cleanup' ? '清理不再使用的本机暂存？' : '作为新任务重新发送？'}</h3><p>{action.kind === 'forget' ? '只删除本机历史，已接收文件保留。活动或可恢复任务仍受保护。' : action.kind === 'cleanup' ? '检查最多 100 项暂存。被草稿、队列或未完成任务引用的内容会保留，已接收文件保留。' : `${action.item.summary || '这次传输'} · 会重新核对源文件，源已变化时需要重新选择。`}</p>
        {action.kind === 'resend' && <label className="queue-wait"><input type="checkbox" checked={waitForPeer} disabled={blocked} onChange={event => setWaitForPeer(event.target.checked)} />对方离线时，在队列中等待</label>}
      </div><div className="inbox-confirm-actions"><button className="ghost" disabled={!!op} onClick={() => setAction(undefined)}>取消</button><button className={action.kind === 'forget' ? 'secondary danger' : 'primary'} disabled={blocked} onClick={submitAction}>{action.kind === 'forget' ? '只删除记录' : action.kind === 'cleanup' ? '确认清理暂存' : '确认重新发送'}</button></div>
    </section>}

    <section className="inbox-history" aria-labelledby="inbox-history-title">
      <div className="section-heading compact"><div><h3 id="inbox-history-title">传输记录</h3><span className="muted">第 {cursors.length} 页 · 本页 {items.length} 条</span></div><div className="inline-actions"><button className="ghost danger" disabled={blocked || !selected.length} onClick={() => { setAction({ kind: 'forget', records: [...selected] }); setError(''); }}>删除选中记录{selected.length ? `（${selected.length}）` : ''}</button><button className="secondary" disabled={!available || history.isFetching} onClick={refresh}>刷新</button></div></div>
      {!available ? <div className="empty-state small"><strong>本机记录尚未就绪</strong><p>在桌面应用中连接后端后，可查看传输历史。</p></div> : history.isPending ? <p className="inbox-placeholder" role="status">正在读取本机记录…</p> : history.isError ? <p className="inbox-error" role="alert">{inboxError(history.error)}</p> : !items.length ? <div className="empty-state small"><strong>没有匹配的记录</strong><p>可调整筛选条件，或先完成一次传输。</p></div> : <div className="inbox-rows">{items.map(item => {
        const device = devices.find(candidate => candidate.id === item.peer_id);
        const checked = selected.some(record => record.task_id === item.task_id);
        return <article key={item.task_id} className={`inbox-row ${detailID === item.task_id ? 'is-selected' : ''}`}>
          <input className="inbox-record-check" type="checkbox" checked={checked} disabled={blocked || !item.can_forget} aria-label={`选择记录 ${item.summary || '文件传输'}`} title={item.can_forget ? '选择本条记录' : '活动、可恢复或队列引用中的记录会保留'} onChange={event => { setAction(undefined); setSelected(previous => event.target.checked ? [...previous, { task_id: item.task_id, revision: item.revision }] : previous.filter(record => record.task_id !== item.task_id)); }} />
          <span className={`task-symbol ${item.direction}`} aria-hidden="true">{item.direction === 'send' ? '↑' : '↓'}</span>
          <div className="inbox-row-main"><button className="inbox-item-title" aria-expanded={detailID === item.task_id} onClick={() => openFiles(item.task_id)}>{item.summary || '文件传输'}</button><div className="inbox-item-meta"><span>{item.direction === 'send' ? '发送至' : '接收自'} {device ? deviceName(device) : item.peer_id ? `设备 ${item.peer_id.slice(0, 8)}` : '未知设备'}</span><span>{item.file_count} 项</span><time dateTime={item.started_at}>{dateLabel(item.started_at)}</time></div><small>{item.bilateral_confirmed ? '双方已确认' : '尚未完成双方确认'} · 已验证 {formatBytes(item.verified_bytes)} · 已保存 {formatBytes(item.committed_bytes)}</small></div>
          <div className="inbox-row-actions"><span className={`state-badge ${item.state}`}>{item.state === 'no_content' ? '全部跳过' : taskLabels[item.state] || item.state}</span><button className="ghost" disabled={!available} onClick={() => openFiles(item.task_id)}>查看文件</button>{item.can_resume && <button className="primary" disabled={blocked || device?.blocked} onClick={() => void execute('inbox-resume', async () => { await Backend.ResumeTask(item.task_id); setNotice('正在恢复原任务；请在传输页查看进度。'); refresh(); })}>恢复原任务</button>}{item.can_resend && <button className="secondary" disabled={blocked || device?.blocked} onClick={() => { setAction({ kind: 'resend', item }); setWaitForPeer(false); setError(''); }}>重新发送</button>}</div>
        </article>;
      })}</div>}
      <div className="inbox-pagination"><button className="secondary" disabled={!available || history.isFetching || cursors.length < 2} onClick={() => changePage(previous => previous.slice(0, -1))}>上一页</button><span>每页最多 25 条</span><button className="secondary" disabled={!available || history.isFetching || !history.data?.next_cursor} onClick={() => { if (history.data?.next_cursor) changePage(previous => [...previous, history.data.next_cursor]); }}>下一页</button></div>
    </section>

    {detailID && <ReceivedContentActions taskID={detailID} revision={detail?.revision} run={run} op={op} available={available} />}
    {detailID && <section className="inbox-files" aria-labelledby="inbox-files-title"><div className="section-heading compact"><div><span className="section-kicker">文件清单</span><h3 id="inbox-files-title">{detail?.summary || '所选任务的文件'}</h3></div><button className="ghost" onClick={() => setDetailID('')}>收起文件</button></div>
      {!available ? <p className="muted">后端就绪后可读取文件清单。</p> : files.isPending ? <p role="status">正在读取文件清单…</p> : files.isError ? <p className="inbox-error" role="alert">{inboxError(files.error)}</p> : !files.data?.available ? <p className="muted">这条旧记录没有完整文件清单，无法从这里定位文件。</p> : !(files.data.files ?? []).length ? <p className="muted">没有文件条目。</p> : <div className="inbox-file-rows">{(files.data.files ?? []).map(file => <div className="inbox-file-row" key={file.file_id}><span className="file-icon" aria-hidden="true">{file.kind === 'directory' ? '▤' : '▧'}</span><div><strong>{file.name}</strong><small>{file.kind === 'directory' ? '目录' : formatBytes(file.size)}{!file.selected && ' · 未选择接收'}</small></div><button className="secondary" disabled={blocked || !file.selected || (!!detail && (detail.direction !== 'receive' || !detail.bilateral_confirmed || detail.state !== 'completed'))} onClick={() => void execute(`inbox-reveal-${file.file_id}`, async () => { await Backend.RevealInboxFile(detailID, file.file_id); setNotice('已请求文件管理器定位该文件。'); })}>定位文件</button></div>)}</div>}
      <div className="inbox-pagination"><button className="ghost" disabled={!available || files.isFetching || fileCursors.length < 2} onClick={() => setFileCursors(previous => previous.slice(0, -1))}>上一页文件</button><span>第 {fileCursors.length} 页</span><button className="ghost" disabled={!available || files.isFetching || !files.data?.next_cursor} onClick={() => { if (files.data?.next_cursor) setFileCursors(previous => [...previous, files.data.next_cursor]); }}>下一页文件</button></div>
    </section>}
  </div>;
}

function dateLabel(value: string): string { const date = new Date(value); return Number.isFinite(date.getTime()) ? date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '时间未记录'; }
