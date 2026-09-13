import { useEffect, useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { BackgroundStatus, DesktopPreferences, EffectiveConfig, NetworkInterfaceInfo, PreferencesStatus } from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/models';
import type { Diagnostics, InboxStatus } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { BackgroundSettings } from '../components/BackgroundSettings';
import { ContentSettings } from '../components/ContentSettings';

type Category = 'general' | 'receive' | 'system' | 'network' | 'storage' | 'diagnostics';
type Props = { preferences: DesktopPreferences; effective?: EffectiveConfig; preferencesStatus?: PreferencesStatus; interfaces: NetworkInterfaceInfo[]; diagnostics?: Diagnostics; inbox?: InboxStatus; background?: BackgroundStatus; run: CommandRunner; controlRun: CommandRunner; op: string; available: boolean; sendToSupported?: boolean };
const categories: { id: Category; label: string; hint: string }[] = [
  { id: 'general', label: '通用', hint: '名称与外观' }, { id: 'receive', label: '文件接收', hint: '目录与同名策略' },
  { id: 'system', label: '系统集成', hint: '后台、通知与分享' }, { id: 'network', label: '网络与发现', hint: '服务与接口' },
  { id: 'storage', label: '存储与记录', hint: '旧内容与清理' }, { id: 'diagnostics', label: '诊断与关于', hint: '连接与版本' },
];
const splitNames = (value: string) => value.split(',').map(item => item.trim()).filter(Boolean);

export function SettingsPage({ preferences, effective, preferencesStatus, interfaces, diagnostics, inbox, background, run, controlRun, op, available, sendToSupported }: Props) {
  const [category, setCategory] = useState<Category>('general');
  const [prefs, setPrefs] = useState(preferences);
  useEffect(() => setPrefs(preferences), [preferences]);
  const update = (patch: Partial<DesktopPreferences>) => setPrefs(current => ({ ...current, ...patch }));
  const save = (section: 'general' | 'receive' | 'network', key: string, message: string, next = prefs) => void run(key, async () => {
    setPrefs(await Backend.SavePreferencesSection(section, next.revision, next));
  }, message);
  return <div className="settings-shell">
    <aside className="settings-nav" aria-label="设置分类">{categories.map(item => <button key={item.id} className={category === item.id ? 'active' : ''} onClick={() => setCategory(item.id)}><strong>{item.label}</strong><small>{item.hint}</small></button>)}</aside>
    <div className="settings-panel">
      {preferencesStatus?.state && !['valid', 'missing'].includes(preferencesStatus.state) && <div className="banner error-banner" role="alert">{preferencesStatus.message || '偏好文件异常，原文件已保留。'}</div>}
      {category === 'general' && <section className="surface settings-page"><span className="section-kicker">通用</span><h2>本机显示</h2><label className="field-label">本机名称<input value={prefs.device_name} onChange={event => update({ device_name: event.target.value })} /></label><label className="field-label">外观<select value="system" disabled><option>跟随系统</option></select></label><label className="field-label">语言<select value="zh-CN" disabled><option>简体中文</option></select></label><button className="primary" disabled={!!op || !available} onClick={() => save('general', 'preferences-general', '通用设置已保存')}>保存通用设置</button></section>}
      {category === 'receive' && <section className="surface settings-page"><span className="section-kicker">文件接收</span><h2>目录与冲突</h2><label className="field-label">默认接收目录<input value={prefs.receive_directory} readOnly placeholder="请选择目录" /></label><button className="secondary" disabled={!!op || !available} onClick={() => void run('settings-directory', async () => { const path = await Backend.PickDirectory(); if (path) { const next = { ...prefs, receive_directory: path }; setPrefs(await Backend.SavePreferencesSection('receive', prefs.revision, next)); } }, '默认接收目录已保存')}>选择目录</button><label className="field-label">遇到同名文件<select value="keep_both" disabled><option>保留两份，自动生成新名称</option></select></label><p className="field-help">默认保留两份；单台设备的目录覆盖在设备详情中设置。</p></section>}
      {category === 'system' && <div className="settings-stack"><BackgroundSettings status={background} run={controlRun} op={op} available={available} /><section className="surface settings-page"><span className="section-kicker">系统分享</span><h2>文件管理器入口</h2><p className="field-help">Windows“发送到”和 macOS Finder 服务作为兼容入口；正式系统分享能力以准确安装包为准。</p>{sendToSupported && <div className="inline-actions"><button className="secondary" disabled={!!op} onClick={() => void run('install-sendto', () => Backend.ConfigureSendTo(true), '已安装发送到入口')}>安装入口</button><button className="ghost" disabled={!!op} onClick={() => void run('uninstall-sendto', () => Backend.ConfigureSendTo(false), '已移除发送到入口')}>移除入口</button></div>}</section></div>}
      {category === 'network' && <section className="surface settings-page"><span className="section-kicker">网络与发现</span><h2>连接设置</h2><label className="field-label">信令服务地址<input value={prefs.server_url} onChange={event => update({ server_url: event.target.value })} /></label><details><summary>高级接口设置</summary><label className="field-label">本机绑定地址<input value={prefs.bind_address} onChange={event => update({ bind_address: event.target.value })} placeholder="留空自动选择" /></label><label className="field-label">接口优先级<input value={(prefs.interface_priority ?? []).join(', ')} onChange={event => update({ interface_priority: splitNames(event.target.value) })} /></label><label className="field-label">排除接口<input value={(prefs.excluded_interfaces ?? []).join(', ')} onChange={event => update({ excluded_interfaces: splitNames(event.target.value) })} /></label><label className="field-label">STUN 地址<input value={(prefs.stun_urls ?? []).join(', ')} onChange={event => update({ stun_urls: splitNames(event.target.value) })} /></label></details><div className="interface-list">{interfaces.filter(item => !item.is_loopback).map(item => <div className="interface-row" key={item.name}><span>{item.name}</span><small>{(item.addresses ?? []).join(' / ')}</small></div>)}</div>{effective && <p className="field-help">当前绑定：{effective.bind_address || '自动'} · 保存后用于新连接</p>}<button className="primary" disabled={!!op || !available} onClick={() => save('network', 'preferences-network', '网络设置已保存；健康传输继续运行')}>保存网络设置</button></section>}
      {category === 'storage' && <section className="surface settings-page"><span className="section-kicker">存储与记录</span><h2>旧版内容与应用暂存</h2><p className="field-help">旧版文字、链接和图片入口已移除。已有草稿、队列和历史不会自动续发，可在记录与清理区处理。</p><ContentSettings run={controlRun} op={op} available={available} /></section>}
      {category === 'diagnostics' && <section className="surface settings-page"><span className="section-kicker">诊断与关于</span><h2>LinkSend 0.5.0</h2><dl className="settings-diagnostics"><div><dt>后台接收</dt><dd>{inbox?.listening ? '正在等待请求' : '尚未就绪'}</dd></div><div><dt>附近发现</dt><dd>{inbox?.lan_available ? '可用' : '不可用'}</dd></div><div><dt>信令健康</dt><dd>{diagnostics?.server_health || '未检查'}</dd></div><div><dt>任务恢复</dt><dd>{diagnostics?.restart_recovery_supported ? '可用' : '不可用'}</dd></div><div><dt>中继</dt><dd>未实现</dd></div></dl></section>}
    </div>
  </div>;
}
