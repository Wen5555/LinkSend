// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { SendDraft, TaskSnapshot, TemporaryShareSnapshot } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { TemporarySharePanel, temporaryShareError } from './TemporarySharePanel';

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
const backend = vi.hoisted(() => ({ TemporaryShares: vi.fn(), CreateTemporaryShare: vi.fn(), ClaimTemporaryShare: vi.fn(), RefreshTemporaryShare: vi.fn(), ApproveTemporaryShare: vi.fn(), DenyTemporaryShare: vi.fn(), StartTemporaryShare: vi.fn(), ConfirmTemporaryShareSave: vi.fn(), CancelTemporaryShare: vi.fn(), RevokeTemporaryShare: vi.fn(), ResumeTemporaryShare: vi.fn(), RetryTemporaryShareAction: vi.fn(), RetryTemporaryShareClaim: vi.fn(), CopyTemporaryShareCode: vi.fn(), PickDirectory: vi.fn() }));
vi.mock('../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => backend);
const roots: Root[] = [];
const draft: SendDraft = { id: 'main', paths: ['C:\\fixture\\所选目录'], peer_id: '', revision: 9, updated_at: '', source_selection: { version: 1, preset: 'source', exclude: ['secret/'] } };
const code = 'ABCDEFGHIJKLMNOPQRSTUVWX234567AB';
const grouped = code.match(/.{1,4}/g)!.join('-');
const share = (patch: Partial<TemporaryShareSnapshot> = {}) => ({ id: 'local-owner', grant_id: 'server-grant', role: 'sender', state: 'offered', grant_state: 'offered', revision: 1, ...patch } as TemporaryShareSnapshot);
const pending = () => share({ state: 'pending', grant_state: 'pending' as TemporaryShareSnapshot['grant_state'], peer_id: 'a'.repeat(64), claim_id: '1'.repeat(32), receiver_public_key: 'A'.repeat(43), generation: 1, revision: 2 });
const button = (container: HTMLElement, text: string) => Array.from(container.querySelectorAll('button')).find(value => value.textContent === text)!;
const settle = () => new Promise(resolve => setTimeout(resolve, 10));
async function click(container: HTMLElement, text: string) { await act(async () => { button(container, text).click(); await settle(); }); }
async function input(container: HTMLElement, text: string) { await act(async () => { const field = container.querySelector<HTMLInputElement>('input[aria-label="临时分享码"]')!; Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(field, text); field.dispatchEvent(new Event('input', { bubbles: true })); await settle(); }); }
async function mount(initial: TemporaryShareSnapshot[] = [], opened = true) {
  backend.TemporaryShares.mockResolvedValue(initial);
  const container = document.createElement('div'); document.body.append(container);
  const root = createRoot(container); roots.push(root);
  const refresh = vi.fn(async () => {}), onBusyChange = vi.fn();
  const render = (available = true, nextDraft: SendDraft = draft, tasks: TaskSnapshot[] = []) => root.render(<TemporarySharePanel draft={nextDraft} tasks={tasks} available={available} refresh={refresh} onBusyChange={onBusyChange} op="" />);
  await act(async () => { render(); await settle(); });
  if (opened) await click(container, '打开临时分享');
  return { container, root, render, refresh, onBusyChange };
}
afterEach(() => { for (const root of roots.splice(0)) act(() => root.unmount()); document.body.replaceChildren(); vi.restoreAllMocks(); vi.resetAllMocks(); });

describe('一次性临时分享', () => {
  it.each([
    ['offered', '等待领取'],
    ['pending', '等待发送方确认'],
    ['approved', '身份已确认，等待继续'],
    ['bound', '已绑定原任务'],
  ])('shows the actual %s grant stage for a ready local owner', async (grantState, expected) => {
    const { container } = await mount([share({ state: 'ready', grant_state: grantState as TemporaryShareSnapshot['grant_state'] })]);
    expect(container.querySelector('.temporary-share-heading strong')!.textContent).toBe(`发送 · ${expected}`);
    expect(container.textContent).not.toContain('等待核对完成结果');
    expect(container.querySelector('.temporary-share-heading strong')!.textContent).not.toContain('已完成');
  });

  it.each(['ready', 'needs_attention'])('does not treat a completed remote grant as a locally verified %s result', async state => {
    const { container } = await mount([share({ state, grant_state: 'completed' as TemporaryShareSnapshot['grant_state'] })]);
    expect(container.querySelector('.temporary-share-heading strong')!.textContent).toBe('发送 · 分享已结束，本机结果待核对');
    expect(container.querySelector('.temporary-share-heading strong')!.textContent).not.toContain('已完成');
  });

  it('shows completion reconciliation only for the persisted complete intent', async () => {
    const { container } = await mount([share({ state: 'needs_attention', grant_state: 'bound' as TemporaryShareSnapshot['grant_state'], pending_action: 'complete' as TemporaryShareSnapshot['pending_action'] })]);
    expect(container.querySelector('.temporary-share-heading strong')!.textContent).toBe('发送 · 等待核对完成结果');
    expect(button(container, '核对并重试原操作')).toBeDefined();
    expect(backend.RetryTemporaryShareAction).not.toHaveBeenCalled();
  });

  it('keeps a remote completed grant unverified even with a pending complete intent', async () => {
    const { container } = await mount([share({ state: 'ready', grant_state: 'completed' as TemporaryShareSnapshot['grant_state'], pending_action: 'complete' as TemporaryShareSnapshot['pending_action'] })]);
    expect(container.querySelector('.temporary-share-heading strong')!.textContent).toBe('发送 · 分享已结束，本机结果待核对');
  });

  it('shows completion only from the local completed state', async () => {
    const { container } = await mount([share({ state: 'completed', grant_state: 'completed' as TemporaryShareSnapshot['grant_state'] })]);
    expect(container.querySelector('.temporary-share-heading strong')!.textContent).toBe('发送 · 已完成');
  });

  it('mounts collapsed and never starts, confirms or retries from a local read', async () => {
    const { container } = await mount([share({ state: 'needs_attention', grant_state: 'bound' as TemporaryShareSnapshot['grant_state'] })], false);
    expect(backend.TemporaryShares).not.toHaveBeenCalled();
    await click(container, '打开临时分享');
    expect(container.textContent).toContain('恢复原分享任务');
    for (const fn of [backend.StartTemporaryShare, backend.ResumeTemporaryShare, backend.ApproveTemporaryShare, backend.ConfirmTemporaryShareSave, backend.RetryTemporaryShareAction]) expect(fn).not.toHaveBeenCalled();
    await click(container, '恢复原分享任务');
    expect(backend.ResumeTemporaryShare).toHaveBeenCalledExactlyOnceWith('local-owner');
  });

  it('creates only from the saved main revision, retaining paths and selection in Go', async () => {
    backend.CreateTemporaryShare.mockResolvedValue({ share: share(), code });
    const setItem = vi.spyOn(Storage.prototype, 'setItem');
    const { container } = await mount();
    await click(container, '创建分享码');
    expect(backend.CreateTemporaryShare).toHaveBeenCalledExactlyOnceWith(9);
    expect(container.textContent).toContain(grouped);
    expect(setItem).not.toHaveBeenCalled();
    await click(container, '复制分享码');
    expect(backend.CopyTemporaryShareCode).toHaveBeenCalledExactlyOnceWith('local-owner', code);
    await click(container, '隐藏分享码');
    expect(container.textContent).not.toContain(grouped);
  });

  it('clears secrets on close and never reconstructs a code from owner snapshots', async () => {
    backend.CreateTemporaryShare.mockResolvedValue({ share: share(), code });
    const { container } = await mount([share()]);
    await click(container, '创建分享码');
    expect(container.textContent).toContain(grouped);
    await click(container, '关闭临时分享');
    await click(container, '打开临时分享');
    expect(container.textContent).not.toContain(grouped);
    expect(button(container, '复制分享码')).toBeUndefined();
    expect(container.textContent).toContain('撤销分享');
  });

  it('does not call a confirmed mutation failed when only workspace refresh fails', async () => {
    backend.CreateTemporaryShare.mockResolvedValue({ share: share(), code });
    const { container, refresh } = await mount();
    refresh.mockRejectedValueOnce(new Error('PRIVATE_REFRESH_DETAIL'));
    await click(container, '创建分享码');
    expect(container.textContent).toContain('后端已确认本次操作');
    expect(container.textContent).toContain('无需重复提交');
    expect(container.textContent).not.toContain('PRIVATE_REFRESH_DETAIL');
    expect(backend.CreateTemporaryShare).toHaveBeenCalledOnce();
  });

  it('does not reveal a late create response after the panel was closed', async () => {
    let finish!: (value: unknown) => void;
    backend.CreateTemporaryShare.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    const { container } = await mount();
    await click(container, '创建分享码');
    await click(container, '关闭临时分享');
    await act(async () => { finish({ share: share(), code }); await settle(); });
    await click(container, '打开临时分享');
    expect(container.textContent).not.toContain(grouped);
    expect(backend.CreateTemporaryShare).toHaveBeenCalledOnce();
  });

  it('clears claim input before submitting and sanitizes errors containing private text', async () => {
    backend.ClaimTemporaryShare.mockRejectedValue(new Error(`SHARE_UNSUPPORTED: ${code} private-body`));
    const { container } = await mount();
    await input(container, code);
    await click(container, '领取分享');
    expect(backend.ClaimTemporaryShare).toHaveBeenCalledExactlyOnceWith(code);
    expect(container.querySelector<HTMLInputElement>('input')!.value).toBe('');
    expect(container.textContent).toContain('当前服务器尚未启用临时分享');
    expect(container.textContent).not.toContain(code);
    expect(container.textContent).not.toContain('private-body');
    expect(temporaryShareError(`unknown ${code}`)).not.toContain(code);
  });

  it('maps the actual revision conflict inside Wails RuntimeError without exposing its detail', () => {
    const wrapped = new Error(`RuntimeError: METADATA_REVISION_CONFLICT: private draft ${code}`);
    expect(temporaryShareError(wrapped)).toBe('上方草稿已变化，请刷新后使用当前选择重新创建。');
    expect(temporaryShareError(wrapped)).not.toContain(code);
    expect(temporaryShareError(wrapped)).not.toContain('private draft');
  });

  it('captures exact claim/key/generation and starts only after the explicit confirmation', async () => {
    const original = pending();
    backend.ApproveTemporaryShare.mockResolvedValue(share({ ...original, state: 'approved', grant_state: 'approved' as TemporaryShareSnapshot['grant_state'], revision: 3 }));
    const { container } = await mount([original]);
    await click(container, '核对接收人');
    expect(backend.ApproveTemporaryShare).not.toHaveBeenCalled();
    expect(backend.StartTemporaryShare).not.toHaveBeenCalled();
    expect(container.querySelector('[aria-label="核对临时接收身份"]')!.textContent).toContain(original.peer_id);
    await click(container, '确认接收人并发送');
    expect(backend.ApproveTemporaryShare).toHaveBeenCalledExactlyOnceWith(original.id, original.claim_id, original.receiver_public_key, original.generation);
    expect(backend.StartTemporaryShare).toHaveBeenCalledExactlyOnceWith(original.id);
  });

  it('disables a stale approval after refreshing a different claimant', async () => {
    const changed = { ...pending(), claim_id: '2'.repeat(32), generation: 2, revision: 3, peer_id: 'b'.repeat(64) };
    backend.RefreshTemporaryShare.mockResolvedValue(changed);
    const { container } = await mount([pending()]);
    await click(container, '核对接收人');
    await click(container, '刷新分享状态');
    expect(button(container, '确认接收人并发送').disabled).toBe(true);
    await click(container, '确认接收人并发送');
    expect(backend.ApproveTemporaryShare).not.toHaveBeenCalled();
    expect(container.textContent).toContain('领取对象或状态已变化');
  });

  it('requires an explicit metadata-preview start after the receiver sees approval', async () => {
    const { container } = await mount([share({ role: 'receiver', state: 'approved', grant_state: 'approved' as TemporaryShareSnapshot['grant_state'] })]);
    expect(backend.StartTemporaryShare).not.toHaveBeenCalled();
    await click(container, '重读本机状态');
    expect(backend.StartTemporaryShare).not.toHaveBeenCalled();
    await click(container, '读取加密文件清单');
    expect(backend.StartTemporaryShare).toHaveBeenCalledExactlyOnceWith('local-owner');
    expect(backend.ConfirmTemporaryShareSave).not.toHaveBeenCalled();
  });

  it('does not bind when a directory picker is cancelled; explicit save binds only that local owner', async () => {
    const awaiting = share({ role: 'receiver', state: 'awaiting_save', grant_state: 'approved' as TemporaryShareSnapshot['grant_state'] });
    backend.PickDirectory.mockResolvedValueOnce('').mockResolvedValueOnce('C:\\chosen');
    backend.ConfirmTemporaryShareSave.mockResolvedValue({ ...awaiting, state: 'active', revision: 2 });
    const { container } = await mount([awaiting]);
    expect(button(container, '确认保存并接收').disabled).toBe(true);
    await click(container, '选择保存文件夹');
    expect(button(container, '确认保存并接收').disabled).toBe(true);
    expect(backend.ConfirmTemporaryShareSave).not.toHaveBeenCalled();
    await click(container, '选择保存文件夹');
    await click(container, '确认保存并接收');
    expect(backend.ConfirmTemporaryShareSave).toHaveBeenCalledExactlyOnceWith('local-owner', 'C:\\chosen', 'keep_both');
  });

  it('keeps receiver cancellation distinct from sender revocation', async () => {
    backend.CancelTemporaryShare.mockResolvedValue(share({ role: 'receiver', state: 'stopped', revision: 2 }));
    const { container } = await mount([share({ role: 'receiver', state: 'pending' })]);
    await click(container, '取消本机接收');
    expect(backend.CancelTemporaryShare).toHaveBeenCalledExactlyOnceWith('local-owner');
    expect(backend.RevokeTemporaryShare).not.toHaveBeenCalled();
  });

  it('coalesces fast duplicate clicks before the button becomes disabled', async () => {
    let finish!: (value: unknown) => void;
    backend.CreateTemporaryShare.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    const { container } = await mount();
    await act(async () => { button(container, '创建分享码').click(); button(container, '创建分享码').click(); await settle(); });
    expect(backend.CreateTemporaryShare).toHaveBeenCalledOnce();
    await act(async () => { finish({ share: share(), code }); await settle(); });
  });

  it('bounds local shares and manifest DOM while preserving pagination', async () => {
    const entries = Array.from({ length: 12 }, (_, id) => ({ id, path: `目录/文件${id}`, type: 'file', size: 1, modified: 0 }));
    const values = Array.from({ length: 12 }, (_, index) => share({ id: `local-${index}`, manifest: index === 0 ? { version: 1, transfer_id: 'a'.repeat(32), chunk_size: 65536, files: entries } : undefined }));
    const { container } = await mount(values);
    expect(container.querySelectorAll('article[data-share-id]')).toHaveLength(5);
    expect(container.querySelectorAll('.temporary-share-manifest li')).toHaveLength(10);
    await click(container, '清单下一页');
    expect(container.querySelectorAll('.temporary-share-manifest li')).toHaveLength(2);
    await click(container, '分享下一页');
    expect(container.querySelectorAll('article[data-share-id]')).toHaveLength(5);
    await click(container, '分享下一页');
    expect(container.querySelectorAll('article[data-share-id]')).toHaveLength(2);
  });

  it('does not read or mutate when the backend is unavailable and removes an existing secret', async () => {
    backend.CreateTemporaryShare.mockResolvedValue({ share: share(), code });
    const { container, render } = await mount();
    await click(container, '创建分享码');
    await act(async () => { render(false); await settle(); });
    expect(container.textContent).not.toContain(grouped);
    expect(container.textContent).toContain('桌面后端尚未就绪');
    await act(async () => { render(true); await settle(); });
    expect(container.textContent).not.toContain(grouped);
  });

  it('retries only the persisted claim with re-entered input instead of making a second claim', async () => {
    backend.RetryTemporaryShareClaim.mockResolvedValue(share({ role: 'receiver', state: 'pending', revision: 2 }));
    const { container } = await mount([share({ role: 'receiver', state: 'claiming', pending_action: 'claim' as TemporaryShareSnapshot['pending_action'] })]);
    await input(container, code);
    expect(button(container, '领取分享').disabled).toBe(true);
    await click(container, '用上方分享码重试领取');
    expect(backend.RetryTemporaryShareClaim).toHaveBeenCalledExactlyOnceWith('local-owner', code);
    expect(backend.ClaimTemporaryShare).not.toHaveBeenCalled();
    expect(container.querySelector<HTMLInputElement>('input')!.value).toBe('');
  });
});
