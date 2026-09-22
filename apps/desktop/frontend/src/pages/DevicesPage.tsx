import { useCallback, useEffect, useRef, useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { ClipboardGrant, DeviceInfo, DeviceProfile, InvitationInfo, MembershipStatus } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { formatPairingCodeInput, shouldClearPairingCode } from '../connection';
import { deviceConnectionLabel, deviceName } from '../presentation';

type Props = { devices: DeviceInfo[]; identityID?: string; membership?: MembershipStatus; name: string; run: CommandRunner; op: string; available: boolean };
type DirectoryKind = 'paired' | 'nearby' | 'removed';
type CloseReason = 'close' | 'removed';
type Pages = Record<DirectoryKind, number>;

const pageSize = 10;
const emptyPages: Pages = { paired: 0, nearby: 0, removed: 0 };

export const lanPairServerMessage = (state: string) => state === 'joined' ? '跨网络设备组也已同步完成。' : state === 'switch_required' ? '局域网可立即使用；跨网络使用前需输入对方配对码并确认切换设备组。' : state === 'pending' ? '局域网可立即使用；跨网络使用请在服务恢复后输入对方配对码继续。' : '当前仅建立局域网信任。';

export function matchesDeviceSearch(device: DeviceInfo, query: string) {
  const needle = query.trim().toLocaleLowerCase();
  if (!needle) return true;
  return [deviceName(device), device.name, device.profile.alias, device.id].some(value => value.toLocaleLowerCase().includes(needle));
}

export function clampDevicePage(page: number, itemCount: number) {
  return Math.max(0, Math.min(page, Math.max(0, Math.ceil(itemCount / pageSize) - 1)));
}

export function LANPairResultNotice({ name, server, onSwitch }: { name: string; server: string; onSwitch: () => void }) {
  return <div className="queue-notice" role="status"><strong>{name} 已完成局域网添加</strong><p>{lanPairServerMessage(server)}</p>{server === 'switch_required' && <button className="secondary" onClick={onSwitch}>输入配对码并切换</button>}</div>;
}

export function DevicesPage({ devices, identityID, membership, name, run, op, available }: Props) {
  const [invite, setInvite] = useState<InvitationInfo>();
  const [code, setCode] = useState('');
  const [address, setAddress] = useState('');
  const [switchCode, setSwitchCode] = useState('');
  const [lanResult, setLanResult] = useState<{ name: string; server: string }>();
  const [query, setQuery] = useState('');
  const [pages, setPages] = useState<Pages>(emptyPages);
  const [detailDeviceID, setDetailDeviceID] = useState<string>();
  const pairingCode = useRef<HTMLInputElement>(null);
  const searchInput = useRef<HTMLInputElement>(null);
  const detailTrigger = useRef<HTMLButtonElement | null>(null);

  const paired = devices.filter(device => device.id !== identityID && device.trusted && !device.blocked);
  const nearby = devices.filter(device => device.id !== identityID && device.nearby && !device.trusted && !device.blocked);
  const removed = devices.filter(device => device.id !== identityID && device.blocked && device.relationship === 'removed');
  const directory = {
    paired: paired.filter(device => matchesDeviceSearch(device, query)),
    nearby: nearby.filter(device => matchesDeviceSearch(device, query)),
    removed: removed.filter(device => matchesDeviceSearch(device, query)),
  };
  const selected = detailDeviceID ? devices.find(device => device.id === detailDeviceID) : undefined;

  useEffect(() => {
    setPages(current => {
      const next: Pages = {
        paired: clampDevicePage(current.paired, directory.paired.length),
        nearby: clampDevicePage(current.nearby, directory.nearby.length),
        removed: clampDevicePage(current.removed, directory.removed.length),
      };
      return next.paired === current.paired && next.nearby === current.nearby && next.removed === current.removed ? current : next;
    });
  }, [directory.paired.length, directory.nearby.length, directory.removed.length]);

  useEffect(() => {
    if ((!selected || (selected.blocked && selected.relationship === 'removed')) && detailDeviceID) {
      setDetailDeviceID(undefined);
      requestAnimationFrame(() => searchInput.current?.focus());
    }
  }, [detailDeviceID, selected]);

  const closeDetail = (reason: CloseReason) => {
    const trigger = detailTrigger.current;
    setDetailDeviceID(undefined);
    requestAnimationFrame(() => {
      if (reason === 'close' && trigger?.isConnected) trigger.focus();
      else searchInput.current?.focus();
    });
  };
  const openDetail = (deviceID: string, trigger: HTMLButtonElement) => {
    detailTrigger.current = trigger;
    setDetailDeviceID(deviceID);
  };
  const changeQuery = (value: string) => {
    setQuery(value);
    setPages(emptyPages);
  };
  const changePage = (kind: DirectoryKind, page: number) => {
    setPages(current => ({ ...current, [kind]: clampDevicePage(page, directory[kind].length) }));
  };

  return <div className="page-stack">
    <div className="page-intro"><div><p className="eyebrow">可信设备</p><h2>设备</h2><p>配对关系与当前连接状态分开显示。</p></div>{membership?.state === 'not_member' ? <button className="primary" disabled={!!op} onClick={() => void run('initialize', () => Backend.InitializeMembership(name || 'LinkSend desktop'), '本机设备组已初始化')}>初始化本机</button> : <button className="primary" disabled={!!op || membership?.state !== 'member'} onClick={() => void run('invite', async () => setInvite(await Backend.CreateInvitation()))}>生成配对码</button>}</div>
    {invite && <InvitationPanel key={invite.token} invite={invite} run={run} />}
    <div className="device-columns">
      <section className="surface device-directory" aria-label="设备目录">
        <div className="section-heading compact"><div><h2>设备目录</h2><small className="field-help">搜索结果按类别分页显示。</small></div><span className="task-count">{paired.length + nearby.length + removed.length} 台</span></div>
        <label className="device-search"><span>查找设备</span><input ref={searchInput} aria-label="按名称、别名或身份查找设备" value={query} onChange={event => changeQuery(event.target.value)} placeholder="名称、别名或完整身份" /></label>
        <DeviceDirectorySection kind="paired" title="已配对设备" total={paired.length} devices={directory.paired} page={pages.paired} query={query} available={available} op={op} run={run} onPage={changePage} onOpen={openDetail} />
        <DeviceDirectorySection kind="nearby" title="添加附近设备" total={nearby.length} devices={directory.nearby} page={pages.nearby} query={query} available={available} op={op} run={run} onPage={changePage} onOpen={openDetail} onLANResult={setLanResult} />
        <DeviceDirectorySection kind="removed" title="已删除设备" total={removed.length} devices={directory.removed} page={pages.removed} query={query} available={available} op={op} run={run} onPage={changePage} onOpen={openDetail} />
        {lanResult && <LANPairResultNotice name={lanResult.name} server={lanResult.server} onSwitch={() => { pairingCode.current?.focus(); pairingCode.current?.scrollIntoView({ block: 'center' }); }} />}
      </section>
      <section className="surface pairing-form"><span className="section-kicker">连接另一台设备</span><h2>跨网络配对</h2><p className="muted">输入一次性配对码建立信任，接收时仍按本机策略确认。</p>
        <form onSubmit={event => {
          event.preventDefault();
          void run('join', async () => { try { await Backend.PairDevice(code.trim(), name || 'LinkSend desktop'); setCode(''); } catch (cause) { if (String(cause).includes('PAIRING_IDENTITY_CONFLICT')) setSwitchCode(code.trim()); throw cause; } }, '配对成功', cause => { if (shouldClearPairingCode(cause)) setCode(''); });
        }}><label className="field-label">配对码<input ref={pairingCode} className="pairing-input" value={code} onChange={event => setCode(formatPairingCodeInput(event.target.value))} placeholder="ABCD-EFGH" maxLength={43} autoComplete="off" spellCheck={false} /></label><button className="primary full" disabled={!!op || !code.trim() || !available}>完成配对</button></form>
        {switchCode && <div className="queue-notice"><strong>本机已属于另一设备组</strong><p>切换会离开当前设备组；历史记录和已接收文件保留。</p><button className="secondary" disabled={!!op} onClick={() => void run('switch-membership', async () => { await Backend.SwitchMembership(switchCode, name || 'LinkSend desktop'); setSwitchCode(''); setCode(''); }, '已切换并完成配对')}>确认切换设备组</button></div>}
        {membership?.message && <p className="field-help pairing-help">{membership.message}</p>}
        <div className="divider" /><h3>附近设备没有出现？</h3><p className="field-help">输入对方的局域网 IPv4 地址，只查找这一台设备。</p>
        <form onSubmit={event => { event.preventDefault(); void run('lan-probe', () => Backend.ProbeLANAddress(address.trim()), '已发送定向发现请求'); }}><label className="field-label">对方局域网地址<input value={address} onChange={event => setAddress(event.target.value)} placeholder="例如 192.168.1.20" inputMode="decimal" spellCheck={false} /></label><button className="secondary full" disabled={!!op || !address.trim() || !available}>查找设备</button></form>
      </section>
    </div>
    {selected && <DeviceDetailDrawer key={selected.id} device={selected} run={run} op={op} onClose={closeDetail} />}
  </div>;
}

function DeviceDirectorySection({ kind, title, total, devices, page, query, available, op, run, onPage, onOpen, onLANResult }: { kind: DirectoryKind; title: string; total: number; devices: DeviceInfo[]; page: number; query: string; available: boolean; op: string; run: CommandRunner; onPage: (kind: DirectoryKind, page: number) => void; onOpen: (id: string, trigger: HTMLButtonElement) => void; onLANResult?: (result: { name: string; server: string }) => void }) {
  const pageCount = Math.max(1, Math.ceil(devices.length / pageSize));
  const activePage = clampDevicePage(page, devices.length);
  const visible = devices.slice(activePage * pageSize, activePage * pageSize + pageSize);
  const hasNoDevices = total === 0;
  const hasNoMatches = !hasNoDevices && visible.length === 0;
  return <section className={'device-directory-section ' + kind} aria-labelledby={'device-section-' + kind}>
    <div className="section-heading compact"><h3 id={'device-section-' + kind}>{title}</h3><span className="task-count">{total} 台{query && ' · 匹配 ' + devices.length + ' 台'}</span></div>
    {hasNoDevices && <div className="empty-state small"><strong>{kind === 'paired' ? '还没有已配对设备' : kind === 'nearby' ? '没有附近设备' : '没有已删除设备'}</strong><p>{kind === 'paired' ? '可输入配对码，或从附近设备发起添加。' : kind === 'nearby' ? '在另一台设备打开 LinkSend 后会出现在这里。' : '已撤销的设备会保留在这里，避免刷新后误认为重新配对。'}</p></div>}
    {hasNoMatches && <p className="field-help device-search-empty">没有匹配的设备。</p>}
    {visible.map(device => <DeviceListRow key={device.id} kind={kind} device={device} available={available} op={op} run={run} onOpen={onOpen} onLANResult={onLANResult} />)}
    {!hasNoDevices && <div className="device-pagination" aria-label={title + '分页'}><button className="ghost" aria-label={title + '上一页'} disabled={activePage === 0} onClick={() => onPage(kind, activePage - 1)}>上一页</button><span>第 {activePage + 1} / {pageCount} 页</span><button className="ghost" aria-label={title + '下一页'} disabled={activePage + 1 >= pageCount} onClick={() => onPage(kind, activePage + 1)}>下一页</button></div>}
  </section>;
}

function DeviceListRow({ kind, device, available, op, run, onOpen, onLANResult }: { kind: DirectoryKind; device: DeviceInfo; available: boolean; op: string; run: CommandRunner; onOpen: (id: string, trigger: HTMLButtonElement) => void; onLANResult?: (result: { name: string; server: string }) => void }) {
  const label = deviceName(device);
  const identity = device.id.slice(0, 12);
  const pendingRevoke = kind === 'removed' && device.service_state === 'pending_revoke_sync';
  const [confirmRevoke, setConfirmRevoke] = useState(false);
  const retryButton = useRef<HTMLButtonElement>(null);
  const hadConfirmation = useRef(false);
  const actionsDisabled = !!op || !available;
  useEffect(() => { setConfirmRevoke(false); }, [device.id, device.group_id, device.incarnation, device.relationship, pendingRevoke]);
  useEffect(() => {
    if (!confirmRevoke && hadConfirmation.current) retryButton.current?.focus();
    hadConfirmation.current = confirmRevoke;
  }, [confirmRevoke]);
  const relationship = kind === 'removed' ? pendingRevoke ? '已移除 · 待同步撤销' : '已移除' : device.blocked ? '已屏蔽' : device.trusted ? '已配对' : '尚未配对';
  return <article className={'device-entry ' + (device.blocked ? 'device-blocked' : '')} data-device-id={device.id}>
    <div className="device-row"><span className="avatar" aria-hidden="true">{label[0] || '?'}</span><div><strong>{label}</strong>{device.profile.alias && <small>对方名称：{device.name || '未命名设备'}</small>}<small>{relationship} · {deviceConnectionLabel(device)}</small><div className="device-tags">{device.profile.my_device && <span>我的设备</span>}{device.profile.pinned && <span>固定 · {device.profile.position}</span>}{device.always_accept && !device.blocked && <span>免确认接收</span>}</div><code>身份 {identity}</code></div>
      {kind === 'paired' && <button className="ghost" aria-label={'打开 ' + label + ' 的设备详情，身份 ' + identity} disabled={!available} onClick={event => onOpen(device.id, event.currentTarget)}>详情</button>}
      {kind === 'nearby' && <button className="secondary" aria-label={'请求添加 ' + label + '，身份 ' + identity} disabled={!!op} onClick={() => void run('lan-add-' + device.id, async () => { const result = await Backend.RequestLANPair(device.id); onLANResult?.({ name: device.name || '附近设备', server: result.server_state }); })}>请求添加</button>}
      {kind === 'removed' && <span className="inline-actions">{pendingRevoke && !confirmRevoke && <button ref={retryButton} className="secondary" aria-label={'继续撤销 ' + label + '，身份 ' + identity} disabled={actionsDisabled} onClick={() => setConfirmRevoke(true)}>继续撤销</button>}<button className="secondary" aria-label={'允许重新添加 ' + label + '，身份 ' + identity} disabled={actionsDisabled || confirmRevoke} onClick={() => void run('allow-readd-' + device.id, () => Backend.UnblockDevice(device.id), '已允许重新添加；请重新发起配对')}>允许重新添加</button></span>}
    </div>
    {pendingRevoke && confirmRevoke && <div className="queue-notice device-remove-confirm" role="alert"><div><strong>继续撤销此设备？</strong><p>本机已移除该设备，服务端尚未确认。继续将再次提交删除请求，历史记录和已接收文件会保留。</p></div><div className="inline-actions"><button className="ghost danger" aria-label={'确认继续撤销 ' + label + '，身份 ' + identity} disabled={actionsDisabled} onClick={() => void run('retry-revoke-' + device.id, () => Backend.RemoveDevice(device.id), '已完成撤销同步').then(ok => { if (ok) setConfirmRevoke(false); })}>确认继续撤销</button><button autoFocus className="secondary" aria-label={'取消继续撤销 ' + label + '，身份 ' + identity} disabled={!!op} onClick={() => setConfirmRevoke(false)}>取消</button></div></div>}
  </article>;
}

function DeviceDetailDrawer({ device, run, op, onClose }: { device: DeviceInfo; run: CommandRunner; op: string; onClose: (reason: CloseReason) => void }) {
  const closeButton = useRef<HTMLButtonElement>(null);
  const drawer = useRef<HTMLElement>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [submitProfile, setSubmitProfile] = useState<(() => void)>();
  const [profileSaving, setProfileSaving] = useState(false);
  useEffect(() => { closeButton.current?.focus(); }, []);
  const label = deviceName(device);
  const identity = device.id.slice(0, 12);
  const registerProfileSubmit = useCallback((submit?: () => void) => setSubmitProfile(() => submit), []);
  return <div className="device-detail-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) onClose('close'); }}>
    <aside ref={drawer} className="device-detail-drawer" role="dialog" aria-modal="true" aria-labelledby={'device-detail-title-' + device.id} onKeyDown={event => {
      if (event.key === 'Escape') { event.preventDefault(); onClose('close'); return; }
      if (event.key !== 'Tab') return;
      const controls = Array.from(drawer.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])') ?? []);
      const first = controls[0], last = controls.at(-1);
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    }}>
      <div className="device-detail-heading"><div><span className="section-kicker">设备详情</span><h2 id={'device-detail-title-' + device.id}>{label}</h2><code>身份 {identity}</code></div><button ref={closeButton} className="ghost" aria-label={'关闭 ' + label + ' 的设备详情'} onClick={() => onClose('close')}>关闭</button></div>
      <div className="device-detail-content"><DeviceEditor device={device} run={run} op={op} onSubmitReady={registerProfileSubmit} onSavingChange={setProfileSaving} /></div>
      <footer className="device-detail-footer">
        {confirmRemove ? <div className="device-remove-confirm" role="alert"><div><strong>删除此设备？</strong><p>将撤销本机授权并停止未完成传输；历史记录和已接收文件会保留。</p></div><div className="inline-actions"><button className="ghost danger" aria-label={'确认删除 ' + label + '，身份 ' + identity} disabled={!!op} onClick={() => void run('remove-' + device.id, () => Backend.RemoveDevice(device.id), '设备已删除；服务不可用时会在恢复后继续同步').then(removed => { if (removed) onClose('removed'); })}>确认删除</button><button className="secondary" disabled={!!op} onClick={() => setConfirmRemove(false)}>取消</button></div></div> : <div className="device-detail-actions"><button type="button" className="primary" disabled={!!op || profileSaving || !submitProfile} onClick={() => submitProfile?.()}>保存设备偏好</button><button className="ghost danger" aria-label={'删除 ' + label + '，身份 ' + identity} disabled={!!op || profileSaving} onClick={() => setConfirmRemove(true)}>删除设备</button></div>}
      </footer>
    </aside>
  </div>;
}

function DeviceEditor({ device, run, op, onSubmitReady, onSavingChange }: { device: DeviceInfo; run: CommandRunner; op: string; onSubmitReady: (submit?: () => void) => void; onSavingChange: (saving: boolean) => void }) {
  const initialProfile = () => ({ ...device.profile, peer_id: device.id });
  const [profile, setProfile] = useState<DeviceProfile>(initialProfile);
  const [dirty, setDirty] = useState(false);
  const [savingProfile, setSavingProfile] = useState(false);
  const [clipboardGrants, setClipboardGrants] = useState<Record<string, ClipboardGrant>>({});
  const [clipboardLoading, setClipboardLoading] = useState(true);
  const [clipboardError, setClipboardError] = useState('');
  const update = (patch: Partial<DeviceProfile>) => {
    setDirty(true);
    setProfile(current => ({ ...current, ...patch, peer_id: device.id }));
  };
  useEffect(() => {
    const latest = initialProfile();
    setProfile(current => {
      if (current.peer_id !== device.id) return latest;
      if (dirty || latest.revision <= current.revision) return current;
      return latest;
    });
  }, [device.id, device.profile.revision, dirty]);
  useEffect(() => {
    let current = true;
    setClipboardLoading(true);
    setClipboardError('');
    void Backend.ClipboardGrants(device.id).then(grants => {
      if (current) setClipboardGrants(Object.fromEntries((grants ?? []).map(grant => [grant.direction + '/' + grant.kind, grant])));
    }).catch(cause => {
      if (current) setClipboardError(String(cause));
    }).finally(() => {
      if (current) setClipboardLoading(false);
    });
    return () => { current = false; };
  }, [device.id]);
  const setClipboardGrant = (direction: 'send' | 'receive', kind: 'text' | 'link' | 'image', enabled: boolean) => {
    const key = direction + '/' + kind;
    const current = clipboardGrants[key];
    void run('clipboard-' + device.id + '-' + direction + '-' + kind, async () => {
      const saved = await Backend.SetClipboardGrant({ peer_id: device.id, direction, kind, enabled, expected_revision: current?.revision ?? 0 });
      setClipboardGrants(previous => ({ ...previous, [key]: saved }));
      setClipboardError('');
    }, enabled ? '自动剪贴板权限已开启' : '自动剪贴板权限已关闭', cause => {
      setClipboardError(String(cause));
      void Backend.ClipboardGrants(device.id).then(grants => setClipboardGrants(Object.fromEntries((grants ?? []).map(grant => [grant.direction + '/' + grant.kind, grant]))));
    });
  };
  const submitProfile = useCallback(() => {
    if (savingProfile) return;
    setSavingProfile(true); onSavingChange(true);
    void run('profile-' + device.id, async () => {
      const saved = await Backend.SaveDeviceProfile(profile);
      setProfile({ ...saved, peer_id: device.id });
      setDirty(false);
    }, '设备偏好已保存').finally(() => { setSavingProfile(false); onSavingChange(false); });
  }, [device.id, onSavingChange, profile, run, savingProfile]);
  useEffect(() => {
    onSubmitReady(submitProfile);
    return () => onSubmitReady(undefined);
  }, [onSubmitReady, submitProfile]);
  return <div className="device-editor">
    <form onSubmit={event => { event.preventDefault(); submitProfile(); }}><fieldset className="device-profile-fields" disabled={savingProfile}>
      <label className="field-label">本机别名<input value={profile.alias} maxLength={80} onChange={event => update({ alias: event.target.value })} placeholder={device.name || '未命名设备'} /></label>
      <div className="preference-checks"><label><input type="checkbox" checked={profile.my_device} disabled={!device.trusted || device.blocked} onChange={event => update({ my_device: event.target.checked })} />我的设备</label><label><input type="checkbox" checked={profile.pinned} onChange={event => update({ pinned: event.target.checked })} />固定在前面</label></div>
      {profile.pinned && <label className="field-label">固定顺序（较小的排在前面）<input type="number" min={0} max={9999} value={profile.position} onChange={event => update({ position: Math.max(0, Number(event.target.value)) })} /></label>}
      <label className="field-label">此设备的接收目录<input value={profile.receive_directory} readOnly placeholder="继承全局接收目录" /></label><div className="inline-actions"><button type="button" className="secondary" disabled={!!op} onClick={() => void run('pick-device-directory', async () => { const path = await Backend.PickDirectory(); if (path) update({ receive_directory: path }); })}>选择目录</button><button type="button" className="ghost" disabled={!profile.receive_directory} onClick={() => update({ receive_directory: '' })}>继承全局目录</button></div>
      <label className="field-label">同名文件策略<select value={profile.conflict_policy} onChange={event => update({ conflict_policy: event.target.value })}><option value="">继承全局设置</option><option value="keep_both">保留两份</option><option value="skip">跳过同名文件</option><option value="error">遇到同名文件时停止</option></select></label>
      <p className="field-help">保存后用于新接收任务；正在传输的文件保持原保存位置。</p>
      {dirty && device.profile.revision !== profile.revision && <p className="queue-notice">设备快照已更新；当前未保存编辑仍保留。保存时会按该设备的修订版本重新核验。</p>}
    </fieldset></form>
    <div className="device-permissions"><section className="clipboard-permissions" aria-labelledby={'clipboard-' + device.id}><div><strong id={'clipboard-' + device.id}>自动剪贴板</strong><p className="field-help">只同步启用的方向与类型。双方对应方向均开启且在线后生效；离线、锁屏或暂停期间的内容不会补发。</p></div>
      {clipboardLoading ? <p className="field-help">正在读取权限…</p> : <div className="clipboard-grant-grid">{(['send', 'receive'] as const).map(direction => <fieldset key={direction}><legend>{direction === 'send' ? '发送给此设备' : '接收此设备内容'}</legend>{([['text', '文字'], ['link', '链接'], ['image', '图片']] as const).map(([kind, label]) => { const grant = clipboardGrants[direction + '/' + kind]; return <label key={kind}><input type="checkbox" checked={grant?.enabled ?? false} disabled={!!op || !device.trusted || device.blocked} onChange={event => setClipboardGrant(direction, kind, event.target.checked)} />{label}</label>; })}</fieldset>)}</div>}
      {clipboardError && <p className="field-help error-text" role="alert">权限读取或保存失败，请重试。</p>}
    </section><label className="remember-choice"><input type="checkbox" checked={device.always_accept && !device.blocked} disabled={!!op || !device.trusted || device.blocked} onChange={event => {
      const enabled = event.target.checked;
      void run('consent-' + device.id, () => Backend.SetAlwaysAccept(device.id, enabled), enabled ? '已开启此设备免确认接收' : '已恢复每次接收确认');
    }} /><span><strong>免确认接收</strong><small>与收藏和“我的设备”标签独立，仍验证设备身份与文件完整性。</small></span></label>
    </div>
  </div>;
}

function InvitationPanel({ invite, run }: { invite: InvitationInfo; run: CommandRunner }) {
  const [remaining, setRemaining] = useState(Math.max(0, invite.expires_in_seconds));
  useEffect(() => {
    const started = Date.now();
    const initial = Math.max(0, invite.expires_in_seconds);
    const timer = window.setInterval(() => setRemaining(Math.max(0, initial - Math.floor((Date.now() - started) / 1000))), 1000);
    return () => window.clearInterval(timer);
  }, [invite.expires_in_seconds]);
  return <div className={'invite-panel pairing-code-panel ' + (remaining ? '' : 'expired')}><div><span className="section-kicker">{remaining ? Math.floor(remaining / 60) + ':' + String(remaining % 60).padStart(2, '0') + ' 后过期 · 单次使用' : '配对码已过期，请重新生成'}</span><p className="pairing-code">{invite.token}</p><small>在另一台设备输入配对码</small></div><button className="secondary" disabled={!remaining} onClick={() => void run('copy-invite', async () => { if (!navigator.clipboard) throw new Error('当前窗口无法访问剪贴板'); await navigator.clipboard.writeText(invite.token); }, '已复制配对码')}>复制配对码</button></div>;
}
