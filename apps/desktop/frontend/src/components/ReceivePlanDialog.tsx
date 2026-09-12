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
  const [offset, setOffset] = useState(0);
  const [selection, setSelection] = useState<Set<number> | null>(null);
  const [directory, setDirectory] = useState('');
  const [policy, setPolicy] = useState<ConflictPolicy>(ConflictPolicy.ConflictKeepBoth);
  const [preview, setPreview] = useState<IncomingPlanPreview>();
  const [remember, setRemember] = useState(false);
  const dialog = useRef<HTMLElement>(null);
  const revision = Math.max(task.revision, preview?.revision ?? 0);
  const files = useQuery({ queryKey: ['incoming', task.id, task.attempt_id, revision, offset], retry: false,
    queryFn: ({ signal }) => Backend.IncomingFiles(task.id, { expected_revision: revision, offset, limit: 100 }).cancelOn(signal) });
  const page = files.data;
  const disabled = !!op || !page || files.isFetching;
  const selectionAvailable = !!page?.subset_supported && !page.resume;
  const currentDirectory = directory || preview?.directory || page?.directory || task.target_directory || '';
  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialog.current?.querySelector<HTMLButtonElement>('button')?.focus();
    return () => previous?.focus();
  }, [task.id]);
  const select = (id: number, checked: boolean) => {
    if (!page) return;
    // Manifest.Validate guarantees IDs are 0..TotalEntries-1. Keep only IDs,
    // never fetch chunk hashes or the unbounded manifest into the WebView.
    setSelection(current => {
      const next = new Set(current ?? Array.from({ length: page.total_entries }, (_, index) => index));
      if (checked) next.add(id); else next.delete(id);
      return next;
    });
    setPreview(undefined);
  };
  const buildPlan = () => void run(`plan-${task.id}`, async () => {
    const result = await Backend.IncomingPlan(task.id, { expected_revision: revision, directory: currentDirectory,
      selected_ids: selection === null ? null : [...selection].sort((a, b) => a - b), conflict_policy: policy, policies: {} });
    setPreview(result);
  });
  const acceptPlan = () => {
    if (!preview) return;
    void run(`accept-plan-${task.id}`, async () => {
      await Backend.AcceptReceivePlan(task.id, preview.revision, preview.plan_digest);
      if (remember && task.peer_id) {
        try { await Backend.SetAlwaysAccept(task.peer_id, true); }
        catch { throw new Error('本次已确认接收，但免确认设置未保存；可稍后在设备页修改。'); }
      }
    }, preview.summary.selected_entries ? '已确认接收所选内容' : '已确认跳过全部内容');
  };
  return <div className="modal-backdrop" role="presentation"><section ref={dialog} className="incoming-modal receive-plan-modal" role="dialog" aria-modal="true" aria-labelledby="receive-plan-title" onKeyDown={event => {
    if (event.key !== 'Tab') return;
    const controls = Array.from(dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled)') ?? []);
    const first = controls[0], last = controls[controls.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
  }}>
    <p className="eyebrow">接收确认</p><h2 id="receive-plan-title">{device ? deviceName(device) : '对方设备'} 发来的内容</h2>
    <p className="field-help">原始请求 {page?.total_entries ?? task.file_count ?? 0} 项 · {formatBytes(page?.original_total ?? task.total_bytes)}。确认前不会接收正文。</p>
    {files.isError && <div className="banner error-banner" role="alert"><p>{humanizeBackendError(files.error)}</p><button onClick={() => void files.refetch()}>重试读取</button></div>}
    <div className="receive-plan-toolbar"><label className="field-label">保存到<input value={currentDirectory} readOnly /></label><button className="secondary" disabled={disabled || page?.resume} onClick={() => void run('plan-directory', async () => { const value = await Backend.PickDirectory(); if (value) { setDirectory(value); setPreview(undefined); } })}>更换目录</button></div>
    <label className="field-label">遇到同名文件<select value={policy} disabled={disabled || page?.resume} onChange={event => { setPolicy(event.target.value as ConflictPolicy); setPreview(undefined); }}><option value={ConflictPolicy.ConflictKeepBoth}>保留两份，生成新名称</option><option value={ConflictPolicy.ConflictSkip} disabled={!selectionAvailable}>跳过冲突项</option><option value={ConflictPolicy.ConflictError}>停止确认，由我处理</option></select></label>
    {!page?.subset_supported && page && <p className="field-help">此发送端只支持全量接收；可以保留两份，不能选收或跳过。</p>}
    {page?.resume && <p className="field-help">恢复沿用原接收计划，避免重复提交或改变保存位置。</p>}
    <div className="receive-plan-selection"><strong>{preview ? '拟接收的条目' : '文件与目录条目'}</strong>{preview && <button className="ghost" disabled={!!op} onClick={() => setPreview(undefined)}>修改选择</button>}<button className="ghost" disabled={disabled || !selectionAvailable} onClick={() => { setSelection(null); setPreview(undefined); }}>全选</button><button className="ghost" disabled={disabled || !selectionAvailable} onClick={() => { setSelection(new Set()); setPreview(undefined); }}>全部跳过</button></div>
    <p className="field-help">文件和空目录分别勾选；目录内的文件按各自选择接收。</p>
    <div className="receive-plan-files" aria-label="待接收条目">{page?.files?.map(file => <label className="receive-plan-file" key={file.id}>
      <input type="checkbox" checked={preview ? file.selected : selection === null ? true : selection.has(file.id)} disabled={disabled || !selectionAvailable || !!preview} onChange={event => select(file.id, event.target.checked)} />
      <span><strong>{file.path}</strong>{preview && file.target_path && file.target_path !== file.path && <small>拟保存为 {file.target_path}</small>}{preview && !file.selected && <small>计划跳过</small>}</span><small>{file.type === 'directory' ? '目录' : formatBytes(file.size)}</small>
    </label>)}</div>
    {page && page.total_entries > 100 && <div className="receive-plan-pager"><button disabled={!!op || offset === 0} onClick={() => setOffset(value => Math.max(0, value - 100))}>上一页</button><span>{offset + 1}–{Math.min(offset + 100, page.total_entries)} / {page.total_entries}</span><button disabled={!!op || offset + 100 >= page.total_entries} onClick={() => setOffset(value => value + 100)}>下一页</button></div>}
    <button className="secondary full" disabled={disabled || device?.blocked} onClick={buildPlan}>{op === `plan-${task.id}` ? '正在核对目录、冲突和空间…' : '预览保存计划'}</button>
    {preview && <div className="receive-plan-summary" role="status"><strong>{preview.summary.selected_entries === 0 ? '本次不接收内容' : `接收 ${preview.summary.selected_files} 个文件、${preview.summary.selected_entries - preview.summary.selected_files} 个目录 · ${formatBytes(preview.summary.selected_total)}`}</strong><span>跳过 {preview.summary.skipped_entries} 项 · {formatBytes(preview.summary.skipped_bytes)}</span><span>当前可用空间 {formatBytes(preview.available_bytes)} · 本次预计写入 {formatBytes(preview.required_bytes)}</span>{preview.reserved_bytes > 0 && <span>其中元数据预留 {formatBytes(preview.reserved_bytes)}</span>}{!preview.space_sufficient && <p className="task-error">空间预检不足，接收可能中断。请更换目录或减少选择。</p>}<small>空间检查是预估，实际写入仍会核对磁盘错误。</small></div>}
    <label className="remember-choice"><input type="checkbox" checked={remember} disabled={!!op || !device?.trusted || device.blocked} onChange={event => setRemember(event.target.checked)} /><span>以后自动接收此设备的内容（可在设备页撤销）</span></label>
    <div className="modal-actions"><button className="ghost danger" disabled={!!op} onClick={() => void run(`reject-${task.id}`, () => Backend.RejectTask(task.id), '已拒绝本次请求')}>拒绝</button><button className="primary" disabled={!!op || !preview || files.isFetching || page?.revision !== preview.revision || preview.revision < task.revision || device?.blocked} onClick={acceptPlan}>{preview?.summary.selected_entries === 0 ? '确认全部跳过' : '确认接收所选内容'}</button></div>
  </section></div>;
}
