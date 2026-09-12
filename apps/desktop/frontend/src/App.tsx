/* eslint-disable @typescript-eslint/no-explicit-any, @typescript-eslint/no-unsafe-function-type, no-irregular-whitespace */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import * as Backend from '../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import * as main from '../bindings/github.com/Wen5555/LinkSend/apps/desktop/models';
import * as app from '../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { connectionMethodLabel, formatPairingCodeInput, humanizeBackendError, mergeTaskSnapshots, shouldClearPairingCode, taskPhaseLabel } from './connection';
import './App.css';

type Tab = 'transfer' | 'devices' | 'settings';
const fmt = (n?: number | null) => n == null ? '未知' : n < 1024 ? `${n} B` : n < 1048576 ? `${(n / 1024).toFixed(1)} KB` : n < 1073741824 ? `${(n / 1048576).toFixed(1)} MB` : `${(n / 1073741824).toFixed(1)} GB`;
const labels: Record<string,string> = { preparing:'准备中', awaiting_acceptance:'等待确认', transferring:'传输中', verifying:'校验中', recovering:'待恢复', paused:'已暂停', pause_requested:'正在暂停', rejected:'已拒绝', completed:'已完成', failed:'失败', cancelled:'已取消', cancel_requested:'正在取消' };
const bridge = () => Boolean((window as any)._wails?.invoke);

export default function App() {
  const [tab,setTab]=useState<Tab>('transfer');
  const [membership,setMembership]=useState<any>();
  const [effective,setEffective]=useState<any>();
  const [prefsStatus,setPrefsStatus]=useState<any>();
  const [status,setStatus]=useState<main.DesktopStatus>();
  const [diag,setDiag]=useState<app.Diagnostics>();
  const [inbox,setInbox]=useState<app.InboxStatus>();
  const [devices,setDevices]=useState<app.DeviceInfo[]>([]);
  const [tasks,setTasks]=useState<app.TaskSnapshot[]>([]);
  const [identity,setIdentity]=useState<app.IdentityInfo>();
  const [prefs,setPrefs]=useState<main.DesktopPreferences>({format_version:1,server_url:'',bind_address:'',interface_priority:[],excluded_interfaces:[],stun_urls:[],receive_directory:'',device_name:''});
  const [ifaces,setIfaces]=useState<main.NetworkInterfaceInfo[]>([]);
  const [peer,setPeer]=useState('');
  const [paths,setPaths]=useState<string[]>([]);
  const [receiveDir,setReceiveDir]=useState('');
  const [error,setError]=useState('');
  const [notice,setNotice]=useState('');
  const [op,setOp]=useState('');
  const [invite,setInvite]=useState<app.InvitationInfo>();
  const [joinToken,setJoinToken]=useState('');

  const taskBusy=useRef(false), remoteBusy=useRef(false), operationBusy=useRef(false);
  const loadTasks=useCallback(async()=>{
    if(!bridge()||taskBusy.current)return;
    taskBusy.current=true;
    try{const incoming=(await Backend.Tasks())??[];setTasks(current=>mergeTaskSnapshots(current,incoming))}
    catch(e){setError(humanizeBackendError(e))}finally{taskBusy.current=false}
  },[]);
  const loadRemote=useCallback(async()=>{
    if(!bridge()||remoteBusy.current)return;
    remoteBusy.current=true;
    try{
      const [d,x,s,m,i]=await Promise.allSettled([Backend.Devices(),Backend.Diagnostics(),Backend.Status(),Backend.Membership(),Backend.InboxStatus()]);
      if(d.status==='fulfilled')setDevices(d.value??[]);
      if(x.status==='fulfilled')setDiag(x.value);
      if(s.status==='fulfilled')setStatus(s.value);
      if(m.status==='fulfilled')setMembership(m.value);
      if(i.status==='fulfilled')setInbox(i.value);
    }finally{remoteBusy.current=false}
  },[]);

  useEffect(()=>{
    if(!bridge()){setError('桌面后端未注入。请启动 LinkSend 桌面应用，浏览器预览仅用于界面检查。');return}
    void Promise.allSettled([Backend.Preferences(),Backend.Identity(),Backend.NetworkInterfaces(),Backend.EffectiveConfig(),Backend.PreferencesStatus()]).then(([p,i,n,c,ps])=>{
      if(p.status==='fulfilled'){setPrefs(p.value);setReceiveDir(p.value.receive_directory||'')}
      if(i.status==='fulfilled')setIdentity(i.value);
      if(n.status==='fulfilled')setIfaces(n.value??[]);
      if(c.status==='fulfilled')setEffective(c.value);
      if(ps.status==='fulfilled')setPrefsStatus(ps.value);
    });
    void loadTasks();void loadRemote();
    const a=window.setInterval(()=>void loadTasks(),700),b=window.setInterval(()=>void loadRemote(),3000);
    return()=>{window.clearInterval(a);window.clearInterval(b)}
  },[loadTasks,loadRemote]);

  const active=tasks.find(t=>t.can_cancel&&!t.can_resume&&!['completed','failed','cancelled','rejected'].includes(t.state));
  const paired=devices.filter(d=>(d.trusted||d.nearby)&&d.id!==status?.identity);
  const selected=devices.find(d=>d.id===peer);
  const incoming=[...tasks].reverse().find(t=>t.direction==='receive'&&t.state==='awaiting_acceptance');
  const run=async(k:string,fn:()=>Promise<unknown>,msg?:string,onError?:(error:unknown)=>void)=>{
    if(operationBusy.current)return;
    operationBusy.current=true;
    setOp(k);setError('');
    try{await fn();if(msg)setNotice(msg);await loadTasks();await loadRemote()}
    catch(e){setError(humanizeBackendError(e));onError?.(e)}finally{operationBusy.current=false;setOp('')}
  };
  const pickFiles=()=>run('pick-files',async()=>{const p=await Backend.PickFiles();if(p?.length)setPaths([...paths,...p.filter(x=>!paths.includes(x))])});
  const pickDir=()=>run('pick-dir',async()=>{const p=await Backend.PickDirectory();if(p){const next={...prefs,receive_directory:p};await Backend.SavePreferences(next);setReceiveDir(p);setPrefs(next)}},'接收目录已更新，后台接收已自动开启');
  const send=()=>run('send',async()=>{await Backend.StartSend(peer,paths);setPaths([])},'已向对方发送确认请求');
  const save=()=>run('save',()=>Backend.SavePreferences({...prefs,receive_directory:receiveDir}),'设置已保存');
  const states=useMemo(()=>[
    ['服务可用',!!status?.ready],['局域网发现',!!inbox?.lan_available],['自动接收在线',!!inbox?.signaling_connected||!!inbox?.lan_available],['目标设备可达',paired.some(d=>d.online)],['直连已建立',tasks.some(t=>['connected','awaiting_acceptance','transferring','verifying'].includes(t.phase))]
  ] as [string,boolean][],[status,diag,inbox,paired,tasks]);

  return <div className="app-shell">
    <aside className="sidebar"><div className="brand"><div className="brand-mark">↗</div><div><strong>LinkSend</strong><span>点对点文件传输</span></div></div><nav aria-label="主导航"><button className={tab==='transfer'?'nav-item active':'nav-item'} onClick={()=>setTab('transfer')}>⇄　传输</button><button className={tab==='devices'?'nav-item active':'nav-item'} onClick={()=>setTab('devices')}>◉　设备</button><button className={tab==='settings'?'nav-item active':'nav-item'} onClick={()=>setTab('settings')}>⚙　设置与诊断</button></nav><div className="sidebar-bottom"><div className="profile-chip"><span className="avatar">{(prefs.device_name||'L')[0]}</span><div><strong>{prefs.device_name||'本机设备'}</strong><small>{identity?identity.id.slice(0,10)+'…':'未连接'}</small></div></div><div className="relay-note"><span className="dot"/>端到端加密直连</div></div></aside>
    <section className="workspace"><header className="topbar"><div><p className="eyebrow">{tab==='transfer'?'工作台':tab==='devices'?'配对':'本地配置'}</p><h1>{tab==='transfer'?'传输':tab==='devices'?'设备':'设置与诊断'}</h1></div><div className="top-status"><span className={inbox?.signaling_connected||inbox?.lan_available?'status-dot ready':'status-dot'}/>{inbox?.listening?'可接收文件':inbox?.lan_available?'局域网接收在线':inbox?.signaling_connected?'接收连接在线':'正在连接'}<button className="icon-button" aria-label="刷新" onClick={()=>{void Backend.RefreshLANDiscovery().catch(()=>undefined);void loadTasks();window.setTimeout(()=>void loadRemote(),250)}}>↻</button></div></header>{error&&<div className="banner error-banner" role="alert"><span>!</span><p>{error}</p><button onClick={()=>setError('')} aria-label="关闭提示">×</button></div>}{notice&&<div className="banner notice-banner" role="status"><span>✓</span><p>{notice}</p><button onClick={()=>setNotice('')} aria-label="关闭提示">×</button></div>}{tab==='transfer'?<Transfer {...{states,paired,selected,peer,setPeer,paths,setPaths,receiveDir,active,pickFiles,pickDir,send,op,tasks,run,inbox}}/>:tab==='devices'?<Devices {...{devices,membership,invite,setInvite,joinToken,setJoinToken,run,op,prefs,status}}/>:<Settings {...{prefs,setPrefs,receiveDir,setReceiveDir,ifaces,diag,save,effective,prefsStatus,inbox}}/>}</section>
    {incoming&&<IncomingConfirm task={incoming} device={devices.find(d=>d.id===incoming.peer_id)} run={run} op={op}/>}
  </div>
}

function Transfer(p:any){return <div className="page-stack"><div className="connection-strip">{p.states.map(([l,ok]:[string,boolean])=><div className="connection-item" key={l}><span className={ok?'status-dot ready':'status-dot'}/><span>{l}</span><small>{ok?'已就绪':'等待中'}</small></div>)}</div><div className="transfer-grid"><section className="surface send-surface"><div className="section-heading"><div><span className="section-kicker">发送</span><h2>选择内容，直接发给对方</h2></div><span className="step-icon blue">↑</span></div><label className="field-label">发送给<select value={p.peer} onChange={(e:any)=>p.setPeer(e.target.value)}><option value="">选择设备</option>{p.paired.map((d:app.DeviceInfo)=><option key={d.id} value={d.id}>{d.name||'未命名设备'} · {d.nearby?'局域网内':d.online?'在线':'暂时离线'}</option>)}</select></label>{p.selected&&<div className="peer-inline"><span className="avatar small">{(p.selected.name||'D')[0]}</span><div><strong>{p.selected.name||'未命名设备'}</strong><small>{p.selected.nearby?(p.selected.trusted?'局域网已发现 · 可以直接发送':'局域网新设备 · 对方确认后建立信任'):`已配对 · ${p.selected.online?'可以发送':'等待对方应用上线'}`}</small></div></div>}<div className="picker-row"><button className="secondary" onClick={p.pickFiles}>＋ 选择文件</button><button className="secondary" onClick={()=>p.run('pick-source',async()=>{const x=await Backend.PickSourceDirectory();if(x)p.setPaths([...p.paths,x])})}>＋ 选择文件夹</button></div><div className="file-list">{p.paths.length?p.paths.map((x:string,i:number)=><div className="file-row" key={x+i}><span className="file-icon">▧</span><span title={x}>{x.split(/[\\/]/).pop()}</span><button aria-label="移除" onClick={()=>p.setPaths(p.paths.filter((_:string,n:number)=>n!==i))}>×</button></div>):<div className="empty-picker"><span>＋</span><strong>选择要发送的文件或文件夹</strong><small>点击发送后，对方会收到确认弹窗</small></div>}</div><button className="primary full send-button" disabled={!!p.active||!p.peer||!p.paths.length||p.op==='send'} onClick={p.send}>{p.active?'当前传输完成后再发送':'发送给对方'}</button><p className="hint">{!p.peer?'请选择一台已配对或局域网发现的设备':!p.paths.length?'请选择至少一个文件或文件夹':'无需让对方提前点击“开始接收”'}</p></section><section className="surface receive-ready"><div className="section-heading"><div><span className="section-kicker">接收</span><h2>{p.inbox?.listening?'已准备好接收':p.inbox?.lan_available?'局域网发现与接收在线':p.inbox?.signaling_connected?'接收连接保持在线':'正在恢复接收'}</h2></div><span className={p.inbox?.signaling_connected||p.inbox?.lan_available?'receive-orb ready':'receive-orb'}>↓</span></div><p className="intro">应用打开时自动等待请求。收到文件后会弹出确认，不需要先点开始。</p><div className="receiver-status"><span className={p.inbox?.signaling_connected||p.inbox?.lan_available?'status-dot ready':'status-dot'}/><div><strong>{p.inbox?.listening?'后台接收已开启':p.inbox?.lan_available?`局域网发现在线 · ${p.inbox.lan_peer_count||0} 台附近设备`:p.inbox?.signaling_connected?'信令在线，正在处理当前任务':'后台接收正在连接'}</strong><small>{p.inbox?.lan_last_error?`局域网状态：${p.inbox.lan_last_error}`:p.inbox?.last_error?`最近状态：${p.inbox.last_error}`:'局域网优先，公网信令自动兜底'}</small></div></div><label className="field-label">保存到<input value={p.receiveDir} readOnly placeholder="请选择接收目录"/></label><button className="secondary full" onClick={p.pickDir}>更改接收目录</button><div className="receive-note"><span>i</span><p>局域网新设备不会自动获得信任；你确认接收且传输完成后才会保存。</p></div></section></div><section className="tasks-section"><div className="section-heading compact"><div><span className="section-kicker">活动与记录</span><h2>任务</h2></div><span className="task-count">{p.tasks.length} 个</span></div>{p.tasks.length?<div className="task-list">{p.tasks.slice().reverse().map((t:app.TaskSnapshot)=><Task key={t.id} t={t} run={p.run}/>)}</div>:<div className="empty-state"><div className="empty-illustration">⇄</div><strong>这里会显示传输记录</strong><p>选择设备和文件后即可发送；接收无需提前操作。</p></div>}</section></div>}

function IncomingConfirm({task,device,run,op}:{task:app.TaskSnapshot,device?:app.DeviceInfo,run:Function,op:string}){
  const [remember,setRemember]=useState(false);
  useEffect(()=>setRemember(false),[task.id]);
  const accept=()=>run(`accept-${task.id}`,()=>remember?Backend.AcceptTaskAlways(task.id):Backend.AcceptTask(task.id),remember?'已接收；以后该设备将自动接收':'已确认接收，传输即将开始');
  return <div className="modal-backdrop" role="presentation"><section className="incoming-modal" role="dialog" aria-modal="true" aria-labelledby="incoming-title"><div className="incoming-icon">↓</div><p className="eyebrow">收到传输请求</p><h2 id="incoming-title">{device?.name||'已配对设备'} 想发送文件</h2><div className="incoming-summary"><strong>{task.manifest_summary||'文件传输'}</strong><span>{task.file_count||0} 项 · {fmt(task.total_bytes)}</span><small>保存到 {task.target_directory}</small></div><label className="remember-choice"><input type="checkbox" checked={remember} onChange={e=>setRemember(e.target.checked)}/><span><strong>以后自动接收此设备的文件</strong><small>仍会校验设备密钥和文件完整性</small></span></label><div className="modal-actions"><button className="ghost danger" disabled={!!op} onClick={()=>run(`reject-${task.id}`,()=>Backend.RejectTask(task.id),'已拒绝本次传输')}>拒绝</button><button className="primary" disabled={!!op} onClick={accept}>确认接收</button></div></section></div>
}

function Task({t,run}:{t:app.TaskSnapshot,run:Function}){
  const pct=t.total_bytes&&t.total_bytes>0?Math.min(100,Math.round(t.processed_bytes/t.total_bytes*100)):0;
  const terminal=['completed','failed','cancelled','rejected'].includes(t.state);
  const actual=t.direction==='send'?`已实际发送 ${fmt(t.sent_bytes)}`:`已实际接收 ${fmt(t.received_bytes)}`;
  return <article className="task-row"><div className={`task-symbol ${t.direction}`}>{t.direction==='send'?'↑':'↓'}</div><div className="task-main"><div className="task-title"><strong>{t.direction==='send'?'发送':'接收'} · {t.source_summary||t.manifest_summary||'文件传输'}</strong><span className={`state-badge ${t.state}`}>{labels[t.state]||'未知状态'}</span></div>{t.manifest_summary&&<div className="task-meta">内容：{t.manifest_summary} · {t.file_count||0} 项</div>}<div className="task-meta">{taskPhaseLabel(t.phase)} · {t.peer_id?t.peer_id.slice(0,10)+'…':'等待对端'}{t.connection_method&&<> · {connectionMethodLabel(t.connection_method)}</>}{t.rate_bytes_per_second?` · ${(t.rate_bytes_per_second/1048576).toFixed(1)} MB/s`:''}</div>{!terminal&&<div className="progress-line" role="progressbar" aria-label="已验证进度" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}><span style={{width:`${pct}%`}}/></div>}<div className="task-accounting"><span>{actual}</span>{t.retransmitted_bytes>0&&<span>其中重传 {fmt(t.retransmitted_bytes)}</span>}<span>已验证 {fmt(t.verified_bytes)}</span><span>已提交 {fmt(t.committed_bytes)}{t.committed_files>0?` · ${t.committed_files} 个文件`:''}</span><span>{t.bilateral_confirmed?'双方已确认':'等待完成确认'}</span></div><div className="task-foot"><span>{t.total_bytes==null?`逻辑完成 ${fmt(t.processed_bytes)} · 总量未知`:`逻辑完成 ${fmt(t.processed_bytes)} / ${fmt(t.total_bytes)}`}</span>{t.error_message&&<span className="task-error">{t.error_code} · {t.error_message}</span>}</div></div><div className="task-actions">{t.can_pause&&<button className="secondary" onClick={()=>run(t.id,()=>Backend.PauseTask(t.id))}>暂停</button>}{t.can_resume&&<button className="primary" onClick={()=>run(t.id,()=>Backend.ResumeTask(t.id),'正在建立新连接并继续传输')}>继续</button>}{t.can_cancel&&t.state!=='awaiting_acceptance'&&<button className="ghost" onClick={()=>run(t.id,()=>Backend.CancelTask(t.id))}>{t.state==='cancel_requested'?'正在取消':'取消'}</button>}{t.can_retry&&<button className="secondary" onClick={()=>run(t.id,()=>Backend.RetryTask(t.id))}>重新发送</button>}{t.state==='completed'&&t.direction==='receive'&&t.target_directory&&<button className="secondary" onClick={()=>Backend.OpenTaskDirectory(t.id)}>打开文件夹</button>}</div></article>
}

function Devices(p:any){const isMember=p.membership?.state==='member';const peers=p.devices.filter((d:app.DeviceInfo)=>d.id!==p.status?.identity);return <div className="page-stack"><div className="page-intro"><div><p className="eyebrow">附近直连，远程长期配对</p><h2>连接另一台设备</h2><p>同一局域网会自动发现并由双方确认；跨网络可使用一次性配对码。</p></div><button className="primary" onClick={()=>p.run('invite',async()=>p.setInvite(await Backend.CreateInvitation()))} disabled={!isMember||p.op==='invite'}>{isMember?'生成配对码':'等待首次配对'}</button></div><LANProbe run={p.run} op={p.op}/>{p.invite&&<InvitationPanel invite={p.invite} run={p.run}/>}<div className="device-columns"><section className="surface"><div className="section-heading compact"><div><span className="section-kicker">附近与已配对</span><h2>可用设备</h2></div><span className="task-count">{peers.length}</span></div>{peers.length?peers.map((d:app.DeviceInfo)=><article className="device-row" key={d.id}><span className="avatar">{(d.name||'D')[0]}</span><div><strong>{d.name||'未命名设备'}</strong><small>{d.trusted?'已配对':d.nearby?'局域网发现 · 首次发送需双方确认':'未配对'} · {d.online?'在线':'离线'}{d.always_accept?' · 自动接收已开启':''}</small><code>设备 {d.id.slice(0,8)}</code></div><div className="device-actions"><span className={d.online?'online-label':'muted-label'}>{d.nearby?'附近':d.online?'在线':'离线'}</span>{d.always_accept&&<button className="ghost" onClick={()=>p.run(`confirm-${d.id}`,()=>Backend.SetAlwaysAccept(d.id,false),'该设备下次发送时将重新确认')}>恢复确认</button>}</div></article>):<div className="empty-state small"><strong>还没有发现设备</strong><p>请让另一台设备打开 LinkSend；跨网络时可生成配对码。</p></div>}</section><section className="surface pairing-form"><span className="section-kicker">输入配对码</span><h2>跨网络配对</h2><p className="muted">输入后即完成长期配对，以后无需再次认证。</p><label className="field-label">配对码<input className="pairing-input" value={p.joinToken} onChange={(e:any)=>p.setJoinToken(formatPairingCodeInput(e.target.value))} placeholder="ABCD-EFGH" maxLength={43} autoComplete="off" spellCheck={false}/></label><button className="primary full" disabled={!p.joinToken.trim()||p.op==='join'} onClick={()=>p.run('join',async()=>{await Backend.PairDevice(p.joinToken.trim(),p.prefs.device_name||'LinkSend desktop');p.setJoinToken('')},'配对成功，可以直接传输',(error:unknown)=>{if(shouldClearPairingCode(error))p.setJoinToken('')})}>完成配对</button>{p.membership?.message&&<p className="field-help pairing-help">{p.membership.message}</p>}</section></div></div>}

function LANProbe({run,op}:{run:Function,op:string}){
  const [address,setAddress]=useState('');
  return <section className="surface lan-probe"><div><span className="section-kicker">自动发现兜底</span><strong>附近设备没有出现？</strong><small>校园网或访客网络可能过滤组播。输入对方当前局域网 IPv4 地址，只探测这个同网段地址。</small></div><div className="lan-probe-actions"><input value={address} onChange={event=>setAddress(event.target.value)} placeholder="例如 10.234.171.192" inputMode="decimal" spellCheck={false}/><button className="secondary" disabled={!address.trim()||op==='lan-probe'} onClick={()=>run('lan-probe',()=>Backend.ProbeLANAddress(address.trim()),'已发送定向发现请求')}>查找</button></div></section>
}

function InvitationPanel({invite,run}:{invite:app.InvitationInfo,run:Function}){
  const [remaining,setRemaining]=useState(Math.max(0,invite.expires_in_seconds??0));
  useEffect(()=>{
    const initial=Math.max(0,invite.expires_in_seconds??0);setRemaining(initial);
    if(!initial)return;
    const started=Date.now();
    const timer=window.setInterval(()=>setRemaining(Math.max(0,initial-Math.floor((Date.now()-started)/1000))),1000);
    return()=>window.clearInterval(timer);
  },[invite.token,invite.expires_in_seconds]);
  const expiry=remaining>0?`${Math.floor(remaining/60)}:${String(remaining%60).padStart(2,'0')} 后过期`:'已过期，请重新生成';
  return <div className={`invite-panel pairing-code-panel ${remaining===0?'expired':''}`}><div><span className="section-kicker">{expiry} · 使用一次后失效</span><p className="pairing-code">{invite.token}</p><small>{remaining>0?'让另一台设备输入这个配对码':'这个配对码不会再被提交，请生成新的配对码'}</small></div><button className="secondary" disabled={remaining===0} onClick={()=>run('copy-invite',async()=>{if(!navigator.clipboard)throw new Error('复制失败：当前窗口不允许访问剪贴板');await navigator.clipboard.writeText(invite.token)},'配对码已复制')}>复制配对码</button></div>
}

function Settings(p:any){const update=(patch:Partial<main.DesktopPreferences>)=>p.setPrefs({...p.prefs,...patch});return <div className="page-stack"><div className="page-intro"><div><p className="eyebrow">本地偏好</p><h2>网络与保存位置</h2><p>通常无需修改网络选项；接收目录保存后立即生效。</p>{p.prefsStatus?.state&&p.prefsStatus.state!=='valid'&&<div className="banner error-banner">{p.prefsStatus.message||'偏好文件异常，原文件已保留'}</div>}</div><button className="primary" onClick={p.save}>保存设置</button></div><div className="settings-grid"><section className="surface"><span className="section-kicker">连接</span><h2>服务与网卡</h2><label className="field-label">信令服务地址<input value={p.prefs.server_url||''} onChange={(e:any)=>update({server_url:e.target.value})} placeholder="https://linksend.oooai.de"/></label><label className="field-label">本机绑定地址<input value={p.prefs.bind_address||''} onChange={(e:any)=>update({bind_address:e.target.value})} placeholder="留空自动选择；或填 192.168.1.20:0"/></label><p className="field-help">默认使用系统可用接口；仅在虚拟网卡抢占连接时指定物理网卡。</p><label className="field-label">接口优先级<input value={(p.prefs.interface_priority||[]).join(', ')} onChange={(e:any)=>update({interface_priority:e.target.value.split(',').map((x:string)=>x.trim()).filter(Boolean)})} placeholder="按顺序填写接口名，逗号分隔"/></label><label className="field-label">排除接口<input value={(p.prefs.excluded_interfaces||[]).join(', ')} onChange={(e:any)=>update({excluded_interfaces:e.target.value.split(',').map((x:string)=>x.trim()).filter(Boolean)})} placeholder="例如虚拟网卡接口名"/></label>{p.effective&&<p className="field-help">当前服务：{p.effective.server_url}；绑定：{p.effective.bind_address||'自动'}</p>}<div className="interface-list">{p.ifaces.filter((x:main.NetworkInterfaceInfo)=>!x.is_loopback).map((x:main.NetworkInterfaceInfo)=><div className="interface-row" key={x.name}><span>{x.name}</span><small>{(x.addresses??[]).map((address:string,index:number)=>`${address} · ${x.address_families?.[index]||'未知地址族'}`).join(' / ')}</small></div>)}</div><label className="field-label">STUN 地址（高级）<input value={(p.prefs.stun_urls||[]).join(', ')} onChange={(e:any)=>update({stun_urls:e.target.value.split(',').map((x:string)=>x.trim()).filter(Boolean)})} placeholder="stun:stun.oooai.de:3478"/></label></section><section className="surface"><span className="section-kicker">接收</span><h2>设备与文件</h2><label className="field-label">本机名称<input value={p.prefs.device_name||''} onChange={(e:any)=>update({device_name:e.target.value})} placeholder="例如：耀文的电脑"/></label><label className="field-label">默认接收目录<input value={p.receiveDir} readOnly placeholder="请选择目录"/></label><button className="secondary full" onClick={async()=>{const x=await Backend.PickDirectory();if(x){p.setReceiveDir(x);update({receive_directory:x})}}}>选择目录</button><div className="diagnostic-block"><span className="section-kicker">只读诊断</span><dl><div><dt>后台接收</dt><dd>{p.inbox?.listening?'正在监听':p.inbox?.signaling_connected?'信令在线':'正在连接'}</dd></div><div><dt>接收连接次数</dt><dd>{p.inbox?.connection_count??0}</dd></div><div><dt>信令健康</dt><dd>{p.diag?.server_health||'未检查'}</dd></div><div><dt>中继</dt><dd>未启用</dd></div><div><dt>任务历史</dt><dd>{p.diag?.history_persisted?'已持久化':'不可用'}</dd></div><div><dt>重启恢复</dt><dd>{p.diag?.restart_recovery_supported?'可用':'不可用'}</dd></div><div><dt>字节级续传</dt><dd>{p.diag?.byte_resume_supported?'可用':'不可用'}</dd></div></dl></div></section></div></div>}
