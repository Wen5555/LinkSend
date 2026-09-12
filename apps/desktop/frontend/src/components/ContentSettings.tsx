import { useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { CommandRunner } from '../hooks/useDesktop';
import { contentError } from './ContentComposer';
import './ContentComposer.css';

type Props = { run: CommandRunner; op: string; available: boolean };
export function ContentSettings({ run, op, available }: Props) {
  const client = useQueryClient();
  const [confirmCleanup, setConfirmCleanup] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false);
  const active = useRef(false);
  const settings = useQuery({ queryKey: ['content', 'settings'], enabled: available, retry: false, queryFn: () => Backend.ContentSettings() });
  const disabled = !available || !!op || busy || !settings.data;
  const execute = async (key: string, action: () => Promise<void>) => {
    if (disabled || active.current) return;
    active.current = true; setBusy(true); setError(''); setNotice('');
    try { await run(key, action, undefined, cause => setError(contentError(cause))); }
    finally { active.current = false; setBusy(false); }
  };
  return <section className="content-settings" aria-label="内容快照设置">
    <h3>发送内容的保留与清理</h3>
    <label className="queue-wait"><input type="checkbox" checked={settings.data?.retain_sent_snapshots ?? false} disabled={disabled} onChange={event => { const retain = event.target.checked; void execute('content-settings', async () => {
      await Backend.SetContentSettings({ retain_sent_snapshots: retain });
      client.setQueryData(['content', 'settings'], { retain_sent_snapshots: retain });
      setNotice(retain ? '已保留发送快照，便于以后重发。' : '已关闭额外保留，可手动清理不再使用的发送快照。');
    }); }} /><span>保留已结束任务的发送内容，便于重发</span></label>
    <p className="content-note">默认关闭。草稿、排队、暂停和可恢复的内容仍会保留；已经清理的内容不能从历史重发。</p>
    <button className="secondary" disabled={disabled} onClick={() => setConfirmCleanup(true)}>清理未使用的发送快照…</button>
    {confirmCleanup && <div className="content-discard" role="group" aria-label="确认清理发送快照"><p>清理不再被使用的发送快照？收到的文件和图片仍保留在接收目录。</p><button className="secondary" disabled={disabled} onClick={() => void execute('content-cleanup', async () => {
      const result = await Backend.CleanupContentSnapshots();
      setNotice(`已清理 ${result.removed} 份发送快照，${result.protected} 份仍被使用或设置保留。`); setConfirmCleanup(false);
    })}>确认清理</button><button className="ghost" disabled={disabled} onClick={() => setConfirmCleanup(false)}>取消</button></div>}
    {settings.isError && <p className="content-error" role="alert">{contentError(settings.error)} <button className="ghost" disabled={!available || busy} onClick={() => void settings.refetch()}>重试读取设置</button></p>}
    {error && <p className="content-error" role="alert">{error}</p>}{notice && <p className="content-notice" role="status">{notice}</p>}
  </section>;
}
