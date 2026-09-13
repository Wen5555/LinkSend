import { useEffect, useRef, useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DeviceInfo, DeviceProfile, InvitationInfo, MembershipStatus } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { formatPairingCodeInput, shouldClearPairingCode } from '../connection';
import { deviceName } from '../presentation';

type Props = { devices: DeviceInfo[]; identityID?: string; membership?: MembershipStatus; name: string; run: CommandRunner; op: string; available: boolean };
export const lanPairServerMessage = (state: string) => state === 'joined' ? '跨网络设备组也已同步完成。' : state === 'switch_required' ? '局域网可立即使用；跨网络使用前需输入对方配对码并确认切换设备组。' : state === 'pending' ? '局域网可立即使用；跨网络使用请在服务恢复后输入对方配对码继续。' : '当前仅建立局域网信任。';
export function LANPairResultNotice({ name, server, onSwitch }: { name: string; server: string; onSwitch: () => void }) {
  return <div className="queue-notice" role="status"><strong>{name} 已完成局域网添加</strong><p>{lanPairServerMessage(server)}</p>{server === 'switch_required' && <button className="secondary" onClick={onSwitch}>输入配对码并切换</button>}</div>;
}

export function DevicesPage({ devices, identityID, membership, name, run, op, available }: Props) {
  const [invite, setInvite] = useState<InvitationInfo>();
  const [code, setCode] = useState('');
  const [address, setAddress] = useState('');
  const [switchCode, setSwitchCode] = useState('');
  const pairingCode = useRef<HTMLInputElement>(null);
  const [lanResult, setLANResult] = useState<{ name: string; server: string }>();
  const peers = devices.filter(device => device.id !== identityID && device.trusted && !device.blocked);
  const nearby = devices.filter(device => device.id !== identityID && device.nearby && !device.trusted && !device.blocked);
  const removed = devices.filter(device => device.id !== identityID && device.blocked && device.relationship === 'removed');
  return <div className="page-stack">
    <div className="page-intro"><div><p className="eyebrow">可信设备</p><h2>设备</h2><p>配对关系与当前连接状态分开显示。</p></div>{membership?.state === 'not_member' ? <button className="primary" disabled={!!op} onClick={() => void run('initialize', () => Backend.InitializeMembership(name || 'LinkSend desktop'), '本机设备组已初始化')}>初始化本机</button> : <button className="primary" disabled={!!op || membership?.state !== 'member'} onClick={() => void run('invite', async () => setInvite(await Backend.CreateInvitation()))}>生成配对码</button>}</div>
    {invite && <InvitationPanel key={invite.token} invite={invite} run={run} />}
    <div className="device-columns">
      <section className="surface device-directory"><div className="section-heading compact"><h2>已配对设备</h2><span className="task-count">{peers.length}</span></div>
        {!peers.length && <div className="empty-state small"><strong>还没有已配对设备</strong><p>可输入配对码，或从附近设备发起添加。</p></div>}
        {peers.map(device => <DeviceRow key={device.id} device={device} run={run} op={op} available={available} />)}
        <div className="nearby-section"><div className="section-heading compact"><h3>添加附近设备</h3><span className="task-count">{nearby.length}</span></div>{!nearby.length ? <p className="field-help">在另一台设备打开 LinkSend 后会出现在这里。</p> : nearby.map(device => <div className="nearby-row" key={device.id}><div><strong>{device.name || '附近设备'}</strong><small>局域网可达 · 尚未建立信任</small></div><button className="secondary" disabled={!!op} onClick={() => void run(`lan-add-${device.id}`, async () => { const result = await Backend.RequestLANPair(device.id); setLANResult({ name: device.name || '附近设备', server: result.server_state }); })}>请求添加</button></div>)}</div>
        {lanResult && <LANPairResultNotice name={lanResult.name} server={lanResult.server} onSwitch={() => { pairingCode.current?.focus(); pairingCode.current?.scrollIntoView({ block: 'center' }); }} />}
        {!!removed.length && <div className="nearby-section"><div className="section-heading compact"><h3>已删除设备</h3><span className="task-count">{removed.length}</span></div>{removed.map(device => <div className="nearby-row" key={device.id}><div><strong>{device.name || '已删除设备'}</strong><small>本机授权已撤销，不会自动重新配对</small></div><button className="secondary" disabled={!!op} onClick={() => void run(`allow-readd-${device.id}`, () => Backend.UnblockDevice(device.id), '已允许重新添加；请重新发起配对')}>允许重新添加</button></div>)}</div>}
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
  </div>;
}

function DeviceRow({ device, run, op, available }: { device: DeviceInfo; run: CommandRunner; op: string; available: boolean }) {
  const [editing, setEditing] = useState(false);
  return <article className={`device-entry ${device.blocked ? 'device-blocked' : ''}`}>
    <div className="device-row"><span className="avatar" aria-hidden="true">{deviceName(device)[0]}</span><div><strong>{deviceName(device)}</strong>{device.profile.alias && <small>对方名称：{device.name || '未命名设备'}</small>}<small>{device.blocked ? '已屏蔽' : device.trusted ? '已信任' : '附近发现 · 尚未信任'} · {device.nearby ? '局域网可达' : device.online ? '在线' : '离线'}</small><div className="device-tags">{device.profile.my_device && <span>我的设备</span>}{device.profile.pinned && <span>固定 · {device.profile.position}</span>}{device.always_accept && !device.blocked && <span>免确认接收</span>}</div><code>身份 {device.id.slice(0, 12)}</code>{device.profile.last_used_at && <small>最近交互 {new Date(device.profile.last_used_at).toLocaleString()}</small>}</div><button className="ghost" aria-expanded={editing} disabled={!available} onClick={() => setEditing(!editing)}>{editing ? '收起' : '设置'}</button></div>
    {editing && <DeviceEditor device={device} run={run} op={op} />}
  </article>;
}

function DeviceEditor({ device, run, op }: { device: DeviceInfo; run: CommandRunner; op: string }) {
  const [profile, setProfile] = useState<DeviceProfile>({ ...device.profile, peer_id: device.id });
  const [confirmRemove, setConfirmRemove] = useState(false);
  const update = (patch: Partial<DeviceProfile>) => setProfile(current => ({ ...current, ...patch }));
  return <div className="device-editor"><form onSubmit={event => {
    event.preventDefault();
    void run(`profile-${device.id}`, async () => setProfile(await Backend.SaveDeviceProfile(profile)), '设备偏好已保存');
  }}>
    <label className="field-label">本机别名<input value={profile.alias} maxLength={80} onChange={event => update({ alias: event.target.value })} placeholder={device.name || '未命名设备'} /></label>
    <div className="preference-checks"><label><input type="checkbox" checked={profile.my_device} disabled={!device.trusted || device.blocked} onChange={event => update({ my_device: event.target.checked })} />我的设备</label><label><input type="checkbox" checked={profile.pinned} onChange={event => update({ pinned: event.target.checked })} />固定在前面</label></div>
    {profile.pinned && <label className="field-label">固定顺序（较小的排在前面）<input type="number" min={0} max={9999} value={profile.position} onChange={event => update({ position: Math.max(0, Number(event.target.value)) })} /></label>}
    <label className="field-label">此设备的接收目录<input value={profile.receive_directory} readOnly placeholder="继承全局接收目录" /></label><div className="inline-actions"><button type="button" className="secondary" disabled={!!op} onClick={() => void run('pick-device-directory', async () => { const path = await Backend.PickDirectory(); if (path) update({ receive_directory: path }); })}>选择目录</button><button type="button" className="ghost" disabled={!profile.receive_directory} onClick={() => update({ receive_directory: '' })}>继承全局目录</button></div>
    <label className="field-label">同名文件策略<select value={profile.conflict_policy} onChange={event => update({ conflict_policy: event.target.value })}><option value="">继承全局设置</option><option value="keep_both">保留两份</option><option value="skip">跳过同名文件</option><option value="error">遇到冲突时停止</option></select></label>
    <p className="field-help">保存后用于新接收任务；正在传输的文件保持原保存位置。</p>
    {device.profile.revision !== profile.revision && <p className="queue-notice">设备偏好已在后台更新。保存冲突时，请收起后重新打开设置。</p>}
    <button className="primary" disabled={!!op}>保存设备偏好</button>
  </form>
    <div className="device-permissions"><label className="remember-choice"><input type="checkbox" checked={device.always_accept && !device.blocked} disabled={!!op || !device.trusted || device.blocked} onChange={event => {
      const enabled = event.target.checked;
      void run(`consent-${device.id}`, () => Backend.SetAlwaysAccept(device.id, enabled), enabled ? '已开启此设备免确认接收' : '已恢复每次接收确认');
    }} /><span><strong>免确认接收</strong><small>与收藏和“我的设备”标签独立，仍验证设备身份与文件完整性。</small></span></label>
      {confirmRemove ? <div className="queue-notice" role="alert"><strong>删除此设备？</strong><p>将撤销本机授权并停止未完成传输；历史记录和已接收文件会保留。</p><div className="inline-actions"><button className="ghost danger" disabled={!!op} onClick={() => void run(`remove-${device.id}`, () => Backend.RemoveDevice(device.id), '设备已删除；服务不可用时会在恢复后继续同步')}>确认删除</button><button className="secondary" disabled={!!op} onClick={() => setConfirmRemove(false)}>取消</button></div></div> : <button className="ghost danger" disabled={!!op} onClick={() => setConfirmRemove(true)}>删除设备</button>}
    </div>
  </div>;
}

function InvitationPanel({ invite, run }: { invite: InvitationInfo; run: CommandRunner }) {
  const [remaining, setRemaining] = useState(Math.max(0, invite.expires_in_seconds));
  useEffect(() => {
    const started = Date.now(), initial = Math.max(0, invite.expires_in_seconds);
    const timer = window.setInterval(() => setRemaining(Math.max(0, initial - Math.floor((Date.now() - started) / 1000))), 1000);
    return () => window.clearInterval(timer);
  }, [invite.expires_in_seconds]);
  return <div className={`invite-panel pairing-code-panel ${remaining ? '' : 'expired'}`}><div><span className="section-kicker">{remaining ? `${Math.floor(remaining / 60)}:${String(remaining % 60).padStart(2, '0')} 后过期 · 单次使用` : '配对码已过期，请重新生成'}</span><p className="pairing-code">{invite.token}</p><small>在另一台设备输入配对码</small></div><button className="secondary" disabled={!remaining} onClick={() => void run('copy-invite', async () => { if (!navigator.clipboard) throw new Error('当前窗口无法访问剪贴板'); await navigator.clipboard.writeText(invite.token); }, '配对码已复制')}>复制配对码</button></div>;
}
