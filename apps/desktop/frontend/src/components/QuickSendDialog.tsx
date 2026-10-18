import { useCallback, useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DeviceInfo, SendDraft } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { useCommand } from '../hooks/useDesktop';
import { CommandLanes } from '../workspace-cache';
import { deviceConnectionLabel, deviceName, fileName } from '../presentation';
import { humanizeBackendError } from '../connection';

const quickKey = ['quick-send-draft'] as const;
type Props = { devices: DeviceInfo[]; identityID?: string; available: boolean; busy: boolean; lanes: CommandLanes; refresh: () => Promise<void>; close: () => void };

export function QuickSendDialog({ devices, identityID, available, busy, lanes, refresh, close }: Props) {
  const client = useQueryClient();
  const draftQuery = useQuery({ queryKey: quickKey, enabled: available, staleTime: 0, refetchInterval: 5000, queryFn: async () => {
    const incoming = await Backend.QuickSendDraft();
    const current = client.getQueryData<SendDraft>(quickKey);
    return current && current.revision > incoming.revision ? current : incoming;
  } });
  const refreshQuick = useCallback(async () => { await Promise.all([refresh(), client.invalidateQueries({ queryKey: quickKey })]); }, [refresh, client]);
  const command = useCommand(refreshQuick, lanes, 'draft');
  const [query, setQuery] = useState('');
  const [wait, setWait] = useState(false);
  const [page, setPage] = useState(0);
  const dialog = useRef<HTMLElement>(null);
  const draft = draftQuery.data;
  const paths = draft?.paths ?? [];
  const selected = devices.find(device => device.id === draft?.peer_id);
  const choices = identityID ? devices.filter(device => device.id !== identityID && device.trusted && !device.blocked && `${deviceName(device)} ${device.id}`.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())).slice(0, 10) : [];
  const ready = !!identityID && selected?.id !== identityID && selected?.trusted && !selected.blocked;
  const reachable = selected && (selected.online || selected.nearby || selected.connection_state === 'connected');
  const disabled = !available || busy || !!command.op || !draft;
  const pages = Math.max(1, Math.ceil(paths.length / 10));
  const activePage = Math.min(page, pages - 1);
  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialog.current?.querySelector<HTMLInputElement>('input[aria-label="查找快速发送设备"]')?.focus();
    return () => { if (previous?.isConnected) previous.focus(); };
  }, []);
  const save = async (patch: Partial<SendDraft>) => {
    if (!draft) return;
    const saved = await Backend.SaveQuickSendDraft({ ...draft, ...patch });
    client.setQueryData(quickKey, saved);
  };
  const enqueue = () => {
    if (!draft || disabled || !paths.length || !ready || !reachable && !wait) return;
    void command.run('quick-enqueue', async () => {
      await Backend.EnqueueQuickSend(draft.revision, wait);
      try { await save({ paths: [] }); }
      catch (error) { throw new Error(`已加入队列，快速草稿尚未清空；重复加入会核对原请求。${humanizeBackendError(error)}`); }
    }, '已加入发送队列，对方仍按接收策略确认').then(ok => { if (ok) close(); });
  };
  return <div className="quick-send-backdrop" role="presentation"><section ref={dialog} className="quick-send-dialog surface" role="dialog" aria-modal="true" aria-labelledby="quick-send-title" onKeyDown={event => {
    if (event.key === 'Escape' && !busy && !command.op) { event.preventDefault(); close(); return; }
    if (event.key !== 'Tab') return;
    const controls = Array.from(dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled)') ?? []);
    const first = controls[0], last = controls[controls.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
  }}>
    <header className="section-heading"><div><span className="section-kicker">托盘快速发送</span><h2 id="quick-send-title">发送文件</h2></div><button className="ghost" aria-label="关闭快速发送" disabled={!!command.op || busy} onClick={close}>×</button></header>
    <p className="field-help">使用独立草稿。取消会保留此处选择，主窗口草稿保持。</p>
    {(command.error || draftQuery.error) && <p className="queue-notice" role="alert">{command.error || humanizeBackendError(draftQuery.error)}</p>}
    <label className="field-label">最近与常用设备<input aria-label="查找快速发送设备" value={query} onChange={event => setQuery(event.target.value)} placeholder="名称、别名或完整设备身份" /></label>
    <label className="field-label">发送给<select aria-label="快速发送目标" value={draft?.peer_id ?? ''} disabled={disabled || !identityID} onChange={event => { const peerID = event.target.value; void command.run('quick-target', () => save({ peer_id: peerID })); }}><option value="">选择已配对设备</option>{draft?.peer_id && !choices.some(device => device.id === draft.peer_id) && <option value={draft.peer_id} disabled={!ready}>{selected ? deviceName(selected) : '已保存目标'} · {ready ? '已选择' : '当前不可发送'}</option>}{choices.map(device => <option key={device.id} value={device.id}>{deviceName(device)} · {deviceConnectionLabel(device)}</option>)}</select></label>
    {!identityID && <p className="field-help" role="status">正在读取本机身份，稍后即可选择发送设备。</p>}
    <p className="field-help">优先置顶与最近使用，每次最多显示10台；可搜索其余设备。</p>
    <div className="picker-row"><button className="secondary" disabled={disabled} onClick={() => void command.run('quick-files', async () => { const picked = await Backend.PickFiles(); if (picked?.length) await save({ paths: [...new Set([...paths, ...picked])] }); })}>选择文件</button><button className="secondary" disabled={disabled} onClick={() => void command.run('quick-folder', async () => { const folder = await Backend.PickSourceDirectory(); if (folder) await save({ paths: [...new Set([...paths, folder])] }); })}>选择文件夹</button></div>
    <div className="file-list compact-file-list">{paths.length ? paths.slice(activePage * 10, (activePage + 1) * 10).map(path => <div className="file-row" key={path}><span title={path}>{fileName(path)}</span><button className="ghost" aria-label={`从快速发送移除 ${fileName(path)}`} disabled={disabled} onClick={() => void command.run('quick-remove', () => save({ paths: paths.filter(item => item !== path) }))}>×</button></div>) : <p className="field-help">尚未选择文件或文件夹</p>}</div>
    {pages > 1 && <div className="bounded-pagination"><button className="ghost" disabled={activePage === 0} onClick={() => setPage(activePage - 1)}>文件上一页</button><span>{activePage + 1} / {pages}</span><button className="ghost" disabled={activePage + 1 >= pages} onClick={() => setPage(activePage + 1)}>文件下一页</button></div>}
    <label className="queue-wait"><input type="checkbox" disabled={disabled} checked={wait} onChange={event => setWait(event.target.checked)} /><span>对方离线时，留在队列中等待</span></label>
    {identityID && selected && !ready && <p className="queue-notice">该目标当前不可发送，请重新选择已配对设备。</p>}
    <footer className="quick-send-actions"><button className="ghost" disabled={!!command.op || busy} onClick={close}>取消</button><button className="primary" disabled={disabled || !paths.length || !ready || !reachable && !wait} onClick={enqueue}>{command.op === 'quick-enqueue' ? '正在核对并加入…' : '加入发送队列'}</button></footer>
  </section></div>;
}
