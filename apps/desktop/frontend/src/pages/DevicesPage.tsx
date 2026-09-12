import { useEffect, useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DeviceInfo, DeviceProfile, InvitationInfo, MembershipStatus } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { formatPairingCodeInput, shouldClearPairingCode } from '../connection';
import { deviceName } from '../presentation';

type Props = { devices: DeviceInfo[]; identityID?: string; membership?: MembershipStatus; name: string; run: CommandRunner; op: string; available: boolean };

export function DevicesPage({ devices, identityID, membership, name, run, op, available }: Props) {
  const [invite, setInvite] = useState<InvitationInfo>();
  const [code, setCode] = useState('');
  const [address, setAddress] = useState('');
  const peers = devices.filter(device => device.id !== identityID);
  return <div className="page-stack">
    <div className="page-intro"><div><p className="eyebrow">附近与已配对</p><h2>设备</h2><p>本地别名、固定顺序和接收权限分别设置。</p></div><button className="primary" disabled={!!op || membership?.state !== 'member'} onClick={() => void run('invite', async () => setInvite(await Backend.CreateInvitation()))}>生成配对码</button></div>
    {invite && <InvitationPanel key={invite.token} invite={invite} run={run} />}
    <div className="device-columns">
      <section className="surface device-directory"><div className="section-heading compact"><h2>我的设备与附近设备</h2><span className="task-count">{peers.length}</span></div>
        {!peers.length && <div className="empty-state small"><strong>还没有发现设备</strong><p>在另一台设备打开 LinkSend；跨网络连接可输入配对码。</p></div>}
        {peers.map(device => <DeviceRow key={device.id} device={device} run={run} op={op} available={available} />)}
      </section>
      <section className="surface pairing-form"><span className="section-kicker">连接另一台设备</span><h2>跨网络配对</h2><p className="muted">输入一次性配对码建立信任，接收时仍按本机策略确认。</p>
        <form onSubmit={event => {
          event.preventDefault();
          void run('join', async () => { await Backend.PairDevice(code.trim(), name || 'LinkSend desktop'); setCode(''); }, '配对成功', cause => { if (shouldClearPairingCode(cause)) setCode(''); });
        }}><label className="field-label">配对码<input className="pairing-input" value={code} onChange={event => setCode(formatPairingCodeInput(event.target.value))} placeholder="ABCD-EFGH" maxLength={43} autoComplete="off" spellCheck={false} /></label><button className="primary full" disabled={!!op || !code.trim() || !available}>完成配对</button></form>
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
    <div className="device-row"><span className="avatar" aria-hidden="true">{deviceName(device)[0]}</span><div><strong>{deviceName(device)}</strong>{device.profile.alias && <small>对方名称：{device.name || '未命名设备'}</small>}<small>{device.blocked ? '已屏蔽' : device.trusted ? '已信任' : '附近发现 · 尚未信任'} · {device.online ? '在线' : '离线'}</small><div className="device-tags">{device.profile.my_device && <span>我的设备</span>}{device.profile.pinned && <span>固定 · {device.profile.position}</span>}{device.always_accept && !device.blocked && <span>免确认接收</span>}</div><code>身份 {device.id.slice(0, 12)}</code>{device.profile.last_used_at && <small>最近交互 {new Date(device.profile.last_used_at).toLocaleString()}</small>}</div><button className="ghost" aria-expanded={editing} disabled={!available} onClick={() => setEditing(!editing)}>{editing ? '收起' : '设置'}</button></div>
    {editing && <DeviceEditor device={device} run={run} op={op} />}
  </article>;
}

function DeviceEditor({ device, run, op }: { device: DeviceInfo; run: CommandRunner; op: string }) {
  const [profile, setProfile] = useState<DeviceProfile>({ ...device.profile, peer_id: device.id });
  const update = (patch: Partial<DeviceProfile>) => setProfile(current => ({ ...current, ...patch }));
  return <div className="device-editor"><form onSubmit={event => {
    event.preventDefault();
    void run(`profile-${device.id}`, async () => setProfile(await Backend.SaveDeviceProfile(profile)), '设备偏好已保存');
  }}>
    <label className="field-label">本机别名<input value={profile.alias} maxLength={80} onChange={event => update({ alias: event.target.value })} placeholder={device.name || '未命名设备'} /></label>
    <div className="preference-checks"><label><input type="checkbox" checked={profile.my_device} disabled={!device.trusted || device.blocked} onChange={event => update({ my_device: event.target.checked })} />我的设备</label><label><input type="checkbox" checked={profile.pinned} onChange={event => update({ pinned: event.target.checked })} />固定在前面</label></div>
    {profile.pinned && <label className="field-label">固定顺序（较小的排在前面）<input type="number" min={0} max={9999} value={profile.position} onChange={event => update({ position: Math.max(0, Number(event.target.value)) })} /></label>}
    <label className="field-label">此设备的接收目录<input value={profile.receive_directory} readOnly placeholder="继承全局接收目录" /></label><div className="inline-actions"><button type="button" className="secondary" disabled={!!op} onClick={() => void run('pick-device-directory', async () => { const path = await Backend.PickDirectory(); if (path) update({ receive_directory: path }); })}>选择目录</button><button type="button" className="ghost" disabled={!profile.receive_directory} onClick={() => update({ receive_directory: '' })}>继承全局目录</button></div>
    <p className="field-help">保存后用于新接收任务；正在传输的文件保持原保存位置。</p>
    {device.profile.revision !== profile.revision && <p className="queue-notice">设备偏好已在后台更新。保存冲突时，请收起后重新打开设置。</p>}
    <button className="primary" disabled={!!op}>保存设备偏好</button>
  </form>
    <div className="device-permissions"><label className="remember-choice"><input type="checkbox" checked={device.always_accept && !device.blocked} disabled={!!op || !device.trusted || device.blocked} onChange={event => {
      const enabled = event.target.checked;
      void run(`consent-${device.id}`, () => Backend.SetAlwaysAccept(device.id, enabled), enabled ? '已开启此设备免确认接收' : '已恢复每次接收确认');
    }} /><span><strong>免确认接收</strong><small>与收藏和“我的设备”标签独立，仍验证设备身份与文件完整性。</small></span></label>
      <button className="ghost danger" disabled={!!op} onClick={() => void run(`block-${device.id}`, () => device.blocked ? Backend.UnblockPeer(device.id) : Backend.BlockPeer(device.id), device.blocked ? '已解除屏蔽；需重新配对或确认建立信任' : '已屏蔽设备并停止其未完成传输')}>{device.blocked ? '解除屏蔽' : '屏蔽并取消信任'}</button>
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
