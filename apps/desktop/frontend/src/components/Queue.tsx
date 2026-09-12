import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DeviceInfo, QueueItem } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { humanizeBackendError } from '../connection';
import { canOrderQueue, deviceName, movedQueueIDs, queueLabels } from '../presentation';

export function Queue({ items, devices, paused, run, op, available }: { items: QueueItem[]; devices: DeviceInfo[]; paused: boolean; run: CommandRunner; op: string; available: boolean }) {
  const pending = items.filter(item => !['completed', 'cancelled', 'expired'].includes(item.state));
  const ordered = pending.filter(canOrderQueue);
  return <section className="queue-section" aria-labelledby="queue-title">
    <div className="section-heading compact"><div><span className="section-kicker">一次执行一项</span><h2 id="queue-title">发送队列 <span className="task-count">{pending.length}</span></h2></div><button className="secondary" disabled={!!op || !available} onClick={() => void run('queue-pause', () => Backend.SetQueuePaused(!paused), paused ? '已恢复队列调度' : '已暂停后续调度，当前传输继续')}>{paused ? '恢复调度' : '暂停调度'}</button></div>
    {paused && <p className="queue-notice" role="status">后续任务暂不开始。当前传输可在任务列表单独暂停。</p>}
    {!pending.length ? <div className="queue-empty">队列为空。传输进行中也可以加入下一项。</div> : <ol className="queue-list">{pending.map(item => {
      const index = ordered.findIndex(row => row.id === item.id);
      const device = devices.find(row => row.id === item.peer_id);
      return <li className="queue-row" key={item.id}>
        <div className="queue-details"><strong>{item.source_summary || '文件或文件夹'}</strong><span>发送给 {device ? deviceName(device) : item.peer_id.slice(0, 10)} · {queueLabels[item.state] || item.state}</span>
          {item.state === 'needs_attention' && <p>{item.last_error === 'SOURCE_CHANGED' ? '源内容已变化，请核对原始文件后确认。' : '请核对文件与目标设备，确认后继续原有待办。'}</p>}
          {item.last_error && <small>{humanizeBackendError(item.last_error)}</small>}
          {item.expires_at && <small>到期时间：{new Date(item.expires_at).toLocaleString()}</small>}
        </div>
        <div className="queue-actions">
          {canOrderQueue(item) && <><button className="ghost" aria-label={`上移 ${item.source_summary}`} disabled={!!op || index === 0} onClick={() => void run(item.id, () => Backend.ReorderQueue(movedQueueIDs(pending, item.id, -1)))}>↑</button><button className="ghost" aria-label={`下移 ${item.source_summary}`} disabled={!!op || index === ordered.length - 1} onClick={() => void run(item.id, () => Backend.ReorderQueue(movedQueueIDs(pending, item.id, 1)))}>↓</button></>}
          {item.state === 'needs_attention' && <button className="primary" disabled={!!op || device?.blocked} onClick={() => void run(item.id, () => Backend.ConfirmQueue(item.id, item.revision), '已确认继续；仍会执行对方的接收策略')}>确认继续</button>}
          <button className="ghost danger" disabled={!!op} onClick={() => void run(item.id, () => Backend.CancelQueue(item.id, item.revision), '已取消队列项')}>取消</button>
        </div>
      </li>;
    })}</ol>}
  </section>;
}
