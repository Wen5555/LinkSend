import { useEffect, useRef, useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DeviceInfo, TaskSnapshot } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { connectionMethodLabel, taskPhaseLabel } from '../connection';
import { deviceName, formatBytes, taskLabels } from '../presentation';

export function TaskList({ tasks, devices, run, op }: { tasks: TaskSnapshot[]; devices: DeviceInfo[]; run: CommandRunner; op: string }) {
  return <section className="tasks-section" aria-labelledby="tasks-title">
    <div className="section-heading compact"><div><span className="section-kicker">活动与记录</span><h2 id="tasks-title">任务</h2></div><span className="task-count">{tasks.length} 个</span></div>
    {!tasks.length ? <div className="empty-state"><strong>还没有传输记录</strong><p>加入发送队列，或等待另一台设备发来请求。</p></div> :
      <div className="task-list">{[...tasks].reverse().map(task => <TaskRow key={task.id} task={task} device={devices.find(d => d.id === task.peer_id)} run={run} op={op} />)}</div>}
  </section>;
}

function TaskRow({ task: t, device, run, op }: { task: TaskSnapshot; device?: DeviceInfo; run: CommandRunner; op: string }) {
  const pct = t.total_bytes && t.total_bytes > 0 ? Math.min(100, Math.round(t.processed_bytes / t.total_bytes * 100)) : 0;
  const terminal = ['completed', 'failed', 'cancelled', 'rejected', 'no_content'].includes(t.state);
  return <article className="task-row" id={`task-${t.id}`} tabIndex={-1}>
    <div className={`task-symbol ${t.direction}`} aria-hidden="true">{t.direction === 'send' ? '↑' : '↓'}</div>
    <div className="task-main">
      <div className="task-title"><strong>{t.direction === 'send' ? '发送' : '接收'} · {t.source_summary || t.manifest_summary || '文件传输'}</strong><span className={`state-badge ${t.state}`}>{taskLabels[t.state] || t.state}</span></div>
      {t.manifest_summary && <div className="task-meta">{t.manifest_summary} · {t.file_count || 0} 项</div>}
      <div className="task-meta">{taskPhaseLabel(t.phase)} · {device ? deviceName(device) : t.peer_id?.slice(0, 10) || '等待对端'}{t.connection_method && ` · ${connectionMethodLabel(t.connection_method)}`}{t.rate_bytes_per_second ? ` · ${formatBytes(t.rate_bytes_per_second)}/s` : ''}</div>
      {!terminal && <div className="progress-line" role="progressbar" aria-label="已验证进度" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}><span style={{ width: `${pct}%` }} /></div>}
      <div className="task-accounting"><span>实际{t.direction === 'send' ? '发送' : '接收'} {formatBytes(t.direction === 'send' ? t.sent_bytes : t.received_bytes)}</span>{t.retransmitted_bytes > 0 && <span>其中重传 {formatBytes(t.retransmitted_bytes)}</span>}<span>已验证 {formatBytes(t.verified_bytes)}</span><span>已提交 {formatBytes(t.committed_bytes)}</span><span>{t.bilateral_confirmed ? '双方已确认' : '尚未完成双方确认'}</span></div>
      <div className="task-foot"><span>逻辑完成 {formatBytes(t.processed_bytes)} / {formatBytes(t.total_bytes)}</span>{t.error_message && <span className="task-error">{t.error_code} · {t.error_message}</span>}</div>
    </div>
    <div className="task-actions">
      {t.can_pause && <button className="secondary" disabled={!!op} onClick={() => void run(t.id, () => Backend.PauseTask(t.id))}>暂停传输</button>}
      {t.can_resume && <button className="primary" disabled={!!op || device?.blocked} onClick={() => void run(t.id, () => Backend.ResumeTask(t.id), '正在重新连接并恢复原任务')}>恢复传输</button>}
      {t.can_cancel && t.state !== 'awaiting_acceptance' && <button className="ghost" disabled={!!op} onClick={() => void run(t.id, () => Backend.CancelTask(t.id))}>取消</button>}
      {t.can_retry && <button className="secondary" disabled={!!op || device?.blocked} onClick={() => void run(t.id, () => Backend.RetryTask(t.id))}>重新发送</button>}
      {t.state === 'completed' && t.direction === 'receive' && <button className="secondary" disabled={!!op} onClick={() => void run(t.id, () => Backend.OpenTaskDirectory(t.id))}>打开文件夹</button>}
    </div>
  </article>;
}

export function IncomingConfirm({ task, device, run, op }: { task: TaskSnapshot; device?: DeviceInfo; run: CommandRunner; op: string }) {
  const [remember, setRemember] = useState(false);
  const dialog = useRef<HTMLElement>(null);
  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialog.current?.querySelector<HTMLButtonElement>('button')?.focus();
    return () => previous?.focus();
  }, [task.id]);
  return <div className="modal-backdrop" role="presentation"><section ref={dialog} className="incoming-modal" role="dialog" aria-modal="true" aria-labelledby="incoming-title" onKeyDown={event => {
    if (event.key !== 'Tab') return;
    const controls = Array.from(dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled)') ?? []);
    const first = controls[0], last = controls[controls.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
  }}>
    <div className="incoming-icon" aria-hidden="true">↓</div><p className="eyebrow">收到传输请求</p><h2 id="incoming-title">{device ? deviceName(device) : '对方设备'} 想发送文件</h2>
    <div className="incoming-summary"><strong>{task.manifest_summary || '文件传输'}</strong><span>{task.file_count || 0} 项 · {formatBytes(task.total_bytes)}</span><small>保存到 {task.target_directory}</small></div>
    <label className="remember-choice"><input type="checkbox" checked={remember} disabled={!device?.trusted || device.blocked} onChange={event => setRemember(event.target.checked)} /><span><strong>以后自动接收此设备的文件</strong><small>{device?.trusted ? '可在设备设置中恢复每次确认' : '首次完成传输并建立信任后，可单独开启'}</small></span></label>
    <div className="modal-actions"><button className="ghost danger" disabled={!!op} onClick={() => void run(`reject-${task.id}`, () => Backend.RejectTask(task.id), '已拒绝本次传输')}>拒绝</button><button className="primary" disabled={!!op || device?.blocked} onClick={() => void run(`accept-${task.id}`, () => remember ? Backend.AcceptTaskAlways(task.id) : Backend.AcceptTask(task.id), '已确认接收')}>确认接收</button></div>
  </section></div>;
}
