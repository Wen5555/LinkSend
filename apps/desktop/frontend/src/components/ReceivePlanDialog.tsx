import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DeviceInfo, IncomingPlanPreview, TaskSnapshot } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { ConflictPolicy } from '../../bindings/github.com/Wen5555/LinkSend/internal/transfer/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { humanizeBackendError } from '../connection';
import { deviceName, formatBytes } from '../presentation';
import './ReceivePlanDialog.css';

type Props = { task: TaskSnapshot; device?: DeviceInfo; run: CommandRunner; op: string };

export function ReceivePlanDialog({ task, device, run, op }: Props) {
  const [advanced, setAdvanced] = useState(false);
  const [selection, setSelection] = useState<Set<number> | null>(null);
  const [directory, setDirectory] = useState('');
  const [policy, setPolicy] = useState<ConflictPolicy>(ConflictPolicy.ConflictKeepBoth);
  const [preview, setPreview] = useState<IncomingPlanPreview>();
  const [remember, setRemember] = useState(false);
  const dialog = useRef<HTMLElement>(null);
  const revision = Math.max(task.revision, preview?.revision ?? 0);
  const files = useQuery({ queryKey: ['incoming', task.id, task.attempt_id, revision], retry: false,
    queryFn: ({ signal }) => Backend.IncomingFiles(task.id, { expected_revision: revision, offset: 0, limit: 100 }).cancelOn(signal) });
  const page = files.data;
  const pageFiles = page?.files ?? [];
  const disabled = !!op || !page || files.isFetching;
  const currentDirectory = directory || preview?.directory || page?.directory || task.target_directory || '';
  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialog.current?.querySelector<HTMLButtonElement>('.primary')?.focus();
    return () => previous?.focus();
  }, [task.id]);
  const acceptDefault = () => void run(`accept-default-${task.id}`, async () => {
    const result = await Backend.AcceptIncomingDefault(task.id, task.attempt_id, task.revision, remember);
    if (result.message) throw new Error(result.message);
  }, '已开始接收文件');
  const buildPlan = () => void run(`plan-${task.id}`, async () => setPreview(await Backend.IncomingPlan(task.id, {
    expected_revision: revision, directory: currentDirectory,
    selected_ids: selection === null ? null : [...selection].sort((a, b) => a - b), conflict_policy: policy, policies: {},
  })));
  const acceptPlan = () => preview && void run(`accept-plan-${task.id}`, async () => {
    await Backend.AcceptReceivePlan(task.id, preview.revision, preview.plan_digest);
    if (remember && task.peer_id) {
      try { await Backend.SetAlwaysAccept(task.peer_id, true); }
      catch { throw new Error('本次已接收，免确认设置未保存；可稍后在设备页修改。'); }
    }
  }, '已开始接收所选内容');
  const names = pageFiles.filter(item => item.type === 'file').slice(0, 3);
  return <div className="modal-backdrop" role="presentation"><section ref={dialog} className="incoming-modal receive-plan-modal" role="dialog" aria-modal="true" aria-labelledby="receive-plan-title" onKeyDown={event => {
    if (event.key === 'Escape' && advanced) { setAdvanced(false); return; }
    if (event.key !== 'Tab') return;
    const controls = Array.from(dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled)') ?? []);
    const first = controls[0], last = controls[controls.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
  }}>
    <p className="eyebrow">接收确认</p><h2 id="receive-plan-title">{device ? deviceName(device) : '对方设备'} 想向你发送文件</h2>
    <p className="incoming-device-state">{device?.trusted ? '已配对设备' : '已验证连接'} · 确认前不会接收文件内容</p>
    <div className="incoming-summary">{names.map(file => <strong key={file.id}>{file.path}</strong>)}{(page?.total_entries ?? 0) > names.length && <span>以及另外 {(page?.total_entries ?? 0) - names.length} 项</span>}<small>共 {page?.total_entries ?? task.file_count ?? 0} 项 · {formatBytes(page?.original_total ?? task.total_bytes)}</small></div>
    <div className="incoming-destination"><span>保存到</span><strong>{currentDirectory || '默认接收目录'}</strong><button className="ghost" disabled={!!op} onClick={() => setAdvanced(value => !value)}>{advanced ? '收起详情' : '查看详情'}</button></div>
    {files.isError && <div className="banner error-banner" role="alert"><p>{humanizeBackendError(files.error)}</p><button onClick={() => void files.refetch()}>重试读取</button></div>}
    {advanced && <div className="receive-advanced">
      <div className="receive-plan-toolbar"><label className="field-label">保存到<input value={currentDirectory} readOnly /></label><button className="secondary" disabled={disabled || page?.resume} onClick={() => void run('plan-directory', async () => { const value = await Backend.PickDirectory(); if (value) { setDirectory(value); setPreview(undefined); } })}>更换目录</button></div>
      <label className="field-label">遇到同名文件<select value={policy} disabled={disabled || page?.resume} onChange={event => { setPolicy(event.target.value as ConflictPolicy); setPreview(undefined); }}><option value={ConflictPolicy.ConflictKeepBoth}>保留两份，生成新名称</option><option value={ConflictPolicy.ConflictSkip} disabled={!page?.subset_supported}>跳过同名项</option><option value={ConflictPolicy.ConflictError}>遇到冲突时停止</option></select></label>
      {page?.resume && <p className="field-help">恢复任务沿用原接收计划和保存位置。</p>}
      <div className="receive-plan-files" aria-label="待接收条目">{pageFiles.map(file => <label className="receive-plan-file" key={file.id}><input type="checkbox" checked={selection === null || selection.has(file.id)} disabled={disabled || !page?.subset_supported || page.resume} onChange={event => { const next = new Set(selection ?? Array.from({ length: page?.total_entries ?? 0 }, (_, index) => index)); if (event.target.checked) next.add(file.id); else next.delete(file.id); setSelection(next); setPreview(undefined); }} /><span><strong>{file.path}</strong></span><small>{file.type === 'directory' ? '目录' : formatBytes(file.size)}</small></label>)}</div>
      {page && page.total_entries > pageFiles.length && <p className="field-help">完整清单共 {page.total_entries} 项；这里显示前100项，未显示的条目保持选中。</p>}
      <button className="secondary full" disabled={disabled} onClick={buildPlan}>{preview ? '重新核对高级计划' : '核对高级计划'}</button>
      {preview && <div className="receive-plan-summary"><strong>将接收 {preview.summary.selected_entries} 项 · {formatBytes(preview.summary.selected_total)}</strong><span>跳过 {preview.summary.skipped_entries} 项</span></div>}
    </div>}
    <label className="remember-choice"><input type="checkbox" checked={remember} disabled={!!op || !device?.trusted || device.blocked} onChange={event => setRemember(event.target.checked)} /><span><strong>以后自动接收此设备发来的文件</strong><small>只影响文件接收，可在设备页关闭。</small></span></label>
    <div className="modal-actions"><button className="ghost danger" disabled={!!op} onClick={() => void run(`reject-${task.id}`, () => Backend.RejectTask(task.id), '已拒绝本次请求')}>拒绝</button>{advanced && preview ? <button className="primary" disabled={!!op || device?.blocked} onClick={acceptPlan}>接收所选内容</button> : <button className="primary" disabled={disabled || device?.blocked} onClick={acceptDefault}>{op === `accept-default-${task.id}` ? '正在安全核对…' : '接收文件'}</button>}</div>
  </section></div>;
}
