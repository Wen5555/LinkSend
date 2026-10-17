import { useEffect, useRef, useState } from 'react';

export const validFileSendLimit = (rate: number) => rate === 0 || Number.isSafeInteger(rate) && rate >= 1024 && rate <= 1073741824;
const initialUnit = (rate: number) => rate >= 1048576 && rate % 1048576 === 0 ? 1048576 : 1024;

export function FileSendLimit({ rate, disabled, change }: { rate: number; disabled: boolean; change: (bytes: number) => void }) {
  const [unit, setUnit] = useState(initialUnit(rate));
  const [amount, setAmount] = useState(String(rate > 0 ? rate / initialUnit(rate) : 1));
  const published = useRef(rate);
  useEffect(() => {
    if (rate === published.current) return;
    published.current = rate;
    const nextUnit = initialUnit(rate);
    setUnit(nextUnit); setAmount(String(rate > 0 ? rate / nextUnit : 1));
  }, [rate]);
  const publish = (next: number) => { published.current = next; change(next); };
  const updateAmount = (text: string) => {
    setAmount(text);
    const bytes = Number(text) * unit;
    publish(text.trim() && bytes > 0 && validFileSendLimit(bytes) ? bytes : -1);
  };
  return <div className="file-send-limit">
    <label className="toggle-row"><span><strong>限制文件发送正文速率</strong><small>关闭时不限速</small></span><input type="checkbox" aria-label="限制文件发送正文速率" checked={rate !== 0} disabled={disabled} onChange={event => { if (event.target.checked) { setAmount('1'); publish(unit); } else publish(0); }} /></label>
    {rate !== 0 && <div className="picker-row"><label className="field-label">正文速率上限<input aria-label="正文速率上限" type="number" min={1024 / unit} max={1073741824 / unit} step="any" value={amount} disabled={disabled} onChange={event => updateAmount(event.target.value)} /></label><label className="field-label">单位<select aria-label="正文速率单位" value={unit} disabled={disabled} onChange={event => { const next = Number(event.target.value); setUnit(next); setAmount(String(rate > 0 ? rate / next : 1)); if (rate <= 0) publish(next); }}><option value={1024}>KiB/s</option><option value={1048576}>MiB/s</option></select></label></div>}
    {!validFileSendLimit(rate) && <p className="queue-notice" role="alert">请输入 1 KiB/s 至 1024 MiB/s 范围内的有效数值。</p>}
    <p className="field-help">保存后用于本进程当前和后续文件发送，恢复与应用层重传也计入。接收与自动剪贴板不使用此预算；这是正文速率上限，网络协议开销另计。</p>
    <p className="field-help">暂停调度只停止下一项。要暂停当前文件，请在进行中选择暂停传输。</p>
  </div>;
}
