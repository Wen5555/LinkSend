import { createContext, useContext } from 'react';
import type { DiagnosticTask } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { SettingsCategory } from '../pages/SettingsPage';
import { connectionMethodLabel } from '../connection';

export const DiagnosticNavigation = createContext<{ openSettings: (category: SettingsCategory) => void; openDevices: () => void } | undefined>(undefined);
const stages: Record<string, string> = { discovery: '附近发现', pairing: '配对授权', presence: '设备在线', control: '连接协调', signaling: '协调服务', candidates: '地址交换', ice: '直连探测', quic: '加密连接', authentication: '身份验证', compatibility: '版本兼容', confirmation: '接收确认', source: '发送源', storage: '接收存储', commit: '文件提交', verification: '完整性校验', transfer: '文件传输', recovery: '任务恢复', paused: '已暂停', completed: '已完成', unknown: '未知' };

export function TaskDiagnostics({ task }: { task: { state: string; diagnostic?: DiagnosticTask | null; error_code?: string } }) {
  const navigation = useContext(DiagnosticNavigation);
  const diagnostic = task.diagnostic;
  if (!diagnostic) return task.error_code ? <p className="field-help">任务遇到问题，请在诊断与关于中检查。</p> : null;
  const guidance = diagnostic.guidance;
  if (guidance.code === 'NONE' && !['awaiting_acceptance', 'paused'].includes(task.state)) return null;
  const target: Partial<Record<string, SettingsCategory>> = { open_network: 'network', open_receive: 'receive', open_updates: 'updates', details: 'diagnostics', review_source: 'diagnostics' };
  const category = target[guidance.action];
  return <div className="task-diagnostics">
    <p className="field-help"><strong>{guidance.reason}</strong>{guidance.next_step && <> · {guidance.next_step}</>}</p>
    <details><summary>连接与任务诊断</summary><dl className="settings-diagnostics">
      <div><dt>阶段</dt><dd>{stages[guidance.stage] || '未知'}</dd></div>
      <div><dt>错误码</dt><dd>{guidance.code === 'NONE' ? '暂无' : guidance.code}</dd></div>
      <div><dt>连接方式</dt><dd>{connectionMethodLabel(diagnostic.method)}</dd></div>
      <div><dt>应用版本</dt><dd>{diagnostic.product_version || '未知'}</dd></div>
      <div><dt>最近一次尝试</dt><dd>{diagnostic.last_attempt ? new Date(diagnostic.last_attempt).toLocaleString() : '未知'}</dd></div>
    </dl>
      {guidance.action === 'open_devices' && navigation && <button className="ghost" onClick={navigation.openDevices}>打开设备页</button>}
      {category && navigation && <button className="ghost" onClick={() => navigation.openSettings(category)}>{category === 'network' ? '打开网络设置' : category === 'receive' ? '打开接收设置' : category === 'updates' ? '检查软件更新' : '打开诊断与导出'}</button>}
    </details>
  </div>;
}
