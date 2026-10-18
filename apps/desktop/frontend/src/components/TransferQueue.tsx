import { useEffect, useRef, useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DeviceInfo, QueueItem, TaskSnapshot, WorkspaceSnapshot } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { deviceName, queueLabels } from '../presentation';
import { TaskRow } from './Tasks';

export function transferRows(workspace?: WorkspaceSnapshot) {
  const tasks = new Map((workspace?.tasks ?? []).map(task => [task.id, task]));
  const rows: { id: string; created: string; queue?: QueueItem; task?: TaskSnapshot }[] = [];
  for (const queue of workspace?.queue ?? []) {
    rows.push({ id: 'queue-' + queue.id, created: queue.created_at, queue, task: tasks.get(queue.task_id) });
    tasks.delete(queue.task_id);
  }
  for (const task of tasks.values()) rows.push({ id: 'task-' + task.id, created: task.started_at, task });
  return rows.sort((a, b) => a.created.localeCompare(b.created) || a.id.localeCompare(b.id));
}

export function TransferQueue({ workspace, devices, run, op, available }: { workspace?: WorkspaceSnapshot; devices: DeviceInfo[]; run: CommandRunner; op: string; available: boolean }) {
  const rows = transferRows(workspace);
  const [page, setPage] = useState(0);
  const previousCount = useRef(rows.length);
  useEffect(() => {
    if (rows.length > previousCount.current) setPage(Math.max(0, Math.ceil(rows.length / 10) - 1));
    previousCount.current = rows.length;
  }, [rows.length]);
  const [confirmClear, setConfirmClear] = useState(false);
  const pages = Math.max(1, Math.ceil(rows.length / 10));
  const activePage = Math.min(page, pages - 1);
  const disabled = !!op || !available;
  const completed = rows.some(row => ['completed', 'no_content'].includes(row.task?.state ?? row.queue?.state ?? ''));
  return <section className="surface transfer-queue" aria-labelledby="transfer-queue-title">
    <div className="section-heading compact"><div><h2 id="transfer-queue-title">传输队列 <span className="task-count">{rows.length}</span></h2><small className="field-help">发送与接收都在这里，历史保存在“记录”。</small></div><div className="inline-actions">
      <button className="ghost" disabled={disabled} onClick={() => void run('queue-pause', () => Backend.SetQueuePaused(!workspace?.queue_paused))}>{workspace?.queue_paused ? '继续发送队列' : '暂停发送队列'}</button>
      <button className="secondary" disabled={disabled || !completed} onClick={() => void run('clear-completed', () => Backend.ClearTransferQueue(false), '已清除已完成任务，历史记录保留')}>清除已完成记录</button>
      <button className="ghost danger" disabled={disabled || !rows.length} onClick={() => setConfirmClear(true)}>清除所有任务</button>
    </div></div>
    {workspace?.queue_paused && <p className="queue-notice">发送队列已暂停，当前传输和接收继续。</p>}
    {confirmClear && <div className="queue-notice" role="alert"><strong>清空传输队列？</strong><p>未完成的发送和接收将取消，已接收文件与历史记录保留。</p><div className="inline-actions"><button className="secondary" disabled={disabled} onClick={() => setConfirmClear(false)}>返回</button><button className="primary" disabled={disabled} onClick={() => void run('clear-all', () => Backend.ClearTransferQueue(true), '已取消未完成任务并清空队列，历史记录保留').then(ok => { if (ok) setConfirmClear(false); })}>确认清空</button></div></div>}
    {(workspace?.batches ?? []).filter(batch => !batch.cancelled && batch.completed < batch.total && rows.some(row => row.queue?.batch_id === batch.id)).map(batch => <div className="batch-summary" key={batch.id}><span>分发至 {batch.total} 台设备 · {batch.completed} 台完成</span><button className="ghost danger" disabled={disabled} onClick={() => void run('batch-' + batch.id, () => Backend.CancelBatch(batch.id, batch.revision))}>取消此批次未完成任务</button></div>)}
    {!rows.length ? <div className="empty-state small"><strong>暂时没有传输任务</strong><p>发送文件后会加入这里，收到的文件也会显示进度。</p></div> : <div className="transfer-rows">{rows.slice(activePage * 10, activePage * 10 + 10).map(({ id, queue, task }) => {
      const device = devices.find(device => device.id === (task?.peer_id || queue?.peer_id));
      const pending = queue && !['completed', 'cancelled', 'expired'].includes(queue.state);
      return <div className="transfer-entry" key={id} data-queue-id={queue?.id}>
        {task ? <TaskRow task={task} device={device} run={run} op={op} queueManaged={!!queue} /> : <div className="transfer-waiting"><span className="task-symbol send" aria-hidden="true">↑</span><div><strong>{queue?.source_summary || '文件传输'}</strong><p>发送给 {device ? deviceName(device) : queue?.peer_id.slice(0, 10)}</p></div><span className={'state-badge ' + queue?.state}>{queueLabels[queue?.state ?? ''] || queue?.state}</span></div>}
        {pending && <div className="transfer-queue-actions">{queue.state === 'needs_attention' && <><span className="field-help">{queue.last_error === 'SOURCE_CHANGED' ? '源文件已变化，请核对后继续。' : '请确认后继续此任务。'}</span><button className="primary" disabled={disabled || device?.blocked} onClick={() => void run(queue.id, () => Backend.ConfirmQueue(queue.id, queue.revision))}>确认继续</button></>}<button className="ghost danger" disabled={disabled || task?.state === 'cancel_requested'} onClick={() => void run(queue.id, () => Backend.CancelQueue(queue.id, queue.revision))}>取消</button></div>}
      </div>;
    })}</div>}
    {pages > 1 && <div className="bounded-pagination" aria-label="传输队列分页"><button className="ghost" disabled={activePage === 0} onClick={() => setPage(activePage - 1)}>上一页</button><span>第 {activePage + 1} / {pages} 页</span><button className="ghost" disabled={activePage + 1 >= pages} onClick={() => setPage(activePage + 1)}>下一页</button></div>}
  </section>;
}
