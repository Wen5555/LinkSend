import { useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DesktopPreferences, EffectiveConfig, NetworkInterfaceInfo, PreferencesStatus } from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/models';
import type { Diagnostics, InboxStatus } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';

type Props = { preferences: DesktopPreferences; effective?: EffectiveConfig; preferencesStatus?: PreferencesStatus; interfaces: NetworkInterfaceInfo[]; diagnostics?: Diagnostics; inbox?: InboxStatus; run: CommandRunner; op: string; available: boolean };
const splitNames = (value: string) => value.split(',').map(item => item.trim()).filter(Boolean);

export function SettingsPage({ preferences, effective, preferencesStatus, interfaces, diagnostics, inbox, run, op, available }: Props) {
  const [prefs, setPrefs] = useState(preferences);
  const update = (patch: Partial<DesktopPreferences>) => setPrefs(current => ({ ...current, ...patch }));
  return <form className="page-stack" onSubmit={event => { event.preventDefault(); void run('preferences', () => Backend.SavePreferences(prefs), '设置已保存'); }}>
    <div className="page-intro"><div><p className="eyebrow">本机偏好</p><h2>设置与诊断</h2><p>通常无需修改网络选项；接收目录保存后用于新的接收任务。</p></div><button className="primary" disabled={!!op || !available}>保存设置</button></div>
    {preferencesStatus?.state && !['valid', 'missing'].includes(preferencesStatus.state) && <div className="banner error-banner" role="alert">{preferencesStatus.message || '偏好文件异常，原文件已保留。请核对设置后保存。'}</div>}
    <div className="settings-grid"><section className="surface"><span className="section-kicker">连接</span><h2>服务与网卡</h2>
      <label className="field-label">信令服务地址<input value={prefs.server_url} onChange={event => update({ server_url: event.target.value })} placeholder="https://linksend.oooai.de" /></label>
      <label className="field-label">本机绑定地址<input value={prefs.bind_address} onChange={event => update({ bind_address: event.target.value })} placeholder="留空自动选择；或填 192.168.1.20:0" /></label>
      <p className="field-help">默认使用系统可用接口；仅在需要时指定物理网卡。</p>
      <label className="field-label">接口优先级<input value={(prefs.interface_priority ?? []).join(', ')} onChange={event => update({ interface_priority: splitNames(event.target.value) })} placeholder="接口名，逗号分隔" /></label>
      <label className="field-label">排除接口<input value={(prefs.excluded_interfaces ?? []).join(', ')} onChange={event => update({ excluded_interfaces: splitNames(event.target.value) })} /></label>
      {effective && <p className="field-help">当前服务：{effective.server_url}；绑定：{effective.bind_address || '自动'}{effective.needs_restart ? ' · 部分设置需重启生效' : ''}</p>}
      <div className="interface-list">{interfaces.filter(item => !item.is_loopback).map(item => <div className="interface-row" key={item.name}><span>{item.name}</span><small>{(item.addresses ?? []).join(' / ')}</small></div>)}</div>
      <label className="field-label">STUN 地址<input value={(prefs.stun_urls ?? []).join(', ')} onChange={event => update({ stun_urls: splitNames(event.target.value) })} placeholder="stun:stun.oooai.de:3478" /></label>
    </section><section className="surface"><span className="section-kicker">接收</span><h2>设备与文件</h2>
      <label className="field-label">本机名称<input value={prefs.device_name} onChange={event => update({ device_name: event.target.value })} /></label>
      <label className="field-label">默认接收目录<input value={prefs.receive_directory} readOnly placeholder="请选择目录" /></label><button type="button" className="secondary full" disabled={!!op || !available} onClick={() => void run('settings-directory', async () => { const path = await Backend.PickDirectory(); if (path) update({ receive_directory: path }); })}>选择目录</button>
      <div className="diagnostic-block"><span className="section-kicker">只读诊断</span><dl><div><dt>后台接收</dt><dd>{inbox?.listening ? '正在等待请求' : inbox?.signaling_connected ? '信令在线' : '尚未就绪'}</dd></div><div><dt>接收连接次数</dt><dd>{inbox?.connection_count ?? 0}</dd></div><div><dt>信令健康</dt><dd>{diagnostics?.server_health || '未检查'}</dd></div><div><dt>中继</dt><dd>尚未实现</dd></div><div><dt>任务历史</dt><dd>{diagnostics?.history_persisted ? '已持久化' : '不可用'}</dd></div><div><dt>重启恢复</dt><dd>{diagnostics?.restart_recovery_supported ? '可用，需确认继续' : '不可用'}</dd></div><div><dt>字节级续传</dt><dd>{diagnostics?.byte_resume_supported ? '可用' : '不可用'}</dd></div></dl></div>
    </section></div>
  </form>;
}
