import { useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { ContentActionResult } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { formatBytes } from '../presentation';
import { contentError, contentKindLabel } from './ContentComposer';
import './ContentComposer.css';

type Props = { taskID: string; revision?: number; run: CommandRunner; op: string; available: boolean };
type Action = 'preview' | 'copy' | 'open' | 'save';
export function contentActionNotice(result: ContentActionResult): string {
  if (result.state === 'cancelled') return '已取消保存。';
  if (result.action === 'preview' && result.state === 'submitted') return '已请求打开文字预览。';
  if (result.action === 'open_url' && result.state === 'submitted') return '已请求在浏览器中打开链接。';
  if (result.action === 'copy' && result.state === 'completed') return '文字已复制到剪贴板。';
  if (result.action === 'save_image' && result.state === 'completed') return '图片已保存到所选位置。';
  return '操作结果已返回，请核对内容状态。';
}

export function ReceivedContentActions({ taskID, revision, run, op, available }: Props) {
  // Keying the body prevents confirmation/status from a previous task carrying over.
  return <ContentActionsBody key={taskID} taskID={taskID} revision={revision} run={run} op={op} available={available} />;
}
function ContentActionsBody({ taskID, revision, run, op, available }: Props) {
  const [expanded, setExpanded] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false);
  const active = useRef(false);
  const disabled = !available || !!op || busy;
  const info = useQuery({ queryKey: ['content', 'task', taskID, revision], enabled: available && !!taskID && expanded,
    retry: false, staleTime: Infinity, refetchOnWindowFocus: false, refetchOnReconnect: false,
    queryFn: ({ signal }) => Backend.ContentTask(taskID).cancelOn(signal) });
  const execute = async (action: Action) => {
    if (disabled || active.current) return;
    active.current = true; setBusy(true); setError(''); setNotice('');
    try { await run(`content-${action}-${taskID}`, async () => {
      const calls = { preview: Backend.PreviewReceivedText, copy: Backend.CopyReceivedText, open: Backend.OpenReceivedURL, save: Backend.SaveReceivedImage };
      const result = await calls[action](taskID);
      setNotice(contentActionNotice(result)); setConfirmOpen(false);
    }, undefined, cause => setError(contentError(cause))); }
    finally { active.current = false; setBusy(false); }
  };
  const value = info.data;
  return <div className="content-actions">
    <button className="ghost" disabled={disabled} aria-expanded={expanded} onClick={() => setExpanded(previous => !previous)}>{expanded ? '收起内容操作' : '查看内容操作'}</button>
    {expanded && <div className="content-actions-detail">
      {info.isLoading && <p className="content-note" role="status">正在核对已接收的内容…</p>}
      {info.isError && <p className="content-error" role="alert">{contentError(info.error)}</p>}
      {value && <><p className="content-note">{contentKindLabel(value.kind)} · {formatBytes(value.size)}{value.width && value.height ? ` · ${value.width} × ${value.height}` : ''}{value.mode === 'file' ? ' · 已按普通文件接收' : ''}</p>
        {!value.available && <p className="content-note">内容尚不可操作，请核对接收结果或保存位置。</p>}
        <div className="content-buttons">{value.can_preview && <button className="secondary" disabled={disabled} onClick={() => void execute('preview')}>预览文字</button>}{value.can_copy && <button className="secondary" disabled={disabled} onClick={() => void execute('copy')}>复制文字</button>}{value.can_open && <button className="secondary" disabled={disabled} onClick={() => setConfirmOpen(true)}>打开链接…</button>}{value.can_save && <button className="secondary" disabled={disabled} onClick={() => void execute('save')}>图片另存为…</button>}</div>
        {value.can_preview && <p className="content-note">预览显示前 4096 个字符；复制保留完整文字。</p>}
        {confirmOpen && value.can_open && <div className="content-discard" role="group" aria-label="确认打开收到的链接"><p>在默认浏览器中打开收到的链接？可先预览文字确认网址。</p><button className="primary" disabled={disabled} onClick={() => void execute('open')}>确认打开</button><button className="ghost" disabled={disabled} onClick={() => setConfirmOpen(false)}>取消</button></div>}</>}
      <button className="ghost" disabled={disabled || info.isFetching} onClick={() => void info.refetch()}>重新核对内容</button>
      {error && <p className="content-error" role="alert">{error}</p>}{notice && <p className="content-notice" role="status">{notice}</p>}
    </div>}
  </div>;
}
