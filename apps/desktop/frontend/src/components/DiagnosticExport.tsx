import { useEffect, useRef, useState } from 'react';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';

export function DiagnosticExport({ available }: { available: boolean }) {
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const pending = useRef(false);
  const mounted = useRef(false);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  const save = async () => {
    if (pending.current || !available) return;
    pending.current = true; setBusy(true); setMessage('');
    try {
      const state = await Backend.ExportDiagnostics();
      if (mounted.current) setMessage(state === 'saved' ? '诊断已保存到所选位置，不会自动上传。' : '已取消导出。');
    } catch {
      if (mounted.current) setMessage('无法导出诊断，请选择可写的新 JSON 文件名后重试。');
    } finally {
      pending.current = false;
      if (mounted.current) setBusy(false);
    }
  };
  return <div className="diagnostic-export"><p className="field-help">本地报告包含版本、连接阶段及最近任务的安全摘要，使用临时任务编号。不会包含凭据、分享码、完整文件路径或文件正文。</p>
    <button className="secondary" disabled={!available || busy} onClick={() => void save()}>{busy ? '正在导出…' : '导出本地诊断'}</button>
    {message && <p className="field-help" role="status">{message}</p>}
  </div>;
}
