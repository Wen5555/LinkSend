import { useEffect, useState } from 'react';
import type { DraftPreview, SendDraft } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { formatBytes } from '../presentation';

type Props = { draft: SendDraft; preview?: DraftPreview; disabled: boolean; save: (patch: Partial<SendDraft>) => void };

export function SourceSelectionPanel({ draft, preview, disabled, save }: Props) {
  const policy = draft.source_selection;
  const savedRules = (policy?.exclude ?? []).join('\n');
  const [rules, setRules] = useState(savedRules);
  useEffect(() => { setRules(savedRules); }, [savedRules]);
  const validPreview = preview?.revision === draft.revision ? preview : undefined;
  const preset = policy?.preset ?? 'all';
  return <section className="source-selection" aria-label="目录发送范围">
    <label className="field-label">目录预设<select aria-label="目录预设" value={preset} disabled={disabled} onChange={event => { const selectedPreset = event.target.value; save({ source_selection: selectedPreset === 'all' && !policy?.exclude?.length ? undefined : { version: 1, preset: selectedPreset, exclude: policy?.exclude ?? [] } }); }}>
      <option value="all">不使用预设</option><option value="source">源码目录</option>
    </select></label>
    <p className="field-help">{policy ? '按已保存规则发送。' : '默认发送完整内容。'}源码预设排除 .git、node_modules、dist、build 等依赖与构建目录；不影响源文件。</p>
    <details><summary>自定义排除规则</summary>
      <label className="field-label">每行一条<textarea aria-label="自定义排除规则" value={rules} rows={3} disabled={disabled} onChange={event => setRules(event.target.value)} maxLength={4200} /></label>
      <p className="field-help">最多 32 条，基于每个所选目录的相对路径，大小写敏感。支持 *、? 和独立路径段 **，目录规则以 / 结尾。不支持取反或完整 gitignore 语义。</p>
      <button className="secondary" disabled={disabled} onClick={() => { const exclude = rules.split(/\r?\n/).map(rule => rule.trim()).filter(Boolean); save({ source_selection: preset === 'all' && exclude.length === 0 ? undefined : { version: 1, preset, exclude } }); }}>应用排除规则</button>
    </details>
    {policy && <button className="ghost" disabled={disabled} onClick={() => save({ source_selection: undefined })}>恢复完整发送</button>}
    {validPreview && <>
      <p className="hint">{validPreview.complete ? '包含' : '已统计包含'} {validPreview.files} 个文件 · {validPreview.directories} 个目录 · {formatBytes(validPreview.bytes)}；排除 {validPreview.excluded_files ?? 0} 个文件 · {validPreview.excluded_directories ?? 0} 个目录 · {formatBytes(validPreview.excluded_bytes ?? 0)}</p>
      {validPreview.problem && <p className="field-help" role="status">{validPreview.problem}</p>}
      {!!validPreview.entries?.length && <details><summary>查看包含与排除的条目{validPreview.entries_truncated ? '（前 200 条）' : ''}</summary><ul className="source-selection-entries">{validPreview.entries.map((entry, index) => <li key={index}><span>{entry.excluded ? '排除' : '包含'}</span> {entry.path}{entry.directory ? '/' : ''}</li>)}</ul></details>}
    </>}
  </section>;
}
