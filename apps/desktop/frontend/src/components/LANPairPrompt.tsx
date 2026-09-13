import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { CommandRunner } from '../hooks/useDesktop';

export function LANPairPrompt({ run, op, available }: { run: CommandRunner; op: string; available: boolean }) {
  const dialog = useRef<HTMLElement>(null);
  const [dismissed, setDismissed] = useState<Set<string>>(new Set());
  const pending = useQuery({ queryKey: ['lan-pair-pending'], queryFn: () => Backend.PendingLANPairings(), refetchInterval: 1000, retry: false, enabled: available });
  const request = pending.data?.find(item => !dismissed.has(item.request_id));
  useEffect(() => { if (request) dialog.current?.querySelector<HTMLButtonElement>('.primary')?.focus(); }, [request?.request_id]);
  if (!request) return null;
  const respond = (accept: boolean) => void run(`lan-${accept ? 'accept' : 'reject'}-${request.request_id}`, async () => {
    await Backend.RespondLANPair(request.request_id, accept); setDismissed(current => new Set(current).add(request.request_id)); await pending.refetch();
  }, accept ? '已同意添加设备' : '已拒绝添加请求');
  return <div className="modal-backdrop"><section ref={dialog} className="incoming-modal" role="dialog" aria-modal="true" aria-labelledby="lan-pair-title" onKeyDown={event => {
    if (event.key === 'Escape') { respond(false); return; }
    if (event.key !== 'Tab') return;
    const controls = Array.from(dialog.current?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? []);
    const first = controls[0], last = controls.at(-1);
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
  }}><p className="eyebrow">附近设备请求</p><h2 id="lan-pair-title">{request.peer_name || '附近设备'} 想添加此设备</h2><p className="field-help">同意后双方建立局域网信任；文件接收仍按本机策略确认。</p><div className="modal-actions"><button className="ghost danger" disabled={!!op} onClick={() => respond(false)}>拒绝</button><button className="primary" disabled={!!op} onClick={() => respond(true)}>添加设备</button></div></section></div>;
}
