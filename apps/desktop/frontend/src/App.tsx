import { useCallback, useEffect, useState } from 'react';
import * as Backend from '../wailsjs/go/main/App';
import { app } from '../wailsjs/go/models';
import { connectionMethodLabel } from './connection';
import './App.css';

const bytes = (n: number) => n < 1024 ? `${n} B` : n < 1024 ** 2 ? `${(n / 1024).toFixed(1)} KB` : `${(n / 1024 ** 2).toFixed(1)} MB`;

function App() {
  const [tab, setTab] = useState<'transfer' | 'devices' | 'settings'>('transfer');
  const [status, setStatus] = useState('正在连接桌面后端…');
  const [ownID, setOwnID] = useState('');
  const [error, setError] = useState('');
  const [devices, setDevices] = useState<app.DeviceInfo[]>([]);
  const [diagnostics, setDiagnostics] = useState<app.Diagnostics | null>(null);
  const [tasks, setTasks] = useState<app.TaskSnapshot[]>([]);
  const [peer, setPeer] = useState('');
  const [paths, setPaths] = useState<string[]>([]);
  const [receiveDir, setReceiveDir] = useState('');
  const [busy, setBusy] = useState(false);
  const refresh = useCallback(async () => {
    const results = await Promise.allSettled([Backend.Devices(), Backend.Tasks(), Backend.Status(), Backend.Diagnostics()]);
    const [d, t, shell, diag] = results;
    if (d.status === 'fulfilled') setDevices(d.value);
    if (t.status === 'fulfilled') setTasks(t.value);
    if (shell.status === 'fulfilled') { setStatus(`${shell.value.platform} · ${shell.value.version}`); setOwnID(shell.value.identity || ''); }
    if (diag.status === 'fulfilled') setDiagnostics(diag.value);
    const failure = results.find(r => r.status === 'rejected');
    if (failure && failure.status === 'rejected') setError(String(failure.reason).replace(/^Error: /, '') || '桌面后端未连接，请从 Wails 应用启动。'); else setError('');
  }, []);
  useEffect(() => { void refresh(); const timer = window.setInterval(() => void refresh(), 1000); return () => window.clearInterval(timer); }, [refresh]);
  const chooseFiles = async () => { try { const picked = await Backend.PickFiles(); if (picked.length) setPaths(picked); } catch (cause) { setError(String(cause)); } };
  const chooseDir = async () => { try { const picked = await Backend.PickDirectory(); if (picked) setReceiveDir(picked); } catch (cause) { setError(String(cause)); } };
  const chooseSourceDir = async () => { try { const picked = await Backend.PickSourceDirectory(); if (picked) setPaths([picked]); } catch (cause) { setError(String(cause)); } };
  const send = async () => { setBusy(true); setError(''); try { await Backend.StartSend(peer, paths); setPaths([]); } catch (cause) { setError(String(cause)); } finally { setBusy(false); } };
  const receive = async () => { setBusy(true); setError(''); try { await Backend.StartReceive('', receiveDir); } catch (cause) { setError(String(cause)); } finally { setBusy(false); } };
  const action = async (fn: () => Promise<unknown>) => { try { await fn(); await refresh(); } catch (cause) { setError(String(cause)); } };
  return <main>
    <header><div><span className="wordmark">LinkSend</span><span className="preview">开发预览</span></div><span className="backend">{status}</span></header>
    <nav><button className={tab === 'transfer' ? 'active' : ''} onClick={() => setTab('transfer')}>传输</button><button className={tab === 'devices' ? 'active' : ''} onClick={() => setTab('devices')}>设备</button><button className={tab === 'settings' ? 'active' : ''} onClick={() => setTab('settings')}>设置 / 诊断</button></nav>
    {error && <p role="alert" className="error">{error}</p>}
    {tab === 'transfer' && <>
      <section className="grid"><div className="panel"><h2>发送</h2><label>可信设备<select value={peer} onChange={e => setPeer(e.target.value)}><option value="">选择设备</option>{devices.filter(d => d.trusted && d.id !== ownID).map(d => <option key={d.id} value={d.id}>{d.name || d.id.slice(0, 12)} · {d.online ? '在线' : '状态未知'}</option>)}</select></label><button onClick={() => void chooseFiles()}>选择文件</button> <button onClick={() => void chooseSourceDir()}>选择目录</button><p className="muted">{paths.length ? paths.join('、') : '尚未选择文件或目录'}</p><button className="primary" disabled={busy || !peer || !paths.length} onClick={() => void send()}>发起发送</button></div>
      <div className="panel"><h2>接收</h2><label>接收目录<input value={receiveDir} readOnly placeholder="请选择目录" /></label><button onClick={() => void chooseDir()}>选择目录</button><p className="muted">接收请求会在任务中等待确认，不会自动接收陌生设备。</p><button className="primary" disabled={busy || !receiveDir} onClick={() => void receive()}>开始等待接收</button></div></section>
      <section><div className="section-title"><h2>任务</h2><button onClick={() => void refresh()}>刷新</button></div>{tasks.length === 0 ? <p className="empty">当前进程还没有任务。</p> : <div className="tasks">{tasks.slice().reverse().map(t => <article className="task" key={t.id}><div><strong>{t.direction === 'send' ? '发送' : '接收'} · {t.source_summary || t.target_directory}</strong><span className={`state state-${t.state}`}>{t.state}</span><p className="muted">{t.phase} · {t.total_bytes ? `${bytes(t.processed_bytes)} / ${bytes(t.total_bytes)}` : bytes(t.processed_bytes)}{t.rate_bytes_per_second ? ` · ${(t.rate_bytes_per_second / 1024 / 1024).toFixed(1)} MB/s` : ''}</p>{t.error_message && <p className="task-error">{t.error_code}: {t.error_message}</p>}</div><div className="task-actions">{t.state === 'awaiting_acceptance' && <><button onClick={() => void action(() => Backend.AcceptTask(t.id))}>接受</button><button onClick={() => void action(() => Backend.RejectTask(t.id))}>拒绝</button></>}{t.can_cancel && <button onClick={() => void action(() => Backend.CancelTask(t.id))}>取消</button>}{t.can_retry && <button onClick={() => void action(() => Backend.RetryTask(t.id))}>重新发送</button>}</div></article>)}</div>}</section>
    </>}
    {tab === 'devices' && <section><h2>已配对设备</h2>{devices.length === 0 ? <p className="empty">暂无设备或后端不可用。</p> : <div className="devices">{devices.map(d => <article className="device" key={d.id}><strong>{d.name || '未命名设备'}</strong><span>{d.trusted ? '已信任' : '待验证'} · {d.online ? '在线' : '状态未知'}</span><code>{d.id}</code></article>)}</div>}</section>}
    {tab === 'settings' && <section><h2>设置 / 诊断</h2><p className="muted">配置通过环境变量提供，当前页面只读展示。</p><dl><div><dt>连接方法</dt><dd>{connectionMethodLabel('direct_unknown')}</dd></div><div><dt>信令服务</dt><dd>{diagnostics?.server_health || '未知'}</dd></div><div><dt>中继</dt><dd>未实现（relay=false）</dd></div><div><dt>任务生命周期</dt><dd>仅当前进程，暂不提供重启恢复</dd></div><div><dt>身份指纹</dt><dd>{diagnostics?.identity?.id ? `${diagnostics.identity.id.slice(0, 16)}…` : '未知'}</dd></div><div><dt>运行环境</dt><dd>{status}</dd></div></dl></section>}
    <footer>文件字节只经过已认证的 QUIC 直连，不经过前端或信令服务器。</footer>
  </main>;
}
export default App;
