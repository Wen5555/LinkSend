import { useState } from 'react';
import type { DeviceInfo, SendDraft } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { deviceConnectionLabel, deviceName } from '../presentation';

type Props = { draft: SendDraft; devices: DeviceInfo[]; identityID?: string; disabled: boolean; save: (patch: Partial<SendDraft>) => void };
const pageSize = 10;
export const batchTargetIDs = (draft?: SendDraft) => draft?.peer_ids?.length ? draft.peer_ids : draft?.peer_id ? [draft.peer_id] : [];

export function BatchTargets({ draft, devices, identityID, disabled, save }: Props) {
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(0);
  const selected = batchTargetIDs(draft);
  const eligible = devices.filter(device => device.id !== identityID && (device.trusted && !device.blocked || selected.includes(device.id)));
  const missing = selected.filter(id => !devices.some(device => device.id === id));
  const candidates = [...eligible.map(device => ({ id: device.id, name: deviceName(device), status: deviceConnectionLabel(device), selectable: device.trusted && !device.blocked })), ...missing.map(id => ({ id, name: `已保存目标 ${id.slice(0, 10)}`, status: '当前不可发送', selectable: false }))];
  const filtered = candidates.filter(device => `${device.name} ${device.id}`.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()));
  const pages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const activePage = Math.min(page, pages - 1);
  const toggle = (id: string, checked: boolean) => {
    const peers = checked ? [...selected, id] : selected.filter(peer => peer !== id);
    save({ peer_ids: peers, peer_id: peers[0] ?? '' });
  };
  return <details className="batch-targets"><summary>多设备分发 · 已选 {selected.length} 台</summary>
    <p className="field-help">选择 2–16 台已配对设备。一次加入独立目标任务，逐个发送；每次发送前重新核对源文件。</p>
    <label className="field-label">查找分发目标<input aria-label="查找分发目标" value={query} onChange={event => { setQuery(event.target.value); setPage(0); }} /></label>
    <div className="batch-target-list">{filtered.slice(activePage * pageSize, (activePage + 1) * pageSize).map(device => <label className="batch-target-choice" key={device.id}>
      <input type="checkbox" aria-label={`分发给 ${device.name}`} checked={selected.includes(device.id)} disabled={disabled || !selected.includes(device.id) && (!device.selectable || selected.length >= 16)} onChange={event => toggle(device.id, event.target.checked)} />
      <span><strong>{device.name}</strong><small>{device.status}</small></span>
    </label>)}</div>
    {!filtered.length && <p className="field-help">没有匹配的已配对设备。</p>}
    {pages > 1 && <div className="bounded-pagination"><button className="ghost" disabled={activePage === 0} onClick={() => setPage(activePage - 1)}>目标上一页</button><span>{activePage + 1} / {pages}</span><button className="ghost" disabled={activePage + 1 === pages} onClick={() => setPage(activePage + 1)}>目标下一页</button></div>}
  </details>;
}
