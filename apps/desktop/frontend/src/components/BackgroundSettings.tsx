import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { BackgroundOptions, BackgroundStatus } from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/models';
import type { CommandRunner } from '../hooks/useDesktop';

const permissions: Record<string, string> = { granted: '已授权', denied: '系统已拒绝', not_authorized: '尚未授权', unknown: '由系统通知设置决定', unavailable: '系统通知暂不可用' };

export function BackgroundSettings({ status, run, op, available }: { status?: BackgroundStatus; run: CommandRunner; op: string; available: boolean }) {
  const disabled = !available || !!op || !status;
  const update = (patch: Partial<BackgroundOptions>) => {
    if (!status) return;
    void run('background-options', async () => {
      await Backend.SetBackgroundOptions({ ...status.options, ...patch });
      if (patch.notifications) await Backend.RequestNotificationPermission();
    }, '后台设置已保存');
  };
  return <section className="surface background-settings"><span className="section-kicker">后台运行</span><h2>关闭窗口与通知</h2>
    <label className="field-label">关闭窗口时<select disabled={disabled} value={status?.options.close_mode ?? ''} onChange={event => update({ close_mode: event.target.value })}><option value="">首次关闭时询问</option><option value="exit">退出应用并停止接收</option><option value="background" disabled={!status?.tray_available}>留在后台继续接收</option></select></label>
    <label className="remember-choice"><input type="checkbox" checked={status?.options.notifications ?? false} disabled={disabled} onChange={event => update({ notifications: event.target.checked })} /><span><strong>接收请求和任务结束时通知我</strong><small>{status?.options.notifications ? permissions[status.notification_permission] || '检查系统通知设置' : '关闭通知不影响接收；请求仍显示在应用中'}</small></span></label>
    <label className="remember-choice"><input type="checkbox" checked={status?.options.prevent_sleep ?? false} disabled={disabled} onChange={event => update({ prevent_sleep: event.target.checked })} /><span><strong>传输正文期间阻止自动睡眠</strong><small>{status?.sleep_inhibited ? '正在保持系统唤醒' : '暂停或结束时释放，不阻止手动睡眠'}</small></span></label>
    <label className="remember-choice"><input type="checkbox" checked={status?.autostart ?? false} disabled={disabled} onChange={event => void run('autostart', () => Backend.ConfigureAutostart(event.target.checked), '已更新本用户的登录启动设置')} /><span><strong>登录系统时启动 LinkSend</strong><small>默认关闭；只有选择后台接收后才会隐藏启动窗口</small></span></label>
    {!status?.tray_available && <p className="field-help">托盘暂不可用，请保留窗口以继续接收。</p>}
    {status?.error && <p className="task-error" role="status">{status.error}</p>}
    <button className="secondary" disabled={!available || !!op} onClick={() => void Backend.QuitApplication()}>退出应用</button>
  </section>;
}
