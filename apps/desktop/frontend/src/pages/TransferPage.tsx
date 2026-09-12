import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DesktopPreferences } from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/models';
import type { DeviceInfo, InboxStatus, SendDraft, WorkspaceSnapshot } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { humanizeBackendError } from '../connection';
import { EnqueueIdentity } from '../workspace-cache';
import { deviceName, fileName } from '../presentation';
import { Queue } from '../components/Queue';
import { TaskList } from '../components/Tasks';

type Props = { workspace?: WorkspaceSnapshot; devices: DeviceInfo[]; identityID?: string; inbox?: InboxStatus; preferences?: DesktopPreferences; run: CommandRunner; op: string; controlRun: CommandRunner; controlOp: string; enqueueIdentity: EnqueueIdentity; available: boolean };

export function TransferPage({ workspace, devices, identityID, inbox, preferences, run, op, controlRun, controlOp, enqueueIdentity, available }: Props) {
  const [waitForPeer, setWaitForPeer] = useState(false);
  const draft = workspace?.draft;
  const paths = draft?.paths ?? [];
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
  return <div className="page-stack">
    <div className="transfer-grid">
      <section className="surface send-surface" data-file-drop-target><div className="section-heading"><div><span className="section-kicker">本机持久草稿</span><h2>准备发送</h2></div><span className="task-count">{paths.length} 项</span></div>
        <label className="field-label">发送给<select value={draft?.peer_id ?? ''} disabled={disabled} onChange={event => saveDraft({ peer_id: event.target.value })}><option value="">选择设备</option>{draft?.peer_id && !peers.some(device => device.id === draft.peer_id) && <option value={draft.peer_id}>{selected ? deviceName(selected) : '已保存的目标'} · 当前不可发送</option>}{peers.map(device => <option key={device.id} value={device.id}>{deviceName(device)} · {device.nearby ? '附近' : device.online ? '在线' : '离线'}</option>)}</select></label>
        {selected && <div className="peer-inline"><span className="avatar small">{deviceName(selected)[0]}</span><div><strong>{deviceName(selected)}</strong><small>{selected.blocked ? '已屏蔽，请在设备页解除后重新建立信任' : selected.trusted ? '已信任 · 接收权限由对方决定' : '附近新设备 · 对方确认并完成传输后建立信任'}</small></div></div>}
        <div className="picker-row"><button className="secondary" disabled={disabled} onClick={() => void run('pick-files', async () => { const picked = await Backend.PickFiles(); if (picked?.length && draft) await Backend.SaveDraft({ ...draft, paths: [...new Set([...paths, ...picked])] }); })}>＋ 选择文件</button><button className="secondary" disabled={disabled} onClick={() => void run('pick-folder', async () => { const path = await Backend.PickSourceDirectory(); if (path && draft) await Backend.SaveDraft({ ...draft, paths: [...new Set([...paths, path])] }); })}>＋ 选择文件夹</button></div>
        <div className="file-list">{paths.length ? paths.map(path => <div className="file-row" key={path}><span className="file-icon" aria-hidden="true">▧</span><span title={path}>{fileName(path)}</span><button disabled={disabled} aria-label={`移除 ${fileName(path)}`} onClick={() => saveDraft({ paths: paths.filter(item => item !== path) })}>×</button></div>) : <div className="empty-picker"><strong>选择文件或文件夹</strong><small>切换目标会保留选择；取消选择窗口不会清空草稿。</small></div>}</div>
        <label className="queue-wait"><input type="checkbox" checked={waitForPeer} disabled={disabled} onChange={event => setWaitForPeer(event.target.checked)} /><span>对方离线时，留在队列中等待</span></label>
        {selected && !selected.online && !waitForPeer && <p className="queue-notice">对方暂时离线。勾选等待后可加入队列，也可以稍后再发。</p>}
        <button className="primary full send-button" disabled={disabled || !draft?.peer_id || !paths.length || !selected || selected.blocked || (!selected.online && !waitForPeer)} onClick={enqueue}>{op === 'enqueue' ? '正在核对内容并加入…' : '加入发送队列'}</button>
        <p className="hint">可将文件或目录拖到这里；系统入口只加入草稿。当前传输不会阻止加入下一项，重启后需确认继续。</p>
        {preview.data && preview.data.revision === draft?.revision && <p className="hint">{preview.data.complete ? '' : '已统计 '} {preview.data.files} 个文件 · {preview.data.directories} 个目录 · {(preview.data.bytes / 1024 / 1024).toLocaleString(undefined, { maximumFractionDigits: 2 })} MiB{preview.data.problem && ` · ${preview.data.problem}`}</p>}
        {draft && <small className="draft-status">{draft.updated_at ? `草稿已保存 · ${new Date(draft.updated_at).toLocaleTimeString()}` : '选择内容后保存草稿'}</small>}
      </section>
      <section className="surface receive-ready"><div className="section-heading"><div><span className="section-kicker">接收</span><h2>{inbox?.listening || inbox?.lan_available ? '可以接收文件' : '接收状态'}</h2></div><span className={inbox?.signaling_connected || inbox?.lan_available ? 'receive-orb ready' : 'receive-orb'} aria-hidden="true">↓</span></div><p className="intro">应用打开时等待接收请求，收到后显示确认。</p>
        <div className="receiver-status"><span className={inbox?.signaling_connected || inbox?.lan_available ? 'status-dot ready' : 'status-dot'} /><div><strong>{inbox?.lan_available ? '局域网接收在线' : inbox?.signaling_connected ? '远程接收连接在线' : '接收尚未就绪'}</strong><small>{inbox?.last_error ? humanizeBackendError(inbox.last_error) : '以实际连接和接收结果为准'}</small></div></div>
        <label className="field-label">默认保存到<input value={preferences?.receive_directory ?? ''} readOnly placeholder="请选择接收目录" /></label><button className="secondary full" disabled={disabled || !preferences} onClick={() => void run('receive-directory', async () => { const path = await Backend.PickDirectory(); if (path && preferences) await Backend.SavePreferences({ ...preferences, receive_directory: path }); }, '默认接收目录已更新')}>更改默认目录</button><p className="field-help">单台设备的专属目录可在设备页设置。</p>
      </section>
    </div>
    <Queue items={workspace?.queue ?? []} devices={devices} paused={workspace?.queue_paused ?? false} run={controlRun} op={controlOp} available={available && !!workspace?.persistence_available} />
    <TaskList tasks={workspace?.tasks ?? []} devices={devices} run={controlRun} op={controlOp} />
  </div>;
}
