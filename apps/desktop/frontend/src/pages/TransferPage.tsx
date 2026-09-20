import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DeviceInfo, SendDraft, WorkspaceSnapshot } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { humanizeBackendError } from '../connection';
import { EnqueueIdentity } from '../workspace-cache';
import { deviceName, fileName } from '../presentation';
import { Queue } from '../components/Queue';
import { TaskList } from '../components/Tasks';

type Props = { workspace?: WorkspaceSnapshot; devices: DeviceInfo[]; identityID?: string; run: CommandRunner; op: string; controlRun: CommandRunner; controlOp: string; enqueueIdentity: EnqueueIdentity; available: boolean; initialView?: 'send' | 'queue' | 'active' };
const pathPageSize = 10;

function clampPathPage(page: number, count: number) {
  return Math.max(0, Math.min(page, Math.max(0, Math.ceil(count / pathPageSize) - 1)));
}

export function TransferPage({ workspace, devices, identityID, run, op, controlRun, controlOp, enqueueIdentity, available, initialView = 'send' }: Props) {
  const [waitForPeer, setWaitForPeer] = useState(false);
  const [view, setView] = useState<'send' | 'queue' | 'active'>(initialView);
  const [pathPage, setPathPage] = useState(0);
  const draft = workspace?.draft;
  const paths = draft?.paths ?? [];
  const pathPageCount = Math.max(1, Math.ceil(paths.length / pathPageSize));
  const activePathPage = clampPathPage(pathPage, paths.length);
  const visiblePaths = paths.slice(activePathPage * pathPageSize, activePathPage * pathPageSize + pathPageSize);
  useEffect(() => { setPathPage(page => clampPathPage(page, paths.length)); }, [paths.length]);
  const preview = useQuery({ queryKey: ['draft-preview', draft?.revision], enabled: available && !!draft?.paths?.length,
    queryFn: () => Backend.PreviewDraft(), retry: false });
  const peers = devices.filter(device => device.id !== identityID && (device.trusted || device.nearby) && !device.blocked);
  const selected = devices.find(device => device.id === draft?.peer_id);
  const disabled = !!op || !available || !workspace?.persistence_available;
  const saveDraft = (patch: Partial<SendDraft>) => {
    if (!draft) return;
    void run('save-draft', () => Backend.SaveDraft({ ...draft, ...patch }));
  };
  const enqueue = () => {
    if (!draft) return;
    const payload = { peer_id: draft.peer_id, paths, wait_for_peer: waitForPeer, expires_at: '' };
    void run('enqueue', async () => {
      // A new Go draft revision is a deliberate new draft. Retain the previous
      // id if a successful mutation is followed by a lost/stale snapshot read.
      const requestID = enqueueIdentity.forPayload(payload, draft.revision);
      await Backend.Enqueue({ ...payload, request_id: requestID });
      try { await Backend.SaveDraft({ ...draft, paths: [] }); }
      catch (cause) { throw new Error(`已加入队列，但草稿未清空。重复加入会核对同一请求；请先刷新。${humanizeBackendError(cause)}`); }
      enqueueIdentity.confirmed(requestID);
    }, '已加入发送队列，对方仍会按接收策略确认');
  };
  return <div className="page-stack transfer-workspace"><div className="view-switch" role="tablist" aria-label="传输视图"><button className={view === 'send' ? 'active' : ''} onClick={() => setView('send')}>发送文件</button><button className={view === 'queue' ? 'active' : ''} onClick={() => setView('queue')}>待发送 <span>{workspace?.queue?.filter(item => !['completed','cancelled','expired'].includes(item.state)).length ?? 0}</span></button><button className={view === 'active' ? 'active' : ''} onClick={() => setView('active')}>进行中</button></div>
    {view === 'send' && <div className="transfer-grid">
      <section className="surface send-surface" data-file-drop-target><div className="section-heading"><div><span className="section-kicker">本机持久草稿</span><h2>准备发送</h2></div><span className="task-count">{paths.length} 项</span></div>
        <label className="field-label">发送给<select value={draft?.peer_id ?? ''} disabled={disabled} onChange={event => saveDraft({ peer_id: event.target.value })}><option value="">选择设备</option>{draft?.peer_id && !peers.some(device => device.id === draft.peer_id) && <option value={draft.peer_id}>{selected ? deviceName(selected) : '已保存的目标'} · 当前不可发送</option>}{peers.map(device => <option key={device.id} value={device.id}>{deviceName(device)} · {device.nearby ? '附近' : device.online ? '在线' : '离线'}</option>)}</select></label>
        {selected && <div className="peer-inline"><span className="avatar small">{deviceName(selected)[0]}</span><div><strong>{deviceName(selected)}</strong><small>{selected.blocked ? '已屏蔽，请在设备页解除后重新建立信任' : selected.trusted ? '已信任 · 接收权限由对方决定' : '附近新设备 · 对方确认并完成传输后建立信任'}</small></div></div>}
        <div className="picker-row"><button className="secondary" disabled={disabled} onClick={() => void run('pick-files', async () => { const picked = await Backend.PickFiles(); if (picked?.length && draft) await Backend.SaveDraft({ ...draft, paths: [...new Set([...paths, ...picked])] }); })}>＋ 选择文件</button><button className="secondary" disabled={disabled} onClick={() => void run('pick-folder', async () => { const path = await Backend.PickSourceDirectory(); if (path && draft) await Backend.SaveDraft({ ...draft, paths: [...new Set([...paths, path])] }); })}>＋ 选择文件夹</button></div>
        <label className="queue-wait"><input type="checkbox" checked={waitForPeer} disabled={disabled} onChange={event => setWaitForPeer(event.target.checked)} /><span>对方离线时，留在队列中等待</span></label>
        {selected && !selected.online && !selected.nearby && !waitForPeer && <p className="queue-notice">对方暂时离线。勾选等待后可加入队列，也可以稍后再发。</p>}
        <button className="primary full send-button" disabled={disabled || !draft?.peer_id || !paths.length || !selected || selected.blocked || (!selected.online && !selected.nearby && !waitForPeer)} onClick={enqueue}>{op === 'enqueue' ? '正在核对内容并加入…' : '发送文件'}</button>
        <div className="file-list compact-file-list">{paths.length ? visiblePaths.map(path => <div className="file-row" data-draft-path={path} key={path}><span className="file-icon" aria-hidden="true">▧</span><span title={path}>{fileName(path)}</span><button disabled={disabled} aria-label={`移除 ${fileName(path)}`} onClick={() => saveDraft({ paths: paths.filter(item => item !== path) })}>×</button></div>) : <div className="empty-picker"><strong>尚未选择文件</strong><small>选择窗口取消后不会清空已有草稿。</small></div>}</div>
        {paths.length > pathPageSize && <div className="bounded-pagination" aria-label="发送草稿分页"><button className="ghost" aria-label="草稿上一页" disabled={activePathPage === 0} onClick={() => setPathPage(page => clampPathPage(page - 1, paths.length))}>上一页</button><span>第 {activePathPage + 1} / {pathPageCount} 页 · 每页最多 {pathPageSize} 项</span><button className="ghost" aria-label="草稿下一页" disabled={activePathPage + 1 >= pathPageCount} onClick={() => setPathPage(page => clampPathPage(page + 1, paths.length))}>下一页</button></div>}
        <p className="hint">可将文件或目录拖到这里；系统入口只加入草稿。当前传输不会阻止加入下一项，重启后需确认继续。</p>
        {preview.data && preview.data.revision === draft?.revision && <p className="hint">{preview.data.complete ? '' : '已统计 '} {preview.data.files} 个文件 · {preview.data.directories} 个目录 · {(preview.data.bytes / 1024 / 1024).toLocaleString(undefined, { maximumFractionDigits: 2 })} MiB{preview.data.problem && ` · ${preview.data.problem}`}</p>}
        {draft && <small className="draft-status">{draft.updated_at ? `草稿已保存 · ${new Date(draft.updated_at).toLocaleTimeString()}` : '选择内容后保存草稿'}</small>}
      </section>
    </div>}
    {view === 'queue' && <section className="surface bounded-view"><Queue items={workspace?.queue ?? []} devices={devices} paused={workspace?.queue_paused ?? false} run={controlRun} op={controlOp} available={available && !!workspace?.persistence_available} /></section>}
    {view === 'active' && <section className="surface bounded-view"><TaskList tasks={(workspace?.tasks ?? []).filter(task => !['completed','cancelled','failed','rejected','no_content'].includes(task.state))} devices={devices} run={controlRun} op={controlOp} /></section>}
  </div>;
}
