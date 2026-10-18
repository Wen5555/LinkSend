import { useEffect, useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { MembershipRecoveryStatus } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { normalizePairingCode, humanizeBackendError, shouldClearPairingCode } from '../connection';

type Props = { available: boolean; membershipState?: string; name: string; run: CommandRunner; op: string };

export function MembershipRecoveryPanel({ available, membershipState, name, run, op }: Props) {
  const [status, setStatus] = useState<MembershipRecoveryStatus>({ state: 'idle' });
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState('join');
  const [confirmed, setConfirmed] = useState(false);
  const [code, setCode] = useState('');
  const [readError, setReadError] = useState('');
  const pending = ['prepared', 'started', 'paused'].includes(status.state);

  useEffect(() => {
    if (!available) return;
    let current = true;
    void Backend.MembershipRecoveryStatus().then(value => {
      if (!current) return;
      setReadError('');
      setStatus(old => old.state === value.state && old.id === value.id && old.mode === value.mode && old.message === value.message ? old : value);
      if (['prepared', 'started', 'paused'].includes(value.state)) setOpen(true);
    }).catch(cause => { if (current) setReadError(humanizeBackendError(cause)); });
    return () => { current = false; };
  }, [available, membershipState]);

  const reload = async () => {
    try { setStatus(await Backend.MembershipRecoveryStatus()); setReadError(''); }
    catch (cause) { setReadError(humanizeBackendError(cause)); }
  };
  const disabled = !!op || !available;

  return <section className="membership-recovery" aria-label="设备组恢复">
    <button className="ghost" aria-expanded={open} onClick={() => setOpen(value => !value)}>{pending ? '继续设备组恢复' : '服务重建后重新连接'}</button>
    {open && <div className="queue-notice">
      <h3>重新连接设备组</h3>
      <p>仅在确认服务已重建或需要重新配对时使用。网络暂时不可用时，请先检查连接。</p>
      <p>保留本机身份、文件、历史、设置和有效的局域网关系。旧组任务将隔离，免确认和剪贴板权限需要重新开启。</p>
      {readError && <p role="alert">{readError}</p>}
      {pending ? <>
        <p role="status">{status.message}</p>
        <strong>{status.mode === 'initialize' ? '在当前服务新建本机设备组' : '用新配对码加入设备组'}</strong>
        {status.mode === 'join' && <label className="field-label">新的配对码<input className="pairing-input" value={code} onChange={event => setCode(event.target.value)} maxLength={43} autoComplete="off" spellCheck={false} disabled={disabled} /></label>}
        {status.mode === 'join' && status.state !== 'prepared' && <p className="field-help">会先核对上次加入是否已完成；未完成时需要新的配对码。</p>}
        <div className="inline-actions">
          <button className="primary" disabled={disabled || !status.id || status.state === 'prepared' && status.mode === 'join' && !code.trim()} onClick={() => void run('continue-membership-recovery', async () => {
            try { await Backend.ContinueMembershipRecovery(status.id!, normalizePairingCode(code), name || 'LinkSend desktop'); setCode(''); }
            finally { await reload(); }
          }, '已重新连接设备组；旧任务不会自动发送', cause => { if (shouldClearPairingCode(cause)) setCode(''); })}>{status.state === 'prepared' ? '确认连接' : '核对并继续恢复'}</button>
          {status.state !== 'paused' && <button className="secondary" disabled={disabled || !status.id} onClick={() => void run('cancel-membership-recovery', async () => { setStatus(await Backend.CancelMembershipRecovery(status.id!)); setCode(''); }, status.state === 'prepared' ? '已取消准备；旧任务仍需重新确认' : '恢复已暂停；服务端结果保留待核对')}>{status.state === 'prepared' ? '取消准备' : '暂停恢复'}</button>}
        </div>
      </> : <>
        {status.state === 'completed' && <p role="status">上次已重新连接。服务再次重建时，可发起新的恢复。</p>}
        <label className="field-label">恢复方式<select value={mode} onChange={event => setMode(event.target.value)} disabled={disabled}><option value="join">加入另一台设备的新设备组</option><option value="initialize">新建本机设备组</option></select></label>
        <label className="checkbox-label"><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} disabled={disabled} />我确认重新连接，旧任务不会自动继续。</label>
        <button className="secondary" disabled={disabled || !confirmed || !!readError} onClick={() => void run('prepare-membership-recovery', async () => { setStatus(await Backend.PrepareMembershipRecovery(mode, true)); setConfirmed(false); setCode(''); }, '恢复已准备；旧任务已隔离')}>准备恢复</button>
      </>}
    </div>}
  </section>;
}
