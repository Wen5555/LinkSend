import { useQuery } from '@tanstack/react-query';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { ClipboardFileProgress } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { bridgeAvailable } from '../hooks/useDesktop';
import './ClipboardCopyProgress.css';

export function activeLargeCopies(copies: ClipboardFileProgress[]) {
  return copies.filter(copy => copy.large && copy.state !== 'completed' && copy.state !== 'cancelled');
}

export function ClipboardCopyProgressView({ copies, cancel }: { copies: ClipboardFileProgress[]; cancel: (id: string) => void }) {
  const active = activeLargeCopies(copies);
  const copy = active[0];
  const preparing = !copy || copy.state === 'queued' || copy.state === 'preparing';
  const failed = copy?.state === 'failed';
  const percent = copy && copy.total > 0 ? Math.min(100, Math.floor(copy.verified * 100 / copy.total)) : 0;
  return <main className="clipboard-copy-progress" aria-live="polite"><div className="clipboard-copy-symbol" aria-hidden>⇄</div><h1>{failed ? '复制未完成' : '正在复制'}</h1><p>{failed ? '请重新复制文件后再试一次。' : preparing ? '正在准备文件…' : copy.direction === 'receive' ? '正在接收并验证文件…' : '正在传送文件…'}</p><progress aria-label="文件复制进度" max={100} value={preparing || failed ? undefined : percent} /><div className="clipboard-copy-bottom"><span>{failed ? '文件未写入剪贴板' : preparing ? '准备中' : `${percent}%`}{active.length > 1 ? ` · 共 ${active.length} 项` : ''}</span>{copy && <button onClick={() => active.forEach(item => cancel(item.id))}>{failed ? '关闭' : '取消复制'}</button>}</div></main>;
}

export function ClipboardCopyProgress() {
  const query = useQuery({ queryKey: ['clipboard-copy-progress'], queryFn: () => Backend.ClipboardWatcher(), enabled: bridgeAvailable(), refetchInterval: 250, retry: false });
  return <ClipboardCopyProgressView copies={query.data?.files ?? []} cancel={id => void Backend.CancelClipboardFile(id)} />;
}
