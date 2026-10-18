import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DeviceInfo, SendDraft, WorkspaceSnapshot } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { humanizeBackendError } from '../connection';
import { EnqueueIdentity } from '../workspace-cache';
import { deviceConnectionLabel, deviceName, fileName } from '../presentation';
import { TransferQueue } from '../components/TransferQueue';
import { SourceSelectionPanel } from '../components/SourceSelectionPanel';
import { BatchTargets, batchTargetIDs } from '../components/BatchTargets';

type Props = { workspace?: WorkspaceSnapshot; devices: DeviceInfo[]; identityID?: string; run: CommandRunner; op: string; controlRun: CommandRunner; controlOp: string; enqueueIdentity: EnqueueIdentity; available: boolean };
const pathPageSize = 10;

function clampPathPage(page: number, count: number) {
  return Math.max(0, Math.min(page, Math.max(0, Math.ceil(count / pathPageSize) - 1)));
}

export function TransferPage({ workspace, devices, identityID, run, op, controlRun, controlOp, enqueueIdentity, available }: Props) {
  const [waitForPeer, setWaitForPeer] = useState(false);
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
  const targets = batchTargetIDs(draft);
  const multiple = targets.length > 1;
  const targetDevices = targets.map(id => devices.find(device => device.id === id));
  const validTargets = targetDevices.length > 0 && targetDevices.every(device => device && !device.blocked && (!multiple || device.trusted));
  const canAttempt = validTargets && targetDevices.every(device => device && (device.online || device.nearby || device.connection_state === 'connected'));
  const disabled = !!op || !available || !workspace?.persistence_available;
  const saveDraft = (patch: Partial<SendDraft>) => {
    if (!draft) return;
    void run('save-draft', () => Backend.SaveDraft({ ...draft, ...patch }));
  };
  const enqueue = () => {
    if (!draft) return;
    const payload = { peer_id: draft.peer_id, peer_ids: targets, paths, source_selection: draft.source_selection, draft_id: draft.id, draft_revision: draft.revision, wait_for_peer: waitForPeer, expires_at: '' };
    void run('enqueue', async () => {
      // A new Go draft revision is a deliberate new draft. Retain the previous
      // id if a successful mutation is followed by a lost/stale snapshot read.
      const requestID = enqueueIdentity.forPayload(payload, draft.revision);
      if (multiple) await Backend.EnqueueBatch({ request_id: requestID, draft_id: draft.id, draft_revision: draft.revision, wait_for_peer: waitForPeer, expires_at: '' });
      else await Backend.Enqueue({ request_id: requestID, peer_id: draft.peer_id, paths, source_selection: draft.source_selection, draft_id: draft.id, draft_revision: draft.revision, wait_for_peer: waitForPeer, expires_at: '' });
      try { await Backend.SaveDraft({ ...draft, paths: [] }); }
      catch (cause) { throw new Error(`已加入队列，但草稿未清空。重复加入会核对同一请求；请先刷新。${humanizeBackendError(cause)}`); }
      enqueueIdentity.confirmed(requestID);
    }, multiple ? '已原子加入多设备批次；每台设备独立确认和执行' : '已加入发送队列，对方仍会按接收策略确认').then(ok => { if (ok) requestAnimationFrame(() => document.getElementById('transfer-queue-title')?.scrollIntoView?.({ block: 'nearest', behavior: 'smooth' })); });
  };
  return <div className="page-stack transfer-workspace">
    <section className="surface send-surface" data-file-drop-target>
      <div className="section-heading compact"><h2>发送文件</h2><small className="field-help">选择后发送，收发进度在下方查看</small></div>
      <div className="send-compose">
        <div className="send-controls">
          {multiple ? <div className="batch-summary"><strong>分发给 {targets.length} 台设备</strong><button className="ghost" disabled={disabled} onClick={() => saveDraft({ peer_ids: [], peer_id: draft?.peer_id ?? '' })}>改为单台</button></div> : <label className="field-label">发送给<select value={draft?.peer_id ?? ''} disabled={disabled} onChange={event => saveDraft({ peer_id: event.target.value, peer_ids: [] })}><option value="">选择设备</option>{draft?.peer_id && !peers.some(device => device.id === draft.peer_id) && <option value={draft.peer_id}>{selected ? deviceName(selected) : '已保存的目标'} · 当前不可发送</option>}{peers.map(device => <option key={device.id} value={device.id}>{deviceName(device)} · {deviceConnectionLabel(device)}</option>)}</select></label>}
          <div className="picker-row"><button className="secondary" disabled={disabled} onClick={() => void run('pick-files', async () => { const picked = await Backend.PickFiles(); if (picked?.length && draft) await Backend.SaveDraft({ ...draft, paths: [...new Set([...paths, ...picked])] }); })}>＋ 选择文件</button><button className="secondary" disabled={disabled} onClick={() => void run('pick-folder', async () => { const path = await Backend.PickSourceDirectory(); if (path && draft) await Backend.SaveDraft({ ...draft, paths: [...new Set([...paths, path])] }); })}>＋ 选择文件夹</button></div>
          <label className="queue-wait"><input type="checkbox" checked={waitForPeer} disabled={disabled} onChange={event => setWaitForPeer(event.target.checked)} /><span>对方离线时，留在队列中等待</span></label>
          {selected && !canAttempt && !waitForPeer && <p className="queue-notice">{multiple ? '部分目标尚未在线或连接未确认。' : selected.service_state === 'membership_synced' ? '服务器列表已同步，尚未找到对方在线。请在对方打开 LinkSend。' : '尚未确认对方当前是否可连接，请检查连接状态。'}勾选等待后可加入队列，也可以稍后再发。</p>}
          {multiple && !validTargets && <p className="queue-notice">部分目标当前不可发送，请展开多设备分发调整选择。</p>}
        </div>
        <div className="send-files">
          <div className="file-list compact-file-list">{paths.length ? visiblePaths.map(path => <div className="file-row" data-draft-path={path} key={path}><span className="file-icon" aria-hidden="true">▧</span><span title={path}>{fileName(path)}</span><button disabled={disabled} aria-label={`移除 ${fileName(path)}`} onClick={() => saveDraft({ paths: paths.filter(item => item !== path) })}>×</button></div>) : <div className="empty-picker"><strong>选择或拖入文件、文件夹</strong><small>发送后会加入下方队列。</small></div>}</div>
          {paths.length > pathPageSize && <div className="bounded-pagination" aria-label="发送草稿分页"><button className="ghost" aria-label="草稿上一页" disabled={activePathPage === 0} onClick={() => setPathPage(page => clampPathPage(page - 1, paths.length))}>上一页</button><span>{activePathPage + 1} / {pathPageCount} 页</span><button className="ghost" aria-label="草稿下一页" disabled={activePathPage + 1 >= pathPageCount} onClick={() => setPathPage(page => clampPathPage(page + 1, paths.length))}>下一页</button></div>}
          <button className="primary full send-button" disabled={disabled || !paths.length || !validTargets || (!canAttempt && !waitForPeer)} onClick={enqueue}>{op === 'enqueue' ? '正在核对内容并加入…' : multiple ? `分发给 ${targets.length} 台设备` : '发送文件'}</button>
        </div>
      </div>
      <details className="send-options"><summary>多设备分发与发送范围{draft?.source_selection?.preset === 'source' ? ' · 已启用源码预设' : ''}</summary>
        {draft && <BatchTargets draft={draft} devices={devices} identityID={identityID} disabled={disabled} save={saveDraft} />}
        {draft && <SourceSelectionPanel draft={draft} preview={preview.data} disabled={disabled} save={saveDraft} />}
      </details>
    </section>
    <TransferQueue workspace={workspace} devices={devices} run={controlRun} op={controlOp} available={available && !!workspace?.persistence_available} />
  </div>;
}
