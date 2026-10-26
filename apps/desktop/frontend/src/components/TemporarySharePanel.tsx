import { useCallback, useEffect, useRef, useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { SendDraft, TaskSnapshot, TemporaryShareSnapshot } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { formatBytes } from '../presentation';
import { taskPhaseLabel } from '../connection';
import './TemporarySharePanel.css';

type Props = { draft?: SendDraft; tasks: TaskSnapshot[]; available: boolean; refresh: () => Promise<void>; onBusyChange: (busy: boolean) => void; op: string };
type Approval = { id: string; claim: string; key: string; generation: number; peer: string };
const pageSize = 5;
const terminal = (share: TemporaryShareSnapshot) => ['completed', 'stopped'].includes(share.state) || ['completed', 'revoked', 'expired'].includes(share.grant_state ?? '');

export function temporaryShareError(cause: unknown): string {
  const code = String(cause ?? '').match(/\b(?:SHARE_[A-Z_]+|SIGNALING_[A-Z_]+|AUTHENTICATION_FAILED|SOURCE_CHANGED|METADATA_REVISION_CONFLICT|CANCELLED|CLIPBOARD_[A-Z_]+|NATIVE_RUNTIME_UNAVAILABLE|DIRECT_FAILED|RATE_LIMITED|PEER_OFFLINE|CANDIDATE_EXCHANGE_TIMEOUT|NO_CANDIDATES|NO_VIABLE_CANDIDATE|CHECK_TIMEOUT|ICE_FAILED|QUIC_HANDSHAKE_(?:TIMEOUT|FAILED)|SESSION_CONFLICT|CONNECTION_INTERRUPTED)\b/)?.[0];
  const messages: Record<string, string> = {
    SHARE_UNSUPPORTED: '当前服务器尚未启用临时分享。请升级并启用服务后重试；已配对设备的收发仍可使用。',
    SHARE_UNAVAILABLE: '临时分享服务暂时不可用，请稍后刷新状态。',
    SHARE_INVALID: '分享码或请求无效，请核对完整分享码后重试。',
    SHARE_REJECTED: '这次分享无法继续。请核对对方身份和分享状态，必要时由发送方撤销后新建。',
    SHARE_CONFLICT: '分享状态已变化，请刷新并重新核对当前领取人。',
    SHARE_EXPIRED: '这次分享已过期，请由发送方重新创建。',
    SHARE_TERMINAL: '这次分享已经结束，不能再次打开；如需再发请创建新分享。',
    SIGNALING_UNREACHABLE: '无法连接分享服务，请检查服务地址和网络后刷新。',
    SIGNALING_TIMEOUT: '分享协调等待超时，请确认双方仍在本次分享中等待，再刷新原分享状态并手动继续。',
    DIRECT_FAILED: '本次直连或传输未完成。请确认对方仍在等待，刷新原分享状态后再手动继续，避免重复创建或领取。',
    PEER_OFFLINE: '对方尚未参与本次连接，请让对方保持应用打开，并在这次分享中继续后再重试。',
    CANDIDATE_EXCHANGE_TIMEOUT: '连接信息交换未完成，请确认协调服务可用且对方仍在等待后重试原分享。',
    NO_CANDIDATES: '未取得可用的直连地址，请检查绑定网卡和STUN配置后重试原分享。',
    NO_VIABLE_CANDIDATE: '未找到可用的直连路径，请检查双方网络是否允许UDP；当前不提供文件中继。',
    CHECK_TIMEOUT: '直连探测超时，请检查双方网络是否允许UDP，再手动重试原分享。',
    ICE_FAILED: '直连探测失败，请检查绑定网卡和双方网络后重试原分享。',
    QUIC_HANDSHAKE_TIMEOUT: '安全连接握手等待超时，请确认双方仍在线，网络恢复后重试原分享。',
    QUIC_HANDSHAKE_FAILED: '安全连接身份或协议核验未完成，请核对双方身份和版本；不要跳过身份核验。',
    SESSION_CONFLICT: '当前连接正在协调，请先刷新原分享状态，再手动继续。',
    CONNECTION_INTERRUPTED: '文件连接已中断，请保留原文件和接收目录，确认双方恢复后从原分享继续。',
    RATE_LIMITED: '临时分享请求暂时受限，可能是请求频繁或分享记录较多。请稍后核对已有分享状态，再手动重试原操作，避免重复创建或领取。',
    AUTHENTICATION_FAILED: '临时分享的身份核验失败，请核对接收方身份后重试。',
    SOURCE_CHANGED: '源文件已变化，请撤销这次分享，核对内容后重新创建。',
    METADATA_REVISION_CONFLICT: '上方草稿已变化，请刷新后使用当前选择重新创建。',
    CANCELLED: '本次操作已停止。',
    CLIPBOARD_WRITE_FAILED: '分享码未能复制，请稍后重试。',
    CLIPBOARD_UNAVAILABLE: '系统剪贴板暂时不可用，请稍后重试。',
    NATIVE_RUNTIME_UNAVAILABLE: '请在桌面应用中复制分享码。',
  };
  return code && messages[code] || '操作未完成，请刷新分享状态后重试。';
}

function mergeShares(current: TemporaryShareSnapshot[], incoming: TemporaryShareSnapshot[]) {
  const values = [...current];
  for (const share of incoming) {
    const index = values.findIndex(value => value.id === share.id);
    if (index < 0) values.push(share);
    else if (share.revision >= values[index].revision) values[index] = share;
  }
  return values;
}

function stateLabel(share: TemporaryShareSnapshot) {
  if (share.state === 'completed') return '已完成';
  if (share.grant_state === 'completed') return '分享已结束，本机结果待核对';
  if (share.pending_action === 'complete') return '等待核对完成结果';
  const labels: Record<string, string> = { creating: '正在创建', claiming: '正在领取', offered: '等待领取', pending: '等待发送方确认', approved: '身份已确认，等待继续', bound: '已绑定原任务', active: '正在连接或传输', awaiting_save: '等待确认保存', needs_attention: '已保存，等待继续', paused: '已暂停', stopped: '本机已停止', failed: '未完成', revoked: '已撤销', expired: '已过期' };
  if (share.state === 'ready') return labels[share.grant_state ?? ''] ?? '请刷新状态';
  return labels[share.state] ?? labels[share.grant_state ?? ''] ?? '请刷新状态';
}

export function TemporarySharePanel(props: Props) {
  const [open, setOpen] = useState(false);
  return <section className="surface temporary-share" aria-label="一次性临时分享">
    <div className="section-heading compact"><div><h2>一次性临时分享</h2><small className="field-help">双方安装 LinkSend，无需加入同一设备组</small></div><button className="secondary" aria-expanded={open} onClick={() => setOpen(value => !value)}>{open ? '关闭临时分享' : '打开临时分享'}</button></div>
    {open && (props.available ? <TemporaryShareContent {...props} /> : <p role="status">桌面后端尚未就绪，请连接后再试。</p>)}
  </section>;
}

function TemporaryShareContent({ draft, tasks, available, refresh, onBusyChange, op }: Props) {
  const [shares, setShares] = useState<TemporaryShareSnapshot[]>([]);
  const [readError, setReadError] = useState('');
  const [operationError, setOperationError] = useState('');
  const [reading, setReading] = useState(true);
  const [busy, setBusy] = useState('');
  const [page, setPage] = useState(0);
  const [input, setInput] = useState('');
  const [secret, setSecret] = useState<{ id: string; value: string }>();
  const [approval, setApproval] = useState<Approval>();
  const [directories, setDirectories] = useState<Record<string, string>>({});
  const mounted = useRef(true);
  const epoch = useRef(0);
  const readSequence = useRef(0);
  const active = useRef(false);
  const disabled = !!op || !!busy || !available;
  const pages = Math.max(1, Math.ceil(shares.length / pageSize));
  const currentPage = Math.min(page, pages - 1);

  const reload = useCallback(async () => {
    const version = epoch.current;
    const sequence = ++readSequence.current;
    try {
      const values = await Backend.TemporaryShares();
      if (!mounted.current || version !== epoch.current || sequence !== readSequence.current) return;
      setShares(current => mergeShares(current, values ?? []));
      setReadError('');
    } catch (cause) {
      if (mounted.current && version === epoch.current) setReadError(temporaryShareError(cause));
    } finally { if (mounted.current) setReading(false); }
  }, []);

  useEffect(() => {
    mounted.current = true;
    void reload();
    const timer = window.setInterval(() => { if (!active.current) void reload(); }, 1500);
    return () => { mounted.current = false; epoch.current++; window.clearInterval(timer); onBusyChange(false); };
  }, [reload, onBusyChange]);
  useEffect(() => {
    if (secret && shares.some(share => share.id === secret.id && (share.grant_state !== 'offered' || terminal(share)))) setSecret(undefined);
  }, [shares, secret]);

  const apply = (share: TemporaryShareSnapshot) => { if (mounted.current) setShares(current => mergeShares(current, [share])); };
  const actOn = async <T,>(key: string, action: () => Promise<T>, success?: (value: T) => void) => {
    if (active.current || disabled) return;
    active.current = true; setBusy(key); setOperationError(''); onBusyChange(true); epoch.current++;
    const version = epoch.current;
    try {
      // Keep secret-bearing closures out of the application's mutation cache.
      // Only sanitized owner snapshots enter a local view or workspace refresh.
      const value = await action();
      if (mounted.current && epoch.current === version) success?.(value);
      try { await refresh(); }
      catch { if (mounted.current) setOperationError('后端已确认本次操作，但工作台刷新失败。请重读状态，无需重复提交。'); }
    } catch (cause) {
      if (mounted.current && epoch.current === version) setOperationError(temporaryShareError(cause));
    } finally {
      active.current = false;
      if (mounted.current) { setBusy(''); onBusyChange(false); await reload(); }
    }
  };
  const approvalShare = shares.find(share => share.id === approval?.id);
  const sameClaim = !!approval && /^[0-9a-f]{64}$/.test(approval.peer) && !!approvalShare && approvalShare.grant_state === 'pending' && approvalShare.peer_id === approval.peer && approvalShare.claim_id === approval.claim && approvalShare.receiver_public_key === approval.key && approvalShare.generation === approval.generation && !approvalShare.pending_action;
  const pendingClaim = shares.some(share => share.pending_action === 'claim');

  return <div className="temporary-share-body">
    <p className="field-help">只授权这次选中的内容和一个接收人。发送方核对接收身份，接收方确认保存后才传文件；不会开启自动接收或剪贴板权限。</p>
    <div className="temporary-share-forms">
      <div><h3>发送上方已选择的内容</h3><p className="field-help">{draft?.paths?.length ?? 0} 个所选文件或文件夹{draft?.source_selection ? ' · 沿用已保存的发送范围' : ' · 完整发送'}。创建后保留上方草稿。</p>
        <button className="primary" disabled={disabled || !draft?.paths?.length} onClick={() => {
          if (!draft) return;
          const revision = draft.revision;
          setSecret(undefined);
          void actOn('create', () => Backend.CreateTemporaryShare(revision), result => { apply(result.share); setSecret({ id: result.share.id, value: result.code }); setPage(Math.floor(shares.length / pageSize)); });
        }}>{busy === 'create' ? '正在核对内容…' : '创建分享码'}</button>
      </div>
      <form onSubmit={event => {
        event.preventDefault();
        const code = input.trim(); setInput('');
        if (!code) return;
        void actOn('claim', () => Backend.ClaimTemporaryShare(code), apply);
      }}><h3>领取另一台设备的分享</h3><label className="field-label">完整分享码<input aria-label="临时分享码" type="password" value={input} onChange={event => setInput(event.target.value)} maxLength={39} autoComplete="off" spellCheck={false} disabled={disabled} /></label><button className="secondary" type="submit" disabled={disabled || !input.trim() || pendingClaim}>领取分享</button>{pendingClaim && <p className="field-help">有待核对的领取请求，请在下方用原分享码重试。</p>}</form>
    </div>
    {secret && <div className="temporary-share-secret" role="status"><strong>本次分享码</strong><code>{secret.value.match(/.{1,4}/g)?.join('-')}</code><small>只在本次打开中显示。关闭或隐藏后无法找回；可撤销后重新创建。</small><div className="inline-actions"><button className="secondary" disabled={disabled} onClick={() => void actOn('copy-code', () => Backend.CopyTemporaryShareCode(secret.id, secret.value))}>复制分享码</button><button className="ghost" onClick={() => setSecret(undefined)}>隐藏分享码</button></div></div>}
    <div className="temporary-share-toolbar"><h3>本机临时分享</h3><button className="ghost" disabled={disabled} onClick={() => void reload()}>重读本机状态</button></div>
    {reading && <p role="status">正在读取本机分享…</p>}
    {readError && <p className="queue-notice" role="alert">{readError}</p>}
    {operationError && <p className="queue-notice" role="alert">{operationError}</p>}
    {!reading && !shares.length && <p className="field-help">还没有临时分享。领取后请刷新对方确认结果，再明确继续。</p>}
    <div className="temporary-share-list">{shares.slice(currentPage * pageSize, currentPage * pageSize + pageSize).map(share => {
      const task = tasks.find(value => value.id === share.task_record_id);
      const running = share.state === 'active' || share.state === 'awaiting_save' || !!task && !['failed', 'paused', 'recovering', 'cancelled', 'completed'].includes(task.state) && !!task.can_cancel;
      const canContinue = !terminal(share) && !share.pending_action && !running;
      return <article className="temporary-share-row" key={share.id} data-share-id={share.id}>
        <div className="temporary-share-heading"><strong>{share.role === 'sender' ? '发送' : '接收'} · {stateLabel(share)}</strong><small>{share.id.slice(0, 8)}</small></div>
        {!terminal(share) && !!(share.resume_until || share.expires_at) && <p className="field-help">{share.resume_until ? '原任务可恢复至 ' : '分享可领取至 '}{new Date((share.resume_until || share.expires_at)! * 1000).toLocaleString()}</p>}
        {share.role === 'sender' && share.grant_state === 'offered' && secret?.id !== share.id && <p className="field-help">此处无法回读分享码。若分享码已丢失，请撤销后重新创建。</p>}
        {share.peer_id && <p className="temporary-share-peer">对方身份 <code>{share.peer_id}</code></p>}
        {share.error_code && <p className="queue-notice" role="alert">{temporaryShareError(share.error_code)}</p>}
        {share.manifest && <ShareManifest share={share} />}
        {task && <div className="temporary-share-progress" role="status">{taskPhaseLabel(task.phase)} · 已验证 {formatBytes(task.verified_bytes)} / {formatBytes(task.total_bytes)}{task.bilateral_confirmed ? ' · 双方已确认' : ''}</div>}
        {share.state === 'awaiting_save' && share.role === 'receiver' && <div className="temporary-share-save"><strong>确认保存这份清单</strong><span>{directories[share.id] || '尚未选择保存目录'}</span><button className="secondary" disabled={disabled} onClick={() => void actOn(`pick-${share.id}`, () => Backend.PickDirectory(), path => { if (path) setDirectories(current => ({ ...current, [share.id]: path })); })}>选择保存文件夹</button><small>同名内容保留两份；确认后按这次保存计划传输。</small><button className="primary" disabled={disabled || !directories[share.id]} onClick={() => { const directory = directories[share.id]; void actOn(`save-${share.id}`, () => Backend.ConfirmTemporaryShareSave(share.id, directory, 'keep_both'), apply); }}>确认保存并接收</button></div>}
        <div className="inline-actions">
          {!terminal(share) && <button className="ghost" disabled={disabled} onClick={() => void actOn(`refresh-${share.id}`, () => Backend.RefreshTemporaryShare(share.id), apply)}>刷新分享状态</button>}
          {share.role === 'sender' && share.grant_state === 'pending' && !share.pending_action && <><button className="primary" disabled={disabled || !share.claim_id || !share.receiver_public_key || !share.generation || !share.peer_id} onClick={() => setApproval({ id: share.id, claim: share.claim_id!, key: share.receiver_public_key!, generation: share.generation!, peer: share.peer_id ?? '' })}>核对接收人</button><button className="ghost danger" disabled={disabled} onClick={() => { const { id, claim_id, generation } = share; void actOn(`deny-${id}`, () => Backend.DenyTemporaryShare(id, claim_id!, generation!), apply); }}>拒绝本次领取</button></>}
          {canContinue && share.grant_state === 'approved' && <button className="primary" disabled={disabled} onClick={() => void actOn(`start-${share.id}`, () => Backend.StartTemporaryShare(share.id))}>{share.role === 'receiver' ? '读取加密文件清单' : '继续已批准的发送'}</button>}
          {canContinue && share.grant_state === 'bound' && <button className="primary" disabled={disabled} onClick={() => void actOn(`resume-${share.id}`, () => Backend.ResumeTemporaryShare(share.id))}>恢复原分享任务</button>}
          {share.pending_action && share.pending_action !== 'claim' && <button className="secondary" disabled={disabled} onClick={() => void actOn(`retry-${share.id}`, () => Backend.RetryTemporaryShareAction(share.id), apply)}>核对并重试原操作</button>}
          {share.pending_action === 'claim' && <button className="secondary" disabled={disabled || !input.trim()} onClick={() => { const code = input.trim(); setInput(''); void actOn(`retry-claim-${share.id}`, () => Backend.RetryTemporaryShareClaim(share.id, code), apply); }}>用上方分享码重试领取</button>}
          {!terminal(share) && share.role === 'sender' && <button className="ghost danger" disabled={disabled} onClick={() => { setSecret(value => value?.id === share.id ? undefined : value); void actOn(`revoke-${share.id}`, () => Backend.RevokeTemporaryShare(share.id), apply); }}>撤销分享</button>}
          {!terminal(share) && <button className="ghost danger" disabled={disabled} onClick={() => void actOn(`cancel-${share.id}`, () => Backend.CancelTemporaryShare(share.id), apply)}>{share.role === 'receiver' ? '取消本机接收' : '停止本机发送'}</button>}
        </div>
      </article>;
    })}</div>
    {pages > 1 && <div className="bounded-pagination" aria-label="临时分享分页"><button className="ghost" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>分享上一页</button><span>{currentPage + 1} / {pages} 页</span><button className="ghost" disabled={currentPage + 1 >= pages} onClick={() => setPage(currentPage + 1)}>分享下一页</button></div>}
    {approval && <section className="temporary-share-approval" aria-label="核对临时接收身份"><h3>只向这一个接收人发送</h3><p>请与对方核对完整身份指纹，再确认发送。</p><code>{approval.peer}</code>{!sameClaim && <p role="alert">领取对象或状态已变化，请返回后重新核对。</p>}<div className="inline-actions"><button className="ghost" disabled={disabled} onClick={() => setApproval(undefined)}>返回</button><button className="primary" disabled={disabled || !sameClaim} onClick={() => {
      const selected = approval;
      void actOn(`approve-${selected.id}`, async () => { const snapshot = await Backend.ApproveTemporaryShare(selected.id, selected.claim, selected.key, selected.generation); apply(snapshot); await Backend.StartTemporaryShare(selected.id); }, () => setApproval(undefined));
    }}>确认接收人并发送</button></div></section>}
  </div>;
}

function ShareManifest({ share }: { share: TemporaryShareSnapshot }) {
  const [page, setPage] = useState(0);
  const files = share.manifest?.files ?? [];
  const pages = Math.max(1, Math.ceil(files.length / 10));
  const current = Math.min(page, pages - 1);
  const total = files.reduce((sum, file) => sum + (file.type === 'file' ? file.size : 0), 0);
  return <details className="temporary-share-manifest"><summary>这份清单 · {files.length} 项 · {formatBytes(total)}</summary><ul>{files.slice(current * 10, current * 10 + 10).map(file => <li key={file.id}><span>{file.path}{file.type === 'directory' ? '/' : ''}</span><small>{file.type === 'file' ? formatBytes(file.size) : '目录'}</small></li>)}</ul>{pages > 1 && <div className="bounded-pagination"><button className="ghost" disabled={current === 0} onClick={() => setPage(current - 1)}>清单上一页</button><span>{current + 1} / {pages}</span><button className="ghost" disabled={current + 1 >= pages} onClick={() => setPage(current + 1)}>清单下一页</button></div>}</details>;
}
