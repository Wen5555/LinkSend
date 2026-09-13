import { useEffect, useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { BackgroundStatus, DesktopPreferences, EffectiveConfig, NetworkInterfaceInfo, PreferencesStatus } from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/models';
import type { Diagnostics, InboxStatus } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { BackgroundSettings } from '../components/BackgroundSettings';
import { ContentSettings } from '../components/ContentSettings';

type Category = 'general' | 'receive' | 'system' | 'network' | 'storage' | 'diagnostics';
type EditableSection = 'general' | 'receive' | 'network';
type DirtyField = 'device_name' | 'receive_directory' | 'conflict_policy' | 'server_url' | 'bind_address' | 'interface_priority' | 'excluded_interfaces' | 'stun_urls';
type Props = { preferences: DesktopPreferences; effective?: EffectiveConfig; preferencesStatus?: PreferencesStatus; interfaces: NetworkInterfaceInfo[]; diagnostics?: Diagnostics; inbox?: InboxStatus; background?: BackgroundStatus; run: CommandRunner; controlRun: CommandRunner; op: string; available: boolean; sendToSupported?: boolean; onOpenLegacyQueue?: () => void };
const categories: { id: Category; label: string; hint: string }[] = [
  { id: 'general', label: '通用', hint: '名称与外观' }, { id: 'receive', label: '文件接收', hint: '目录与同名策略' },
  { id: 'system', label: '系统集成', hint: '后台、通知与分享' }, { id: 'network', label: '网络与发现', hint: '服务与接口' },
  { id: 'storage', label: '存储与记录', hint: '旧内容与清理' }, { id: 'diagnostics', label: '诊断与关于', hint: '连接与版本' },
];
const splitNames = (value: string) => value.split(',').map(item => item.trim()).filter(Boolean);
const sectionFields: Record<EditableSection, DirtyField[]> = {
  general: ['device_name'],
  receive: ['receive_directory', 'conflict_policy'],
  network: ['server_url', 'bind_address', 'interface_priority', 'excluded_interfaces', 'stun_urls'],
};
export function mergePreferences(current: DesktopPreferences, latest: DesktopPreferences, dirty: Set<DirtyField>): DesktopPreferences {
  const merged = { ...latest };
  for (const field of dirty) Object.assign(merged, { [field]: current[field] });
  return merged;
}
export function mergeSavedPreferences(current: DesktopPreferences, saved: DesktopPreferences, dirty: Set<DirtyField>, section: EditableSection) {
  const committed = new Set(sectionFields[section]);
  const remaining = new Set([...dirty].filter(field => !committed.has(field)));
  return { preferences: mergePreferences(current, saved, remaining), dirty: remaining };
}

export function SettingsPage({ preferences, effective, preferencesStatus, interfaces, diagnostics, inbox, background, run, controlRun, op, available, sendToSupported, onOpenLegacyQueue }: Props) {
  const [category, setCategory] = useState<Category>('general');
  const [prefs, setPrefs] = useState(preferences);
  const [baselineRevision, setBaselineRevision] = useState(preferences.revision);
  const [latestSeenRevision, setLatestSeenRevision] = useState(preferences.revision);
  const [dirty, setDirty] = useState<Set<DirtyField>>(new Set());
  const [mergeRequired, setMergeRequired] = useState(false);
  useEffect(() => {
    if (preferences.revision <= latestSeenRevision) return;
    setLatestSeenRevision(preferences.revision);
    if (dirty.size === 0) { setPrefs(preferences); setBaselineRevision(preferences.revision); return; }
    setPrefs(current => mergePreferences(current, preferences, dirty));
    setMergeRequired(true);
  }, [preferences, latestSeenRevision, dirty]);
  const update = (_section: EditableSection, patch: Partial<DesktopPreferences>) => {
    setDirty(current => {
      const next = new Set(current);
      for (const field of Object.keys(patch) as DirtyField[]) next.add(field);
      return next;
    });
    setPrefs(current => ({ ...current, ...patch }));
  };
  const save = (section: EditableSection, key: string, message: string) => void run(key, async () => {
    const saved = await Backend.SavePreferencesSection(section, baselineRevision, prefs);
    setDirty(current => {
      const merged = mergeSavedPreferences(prefs, saved, current, section);
      setPrefs(draft => mergePreferences(draft, saved, merged.dirty));
      return merged.dirty;
    });
    setBaselineRevision(saved.revision); setLatestSeenRevision(saved.revision); setMergeRequired(false);
  }, message, () => setMergeRequired(true));
  return <div className="settings-shell">
    <aside className="settings-nav" aria-label="设置分类">{categories.map(item => <button key={item.id} className={category === item.id ? 'active' : ''} onClick={() => setCategory(item.id)}><strong>{item.label}</strong><small>{item.hint}</small></button>)}</aside>
    <div className="settings-panel">
      {preferencesStatus?.state && !['valid', 'missing'].includes(preferencesStatus.state) && <div className="banner error-banner" role="alert">{preferencesStatus.message || '偏好文件异常，原文件已保留。'}</div>}
      {mergeRequired && <div className="queue-notice" role="alert"><strong>设置已在其他位置更新</strong><p>未编辑字段已采用新值，你正在输入的字段仍保留。确认后可重试当前分类。</p><button className="secondary" onClick={() => { setBaselineRevision(latestSeenRevision); setMergeRequired(false); }}>保留输入并合并最新设置</button></div>}
      {category === 'general' && <section className="surface settings-page"><span className="section-kicker">通用</span><h2>本机显示</h2><label className="field-label">本机名称<input value={prefs.device_name} onChange={event => update('general', { device_name: event.target.value })} /></label><label className="field-label">外观<select value="system" disabled><option>跟随系统</option></select></label><label className="field-label">语言<select value="zh-CN" disabled><option>简体中文</option></select></label><button className="primary" disabled={!!op || !available || mergeRequired} onClick={() => save('general', 'preferences-general', '通用设置已保存')}>保存通用设置</button></section>}
      {category === 'receive' && <section className="surface settings-page"><span className="section-kicker">文件接收</span><h2>目录与冲突</h2><label className="field-label">默认接收目录<input value={prefs.receive_directory} readOnly placeholder="请选择目录" /></label><button className="secondary" disabled={!!op || !available} onClick={() => void run('settings-directory', async () => { const path = await Backend.PickDirectory(); if (path) update('receive', { receive_directory: path }); })}>选择目录</button><label className="field-label">遇到同名文件<select value={prefs.conflict_policy || 'keep_both'} onChange={event => update('receive', { conflict_policy: event.target.value })}><option value="keep_both">保留两份，自动生成新名称</option><option value="skip">跳过同名文件</option><option value="error">遇到同名文件时停止</option></select></label><p className="field-help">默认保留两份；单台设备可在设备详情中覆盖或继承此策略。</p><button className="primary" disabled={!!op || !available || mergeRequired} onClick={() => save('receive', 'preferences-receive', '文件接收设置已保存')}>保存文件接收设置</button></section>}
      {category === 'system' && <div className="settings-stack"><BackgroundSettings status={background} run={controlRun} op={op} available={available} /><section className="surface settings-page"><span className="section-kicker">系统分享</span><h2>文件管理器入口</h2><p className="field-help">Windows“发送到”和 macOS Finder 服务作为兼容入口；正式系统分享能力以准确安装包为准。</p>{sendToSupported && <div className="inline-actions"><button className="secondary" disabled={!!op} onClick={() => void run('install-sendto', () => Backend.ConfigureSendTo(true), '已安装发送到入口')}>安装入口</button><button className="ghost" disabled={!!op} onClick={() => void run('uninstall-sendto', () => Backend.ConfigureSendTo(false), '已移除发送到入口')}>移除入口</button></div>}</section></div>}
      {category === 'network' && <section className="surface settings-page"><span className="section-kicker">网络与发现</span><h2>连接设置</h2><label className="field-label">信令服务地址<input value={prefs.server_url} onChange={event => update('network', { server_url: event.target.value })} /></label><details><summary>高级接口设置</summary><label className="field-label">本机绑定地址<input value={prefs.bind_address} onChange={event => update('network', { bind_address: event.target.value })} placeholder="留空自动选择" /></label><label className="field-label">接口优先级<input value={(prefs.interface_priority ?? []).join(', ')} onChange={event => update('network', { interface_priority: splitNames(event.target.value) })} /></label><label className="field-label">排除接口<input value={(prefs.excluded_interfaces ?? []).join(', ')} onChange={event => update('network', { excluded_interfaces: splitNames(event.target.value) })} /></label><label className="field-label">STUN 地址<input value={(prefs.stun_urls ?? []).join(', ')} onChange={event => update('network', { stun_urls: splitNames(event.target.value) })} /></label></details><div className="interface-list">{interfaces.filter(item => !item.is_loopback).map(item => <div className="interface-row" key={item.name}><span>{item.name}</span><small>{(item.addresses ?? []).join(' / ')}</small></div>)}</div>{effective && <p className="field-help">当前绑定：{effective.bind_address || '自动'} · 保存后用于新连接</p>}<button className="primary" disabled={!!op || !available || mergeRequired} onClick={() => save('network', 'preferences-network', '网络设置已保存；健康传输继续运行')}>保存网络设置</button></section>}
      {category === 'storage' && <section className="surface settings-page"><span className="section-kicker">存储与记录</span><h2>旧版内容与应用暂存</h2><p className="field-help">旧版文字、链接和图片入口已移除。已有草稿、队列和历史不会自动续发，可在记录与清理区处理。</p><ContentSettings run={controlRun} op={op} available={available} onOpenQueue={onOpenLegacyQueue} /></section>}
      {category === 'diagnostics' && <section className="surface settings-page"><span className="section-kicker">诊断与关于</span><h2>LinkSend 0.5.0</h2><dl className="settings-diagnostics"><div><dt>后台接收</dt><dd>{inbox?.listening ? '正在等待请求' : '尚未就绪'}</dd></div><div><dt>附近发现</dt><dd>{inbox?.lan_available ? '可用' : '不可用'}</dd></div><div><dt>信令健康</dt><dd>{diagnostics?.server_health || '未检查'}</dd></div><div><dt>任务恢复</dt><dd>{diagnostics?.restart_recovery_supported ? '可用' : '不可用'}</dd></div><div><dt>中继</dt><dd>未实现</dd></div></dl></section>}
    </div>
  </div>;
}
