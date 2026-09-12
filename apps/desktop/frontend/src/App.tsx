import { useEffect, useRef, useState } from 'react';
import * as Backend from '../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DesktopPreferences } from '../bindings/github.com/Wen5555/LinkSend/apps/desktop/models';
import { useCommand, useDesktop } from './hooks/useDesktop';
import { CommandLanes, EnqueueIdentity } from './workspace-cache';
import { humanizeBackendError } from './connection';
import { TransferPage } from './pages/TransferPage';
import { DevicesPage } from './pages/DevicesPage';
import { SettingsPage } from './pages/SettingsPage';
import { IncomingConfirm } from './components/Tasks';
import './App.css';

type Tab = 'transfer' | 'devices' | 'settings';
const tabs: { id: Tab; label: string; icon: string }[] = [
  { id: 'transfer', label: '传输', icon: '⇄' }, { id: 'devices', label: '设备', icon: '◉' }, { id: 'settings', label: '设置与诊断', icon: '⚙' },
];
const emptyPreferences: DesktopPreferences = { format_version: 1, server_url: '', bind_address: '', interface_priority: [], excluded_interfaces: [], stun_urls: [], receive_directory: '', device_name: '' };

export default function App() {
  const [tab, setTab] = useState<Tab>('transfer');
  const desktop = useDesktop();
  const lanes = useRef(new CommandLanes());
  const command = useCommand(desktop.refresh, lanes.current, 'draft');
  const control = useCommand(desktop.refresh, lanes.current, 'transfer-control');
  const enqueueIdentity = useRef(new EnqueueIdentity(() => Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('')));
  const workspace = desktop.workspace.data;
  useEffect(() => {
    for (const item of workspace?.queue ?? []) enqueueIdentity.current.confirmed(item.request_id);
  }, [workspace?.queue]);
  const devices = desktop.devices.data ?? [];
  const shell = desktop.shell.data;
  const entryRevision = useRef(0);
  useEffect(() => {
    const revision = shell?.entries.draft_revision ?? 0;
    if (revision > entryRevision.current) {
      entryRevision.current = revision;
      setTab('transfer');
      void desktop.refresh();
    }
  }, [shell?.entries.draft_revision, desktop.refresh]);
  const config = desktop.configuration.data;
  const prefs = desktop.preferences.data ?? emptyPreferences;
  const inbox = shell?.inbox;
  const available = desktop.enabled && !desktop.workspace.isError && !!workspace?.persistence_available;
  const incoming = [...(workspace?.tasks ?? [])].reverse().find(task => task.direction === 'receive' && task.state === 'awaiting_acceptance');
  const loadError = desktop.workspace.error ?? desktop.devices.error ?? desktop.shell.error ?? desktop.preferences.error ?? desktop.configuration.error;
  return <div className="app-shell">
    <aside className="sidebar"><div className="brand"><div className="brand-mark" aria-hidden="true">↗</div><div><strong>LinkSend</strong><span>点对点文件传输</span></div></div>
      <nav aria-label="主导航">{tabs.map(item => <button key={item.id} className={`nav-item ${tab === item.id ? 'active' : ''}`} aria-current={tab === item.id ? 'page' : undefined} onClick={() => setTab(item.id)}><span aria-hidden="true">{item.icon}</span> {item.label}</button>)}</nav>
      <div className="sidebar-bottom"><div className="profile-chip"><span className="avatar">{(prefs.device_name || 'L')[0]}</span><div><strong>{prefs.device_name || '本机设备'}</strong><small>{config?.identity.id ? `${config.identity.id.slice(0, 10)}…` : '未连接'}</small></div></div><div className="relay-note">端到端加密直连</div></div>
    </aside>
    <section className="workspace"><header className="topbar"><div><p className="eyebrow">{tab === 'transfer' ? '工作台' : tab === 'devices' ? '本机设备偏好' : '本地配置'}</p><h1>{tabs.find(item => item.id === tab)?.label}</h1></div><div className="top-status"><span className={inbox?.signaling_connected || inbox?.lan_available ? 'status-dot ready' : 'status-dot'} />{!desktop.enabled ? '后端未连接' : inbox?.listening ? '可以接收文件' : inbox?.lan_available ? '局域网接收在线' : inbox?.signaling_connected ? '接收连接在线' : '接收尚未就绪'}<button className="icon-button" disabled={!desktop.enabled || !!command.op} aria-label="刷新设备和工作台" onClick={() => void command.run('refresh', async () => { if (inbox?.lan_available) await Backend.RefreshLANDiscovery(); await desktop.refresh(); })}>↻</button></div></header>
      {!desktop.enabled && <div className="banner error-banner" role="status"><p>桌面后端未连接。请启动 LinkSend 桌面应用；浏览器预览只显示界面，不执行传输。</p></div>}
      {desktop.enabled && loadError && <div className="banner error-banner" role="alert"><p>{humanizeBackendError(loadError)}</p><button className="ghost" onClick={() => void desktop.refresh()}>重试读取</button></div>}
      {desktop.enabled && desktop.workspace.isPending && <p className="loading-status" role="status">正在读取保存的草稿与队列…</p>}
      {shell?.entries.error && <div className="banner error-banner" role="alert"><p>{shell.entries.error}</p></div>}
      {command.error && <div className="banner error-banner" role="alert"><p>{command.error}</p><button onClick={command.clearError} aria-label="关闭错误提示">×</button></div>}
      {command.notice && <div className="banner notice-banner" role="status"><p>{command.notice}</p><button onClick={command.clearNotice} aria-label="关闭操作提示">×</button></div>}
      {control.error && <div className="banner error-banner" role="alert"><p>{control.error}</p><button onClick={control.clearError} aria-label="关闭传输控制错误">×</button></div>}
      {control.notice && <div className="banner notice-banner" role="status"><p>{control.notice}</p><button onClick={control.clearNotice} aria-label="关闭传输控制提示">×</button></div>}
      {tab === 'transfer' ? <TransferPage workspace={workspace} devices={devices} identityID={shell?.status.identity} inbox={inbox} preferences={desktop.preferences.data} run={command.run} op={command.op} controlRun={control.run} controlOp={control.op} enqueueIdentity={enqueueIdentity.current} available={available} /> :
        tab === 'devices' ? <DevicesPage devices={devices} identityID={shell?.status.identity} membership={shell?.membership} name={prefs.device_name} run={command.run} op={command.op} available={desktop.enabled} /> :
          <SettingsPage sendToSupported={shell?.entries.send_to_supported} key={JSON.stringify(prefs)} preferences={prefs} effective={config?.effective} preferencesStatus={config?.preferencesStatus} interfaces={config?.interfaces ?? []} diagnostics={shell?.diagnostics} inbox={inbox} run={command.run} op={command.op} available={desktop.enabled} />}
    </section>
    {incoming && <IncomingConfirm key={incoming.id} task={incoming} device={devices.find(device => device.id === incoming.peer_id)} run={control.run} op={control.op} />}
  </div>;
}
