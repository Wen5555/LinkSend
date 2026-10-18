import { useEffect, useRef, useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { Status } from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/updater/models';
import { formatBytes } from '../presentation';

export function SoftwareUpdates({ available }: { available: boolean }) {
  const [status, setStatus] = useState<Status>();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const sequence = useRef(0);
  const mounted = useRef(false);
  const commandPending = useRef(false);

  useEffect(() => {
    mounted.current = true;
    if (available) {
      const read = ++sequence.current;
      void Backend.UpdateStatus().then(value => {
        if (mounted.current && read === sequence.current) setStatus(value);
      }).catch(() => {
        if (mounted.current && read === sequence.current) setError('无法读取更新状态，请重试检查');
      });
    }
    return () => { mounted.current = false; sequence.current++; };
  }, [available]);

  useEffect(() => {
    if (!available || !status?.busy || pending) return;
    const timer = window.setTimeout(() => {
      const read = ++sequence.current;
      void Backend.UpdateStatus().then(value => {
        if (mounted.current && read === sequence.current) { setStatus(value); setError(''); }
      }).catch(() => {
        if (mounted.current && read === sequence.current) {
          setError('更新状态读取失败，请重新打开此页面查看；下载仍由后台管理');
          setStatus(previous => previous ? { ...previous, busy: false, can_download: false, can_open: false } : previous);
        }
      });
    }, 600);
    return () => window.clearTimeout(timer);
  }, [available, pending, status]);

  const perform = async (action: () => Promise<Status>) => {
    if (commandPending.current) return;
    commandPending.current = true;
    const command = ++sequence.current;
    setPending(true); setError('');
    try {
      const value = await action();
      if (mounted.current && command === sequence.current) setStatus(value);
    } catch {
      if (mounted.current && command === sequence.current) setError('更新操作未能确认，请重新读取状态后重试');
    } finally {
      commandPending.current = false;
      if (mounted.current) setPending(false);
    }
  };
  const busy = pending || !!status?.busy;
  const percent = status?.package_bytes ? Math.min(100, Math.floor(status.downloaded_bytes * 100 / status.package_bytes)) : 0;

  return <section className="surface settings-page software-updates">
    <span className="section-kicker">GitHub 正式版</span><h2>软件更新</h2>
    <p className="field-help">从 Wen5555/LinkSend 获取最新正式版。点击检查后联网，下载完成并校验通过后可打开安装包。</p>
    <dl className="settings-diagnostics">
      <div><dt>当前版本</dt><dd>{status?.current_version || '读取中'}</dd></div>
      <div><dt>最新正式版</dt><dd>{status?.latest_version || '尚未检查'}</dd></div>
      {status?.platform && <div><dt>本机平台</dt><dd>{status.platform}</dd></div>}
    </dl>
    <p className="update-message" role="status">{status?.message || (available ? '点击检查更新' : '请在桌面应用中检查更新')}</p>
    {(error || status?.error) && <p className="banner error-banner" role="alert">{error || status?.error}</p>}
    {status?.stage === 'downloading' && <div className="update-progress">
      <progress aria-label="更新下载进度" value={status.downloaded_bytes} max={status.package_bytes || 1} />
      <span>{percent}% · {formatBytes(status.downloaded_bytes)} / {formatBytes(status.package_bytes)}</span>
    </div>}
    <div className="inline-actions">
      <button className="secondary" disabled={!available || busy} onClick={() => void perform(Backend.CheckForUpdates)}>检查更新</button>
      {status?.can_download && <button className="primary" disabled={!available || busy} onClick={() => void perform(Backend.DownloadUpdate)}>下载更新（{formatBytes(status.package_bytes)}）</button>}
      {status?.can_open && <button className="primary" disabled={!available || busy} onClick={() => void perform(Backend.OpenUpdatePackage)}>{status.stage === 'opened' ? '再次打开安装包' : '打开安装包'}</button>}
      {status?.busy && status.stage !== 'opening' && <button className="ghost" disabled={!available || pending || status.stage === 'cancelling'} onClick={() => void perform(Backend.CancelUpdate)}>取消</button>}
      <button className="ghost" disabled={!available} onClick={() => void Backend.OpenUpdateReleasePage().catch(() => setError('无法打开 GitHub 发布页，请检查系统浏览器设置'))}>查看 GitHub 发布页</button>
    </div>
    {status?.can_open && <p className="field-help">Windows 使用安装向导；Mac 打开 DMG 后将 LinkSend 拖入“应用程序”。安装前请退出 LinkSend（包括托盘后台），完成后重新启动。配置、配对与接收文件会保留；Windows 便携版用户也可通过安装向导安装新版。</p>}
    {status?.stage === 'opened' && <button className="secondary" disabled={pending} onClick={() => void Backend.QuitApplication().catch(() => setError('暂时无法退出，请使用应用退出菜单'))}>退出 LinkSend</button>}
    {status?.notes && <details className="update-notes" open><summary>更新说明</summary><div>{status.notes}</div></details>}
  </section>;
}
