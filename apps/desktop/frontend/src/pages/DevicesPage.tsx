import { useCallback, useEffect, useRef, useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { ClipboardGrant, DeviceInfo, DeviceProfile, InvitationInfo, MembershipStatus, InboxStatus } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { normalizePairingCode, shouldClearPairingCode } from '../connection';
import { deviceConnectionLabel, deviceName } from '../presentation';
import { MembershipRecoveryPanel } from './MembershipRecoveryPanel';

type Props = { devices: DeviceInfo[]; identityID?: string; membership?: MembershipStatus; inbox?: InboxStatus; name: string; run: CommandRunner; op: string; available: boolean };
type DirectoryKind = 'paired' | 'nearby' | 'blocked';
type CloseReason = 'close' | 'removed';
type Pages = Record<DirectoryKind, number>;

const pageSize = 10;
const emptyPages: Pages = { paired: 0, nearby: 0, blocked: 0 };

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

export function DevicesPage({ devices, identityID, membership, inbox, name, run, op, available }: Props) {
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
  const [discoveryError, setDiscoveryError] = useState('');

  useEffect(() => {
    if (!available) return;
    let current = true;
    void Backend.RefreshLANDiscovery().then(() => {
      if (current) setDiscoveryError('');
    }).catch(() => {
      if (current) setDiscoveryError('局域网发现暂未就绪，可刷新重试。');
    });
    return () => { current = false; };
  }, [available]);

  const paired = devices.filter(device => device.id !== identityID && device.trusted && !device.blocked);
  const nearby = devices.filter(device => device.id !== identityID && device.nearby && !device.trusted && !device.blocked);
  const blocked = devices.filter(device => device.id !== identityID && device.blocked && device.relationship === 'blocked');
  const directory = {
    paired: paired.filter(device => matchesDeviceSearch(device, query)),
    nearby: nearby.filter(device => matchesDeviceSearch(device, query)),
    blocked: blocked.filter(device => matchesDeviceSearch(device, query)),
  };
  const selected = detailDeviceID ? devices.find(device => device.id === detailDeviceID) : undefined;

  useEffect(() => {
    setPages(current => {
      const next: Pages = {
        paired: clampDevicePage(current.paired, directory.paired.length),
        nearby: clampDevicePage(current.nearby, directory.nearby.length),
        blocked: clampDevicePage(current.blocked, directory.blocked.length),
      };
      return next.paired === current.paired && next.nearby === current.nearby && next.blocked === current.blocked ? current : next;
    });
  }, [directory.paired.length, directory.nearby.length, directory.blocked.length]);

  useEffect(() => {
    if ((!selected || selected.blocked) && detailDeviceID) {
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
    <MembershipRecoveryPanel available={available} membershipState={membership?.state} name={name} run={run} op={op} />
    <LANDiscoveryNotice inbox={inbox} error={inbox?.lan_available ? '' : discoveryError} />
    <div className="device-columns">
      <section className="surface device-directory" aria-label="设备目录">
        <div className="section-heading compact"><div><h2>设备目录</h2><small className="field-help">搜索结果按类别分页显示。</small></div><span className="task-count">{paired.length + nearby.length + blocked.length} 台</span></div>
        <label className="device-search"><span>查找设备</span><input ref={searchInput} aria-label="按名称、别名或身份查找设备" value={query} onChange={event => changeQuery(event.target.value)} placeholder="名称、别名或完整身份" /></label>
        <DeviceDirectorySection kind="paired" title="已配对设备" total={paired.length} devices={directory.paired} page={pages.paired} query={query} available={available} op={op} run={run} onPage={changePage} onOpen={openDetail} />

        <DeviceDirectorySection kind="blocked" title="黑名单" total={blocked.length} devices={directory.blocked} page={pages.blocked} query={query} available={available} op={op} run={run} onPage={changePage} onOpen={openDetail} />
        {lanResult && <LANPairResultNotice name={lanResult.name} server={lanResult.server} onSwitch={() => { pairingCode.current?.focus(); pairingCode.current?.scrollIntoView({ block: 'center' }); }} />}
      </section>
      <section className="surface pairing-form"><span className="section-kicker">连接另一台设备</span><h2>跨网络配对</h2><p className="muted">输入一次性配对码建立信任，接收时仍按本机策略确认。</p>
        <form onSubmit={event => {
          event.preventDefault();
          const invitation = normalizePairingCode(code);
          void run('join', async () => { try { await Backend.PairDevice(invitation, name || 'LinkSend desktop'); setCode(''); } catch (cause) { if (String(cause).includes('PAIRING_IDENTITY_CONFLICT')) setSwitchCode(invitation); throw cause; } }, '配对成功', cause => { if (shouldClearPairingCode(cause)) setCode(''); });
        }}><label className="field-label">配对码<input ref={pairingCode} className="pairing-input" value={code} onChange={event => setCode(event.target.value)} placeholder="ABCD-EFGH" maxLength={43} autoComplete="off" spellCheck={false} /></label><button className="primary full" disabled={!!op || !code.trim() || !available}>完成配对</button></form>
        {switchCode && <div className="queue-notice"><strong>本机已属于另一设备组</strong><p>切换会离开当前设备组；历史记录和已接收文件保留。</p><button className="secondary" disabled={!!op} onClick={() => void run('switch-membership', async () => { await Backend.SwitchMembership(switchCode, name || 'LinkSend desktop'); setSwitchCode(''); setCode(''); }, '已切换并完成配对')}>确认切换设备组</button></div>}
        {membership?.message && <p className="field-help pairing-help">{membership.message}</p>}
        <div className="divider" /><DeviceDirectorySection kind="nearby" title="局域网发现的设备" total={nearby.length} devices={directory.nearby} page={pages.nearby} query={query} available={available} op={op} run={run} onPage={changePage} onOpen={openDetail} onLANResult={setLanResult} /><details className="manual-lan-discovery"><summary>附近没有设备出现，或没有想要的设备？</summary><p className="field-help">输入对方的局域网 IPv4 地址，只查找这一台设备。</p>
        <form onSubmit={event => { event.preventDefault(); void run('lan-probe', () => Backend.ProbeLANAddress(address.trim()), '已发送定向发现请求'); }}><label className="field-label">对方局域网地址<input value={address} onChange={event => setAddress(event.target.value)} placeholder="例如 192.168.1.20" inputMode="decimal" spellCheck={false} /></label><button className="secondary full" disabled={!!op || !address.trim() || !available}>查找设备</button></form></details>
      </section>
    </div>
    {selected && <DeviceDetailDrawer key={selected.id} device={selected} run={run} op={op} onClose={closeDetail} />}
  </div>;
}

export function LANDiscoveryNotice({ inbox, error }: { inbox?: Pick<InboxStatus, 'lan_last_error' | 'lan_provider_errors'>; error?: string }) {
  const labels: Record<string, string> = { listen: '固定发现端口不可用，正在使用临时端口并自动重试。', receive: '发现接收暂不可用，正在自动重建。', interfaces: '局域网网卡信息暂不可用。', multicast: '组播发现受限。', broadcast: '广播发现受限。', multicast_send: '组播探测发送失败。', broadcast_send: '广播探测发送失败。', announcement: '自动探测暂时无法发送。' };
  const providers = Object.keys(inbox?.lan_provider_errors ?? {}).sort();
  if (!error && !inbox?.lan_last_error && providers.length === 0) return null;
  return <aside className="queue-notice" role="status"><strong>局域网发现通道受限</strong>
    {(error || inbox?.lan_last_error) && <p>{error || '局域网发现暂未就绪。'}</p>}
    {providers.map(provider => <p key={provider}>{labels[provider] || '部分发现通道暂不可用。'}</p>)}
    <p>其他通道会继续尝试；也可在下方输入对方的局域网地址。发现设备后仍需明确配对。</p>
  </aside>;
}

function DeviceDirectorySection({ kind, title, total, devices, page, query, available, op, run, onPage, onOpen, onLANResult }: { kind: DirectoryKind; title: string; total: number; devices: DeviceInfo[]; page: number; query: string; available: boolean; op: string; run: CommandRunner; onPage: (kind: DirectoryKind, page: number) => void; onOpen: (id: string, trigger: HTMLButtonElement) => void; onLANResult?: (result: { name: string; server: string }) => void }) {
  const pageCount = Math.max(1, Math.ceil(devices.length / pageSize));
  const activePage = clampDevicePage(page, devices.length);
  const visible = devices.slice(activePage * pageSize, activePage * pageSize + pageSize);
  const hasNoDevices = total === 0;
  const hasNoMatches = !hasNoDevices && visible.length === 0;
  return <section className={'device-directory-section ' + kind} aria-labelledby={'device-section-' + kind}>
    <div className="section-heading compact"><h3 id={'device-section-' + kind}>{title}</h3><span className="task-count">{total} 台{query && ' · 匹配 ' + devices.length + ' 台'}</span></div>
    {hasNoDevices && <div className="empty-state small"><strong>{kind === 'paired' ? '还没有已配对设备' : kind === 'nearby' ? '没有附近设备' : '黑名单为空'}</strong><p>{kind === 'paired' ? '可输入配对码，或从附近设备发起添加。' : kind === 'nearby' ? '在另一台设备打开 LinkSend 后会出现在这里。' : '加入黑名单的设备会被拒绝连接，可在此移除。'}</p></div>}
    {hasNoMatches && <p className="field-help device-search-empty">没有匹配的设备。</p>}
    {visible.map(device => <DeviceListRow key={device.id} kind={kind} device={device} available={available} op={op} run={run} onOpen={onOpen} onLANResult={onLANResult} />)}
    {!hasNoDevices && <div className="device-pagination" aria-label={title + '分页'}><button className="ghost" aria-label={title + '上一页'} disabled={activePage === 0} onClick={() => onPage(kind, activePage - 1)}>上一页</button><span>第 {activePage + 1} / {pageCount} 页</span><button className="ghost" aria-label={title + '下一页'} disabled={activePage + 1 >= pageCount} onClick={() => onPage(kind, activePage + 1)}>下一页</button></div>}
  </section>;
}

function DeviceListRow({ kind, device, available, op, run, onOpen, onLANResult }: { kind: DirectoryKind; device: DeviceInfo; available: boolean; op: string; run: CommandRunner; onOpen: (id: string, trigger: HTMLButtonElement) => void; onLANResult?: (result: { name: string; server: string }) => void }) {
  const label = deviceName(device);
  const identity = device.id.slice(0, 12);
  const actionsDisabled = !!op || !available;
  const relationship = kind === 'blocked' ? '已加入黑名单' : device.blocked ? '已屏蔽' : device.trusted ? '已配对' : '尚未配对';
  return <article className={'device-entry ' + (device.blocked ? 'device-blocked' : '')} data-device-id={device.id}>
    <div className="device-row"><span className="avatar" aria-hidden="true">{label[0] || '?'}</span><div><strong>{label}</strong>{device.profile.alias && <small>对方名称：{device.name || '未命名设备'}</small>}<small>{relationship} · {deviceConnectionLabel(device)}</small><div className="device-tags">{device.profile.my_device && <span>我的设备</span>}{device.profile.pinned && <span>固定 · {device.profile.position}</span>}{device.always_accept && !device.blocked && <span>免确认接收</span>}</div><code>身份 {identity}</code></div>
      {kind === 'paired' && <button className="ghost" aria-label={'打开 ' + label + ' 的设备详情，身份 ' + identity} disabled={!available} onClick={event => onOpen(device.id, event.currentTarget)}>详情</button>}
      {kind === 'nearby' && <button className="secondary" aria-label={'请求添加 ' + label + '，身份 ' + identity} disabled={!!op} onClick={() => void run('lan-add-' + device.id, async () => { const result = await Backend.RequestLANPair(device.id); onLANResult?.({ name: device.name || '附近设备', server: result.server_state }); })}>请求添加</button>}
      {kind === 'blocked' && <button className="secondary" aria-label={'移出黑名单 ' + label + '，身份 ' + identity} disabled={actionsDisabled} onClick={() => void run('unblock-' + device.id, () => Backend.UnblockDevice(device.id), '已移出黑名单；请重新发起配对')}>移出黑名单</button>}
    </div>

  </article>;
}

function DeviceDetailDrawer({ device, run, op, onClose }: { device: DeviceInfo; run: CommandRunner; op: string; onClose: (reason: CloseReason) => void }) {
  const closeButton = useRef<HTMLButtonElement>(null);
  const drawer = useRef<HTMLElement>(null);
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
        <div className="device-detail-actions"><button type="button" className="primary" disabled={!!op || profileSaving || !submitProfile} onClick={() => submitProfile?.()}>保存设备偏好</button><button className="ghost" disabled={!!op || profileSaving} onClick={() => void run('block-' + device.id, () => Backend.BlockPeer(device.id), '已加入黑名单，将拒绝此设备连接').then(ok => { if (ok) onClose('removed'); })}>加入黑名单</button><button className="ghost danger" aria-label={'删除 ' + label + '，身份 ' + identity} disabled={!!op || profileSaving} onClick={() => void run('remove-' + device.id, () => Backend.RemoveDevice(device.id), '已删除设备，可重新发起配对').then(ok => { if (ok) onClose('removed'); })}>删除设备</button></div>
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
