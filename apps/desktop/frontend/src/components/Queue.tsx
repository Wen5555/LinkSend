import { useEffect, useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { BatchSnapshot, DeviceInfo, QueueItem } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { humanizeBackendError } from '../connection';
import { canOrderQueue, deviceName, movedQueueIDs, queueLabels } from '../presentation';

const queuePageSize = 10;
function clampQueuePage(page: number, count: number) {
  return Math.max(0, Math.min(page, Math.max(0, Math.ceil(count / queuePageSize) - 1)));
}

export function Queue({ items, batches = [], devices, paused, run, op, available }: { items: QueueItem[]; batches?: BatchSnapshot[]; devices: DeviceInfo[]; paused: boolean; run: CommandRunner; op: string; available: boolean }) {
  const pending = items.filter(item => !['completed', 'cancelled', 'expired'].includes(item.state));
  const ordered = pending.filter(canOrderQueue);
  const [page, setPage] = useState(0);
  const pageCount = Math.max(1, Math.ceil(pending.length / queuePageSize));
  const activePage = clampQueuePage(page, pending.length);
  const visible = pending.slice(activePage * queuePageSize, activePage * queuePageSize + queuePageSize);
  const batchIDs = [...new Set([...visible.map(item => item.batch_id).filter((id): id is string => !!id), ...batches.filter(batch => batch.state === 'cancelling').map(batch => batch.id)])];
  useEffect(() => { setPage(current => clampQueuePage(current, pending.length)); }, [pending.length]);
  return <section className="queue-section" aria-labelledby="queue-title">
    <div className="section-heading compact"><div><span className="section-kicker">一次执行一项</span><h2 id="queue-title">发送队列 <span className="task-count">{pending.length}</span></h2></div><button className="secondary" disabled={!!op || !available} onClick={() => void run('queue-pause', () => Backend.SetQueuePaused(!paused), paused ? '已恢复队列调度' : '已暂停后续调度，当前传输继续')}>{paused ? '恢复调度' : '暂停调度'}</button></div>
    {paused && <p className="queue-notice" role="status">后续任务暂不开始。当前传输可在任务列表单独暂停。</p>}
    {batchIDs.map(id => { const batch = batches.find(batch => batch.id === id); return <div className="batch-summary queue-notice" key={id}><span><strong>分发批次 {id.slice(0, 8)}</strong>{batch && <small> · {batch.completed}/{batch.total} 完成 · {batch.waiting} 待发送 · {batch.needs_attention} 需确认{batch.state === 'cancelling' && ' · 正在停止活动任务'}</small>}</span><button className="ghost danger" disabled={!available || !!op || batch?.cancelled} onClick={() => void run(`cancel-batch-${id}`, async () => { const current = batch ?? await Backend.GetBatch(id); await Backend.CancelBatch(id, current.revision); }, '已保存批次取消；已完成目标与接收文件保留')}>取消此批次未完成目标</button></div>; })}
    {!pending.length ? <div className="queue-empty">队列为空。传输进行中也可以加入下一项。</div> : <><ol className="queue-list">{visible.map(item => {
      const index = ordered.findIndex(row => row.id === item.id);
      const device = devices.find(row => row.id === item.peer_id);
      return <li className="queue-row" data-queue-id={item.id} key={item.id}>
        <div className="queue-details"><strong>{item.source_summary || '文件或文件夹'}</strong><span>发送给 {device ? deviceName(device) : item.peer_id.slice(0, 10)} · {queueLabels[item.state] || item.state}</span>
          {item.batch_id && <small>批次 {item.batch_id.slice(0, 8)} · 第 {item.batch_position} 个目标</small>}
          {item.source_selection && <small>{item.source_selection.preset === 'source' ? '源码目录' : '自定义排除'} · 规则版本 {item.source_selection.version} · 恢复沿用此范围</small>}
          {item.state === 'needs_attention' && <p>{item.last_error === 'SOURCE_CHANGED' ? '源内容已变化，请核对原始文件后确认。' : '请核对文件与目标设备，确认后继续原有待办。'}</p>}
          {item.last_error && <small>{humanizeBackendError(item.last_error)}</small>}
          {item.expires_at && <small>到期时间：{new Date(item.expires_at).toLocaleString()}</small>}
        </div>
        <div className="queue-actions">
          {canOrderQueue(item) && <><button className="ghost" aria-label={`上移 ${item.source_summary}，队列项 ${item.id}`} disabled={!!op || index === 0} onClick={() => void run(item.id, () => Backend.ReorderQueue(movedQueueIDs(pending, item.id, -1)))}>↑</button><button className="ghost" aria-label={`下移 ${item.source_summary}，队列项 ${item.id}`} disabled={!!op || index === ordered.length - 1} onClick={() => void run(item.id, () => Backend.ReorderQueue(movedQueueIDs(pending, item.id, 1)))}>↓</button></>}
          {item.state === 'needs_attention' && <button className="primary" aria-label={`确认继续 ${item.source_summary}，队列项 ${item.id}`} disabled={!!op || device?.blocked} onClick={() => void run(item.id, () => Backend.ConfirmQueue(item.id, item.revision), '已确认继续；仍会执行对方的接收策略')}>确认继续</button>}
          <button className="ghost danger" aria-label={`取消 ${item.source_summary}，队列项 ${item.id}`} disabled={!!op} onClick={() => void run(item.id, () => Backend.CancelQueue(item.id, item.revision), '已取消队列项')}>取消</button>
        </div>
      </li>;
    })}</ol>{pending.length > queuePageSize && <div className="bounded-pagination" aria-label="发送队列分页"><button className="ghost" aria-label="队列上一页" disabled={activePage === 0} onClick={() => setPage(current => clampQueuePage(current - 1, pending.length))}>上一页</button><span>第 {activePage + 1} / {pageCount} 页 · 每页最多 {queuePageSize} 项</span><button className="ghost" aria-label="队列下一页" disabled={activePage + 1 >= pageCount} onClick={() => setPage(current => clampQueuePage(current + 1, pending.length))}>下一页</button></div>}</>}
  </section>;
}
