// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { BatchSnapshot, DeviceInfo, SendDraft, WorkspaceSnapshot } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { BatchTargets } from './BatchTargets';
import { TransferPage } from '../pages/TransferPage';
import { Queue } from './Queue';
import { createQueryClient, EnqueueIdentity } from '../workspace-cache';
import type { CommandRunner } from '../hooks/useDesktop';

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
const backend = vi.hoisted(() => ({ PreviewDraft: vi.fn(async () => ({ revision: 7, files: 1, directories: 0, bytes: 5, complete: true })), Enqueue: vi.fn(), EnqueueBatch: vi.fn(), SaveDraft: vi.fn(), GetBatch: vi.fn(), CancelBatch: vi.fn() }));
vi.mock('../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => backend);
const roots: Root[] = [];
afterEach(() => { for (const root of roots.splice(0)) act(() => root.unmount()); document.body.replaceChildren(); vi.clearAllMocks(); });
const peer = (id: string, patch: Partial<DeviceInfo> = {}): DeviceInfo => ({ id, name: id, group_id: '', public_key_hex: '', admin: false, online: true, trusted: true, always_accept: false, nearby: false, blocked: false, relationship: 'group_paired', service_state: 'membership_synced', lan_control_state: 'unavailable', connection_state: 'not_connected', profile: { peer_id: id, alias: '', my_device: false, pinned: false, position: 0, receive_directory: '', conflict_policy: '', last_used_at: '', revision: 1 }, ...patch });
const draft: SendDraft = { id: 'main', peer_id: 'a', peer_ids: ['a','b','c'], paths: ['C:\\fixture\\file.txt'], revision: 7, updated_at: '' };
const batch: BatchSnapshot = { id: 'batch', request_id: 'request', draft_id: 'main', draft_revision: 7, revision: 1, created_at: '', cancelled: false, state: 'waiting', total: 3, completed: 1, waiting: 2, running: 0, needs_attention: 0, stopped: 0 };
const run: CommandRunner = async (_key, action, _message, onError) => { try { await action(); return true; } catch (error) { await onError?.(error); return false; } };
function mount(render: (root: Root) => void) { const container = document.createElement('div'); document.body.append(container); const root = createRoot(container); roots.push(root); act(() => render(root)); return container; }
async function settle() { await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); }); }

describe('Go批次分发入口', () => {
  it('saves the complete target list and permits removing a revoked selected target', () => {
    const save = vi.fn();
    const container = mount(root => root.render(<BatchTargets draft={draft} devices={[peer('a'),peer('b',{blocked:true,trusted:false}),peer('c'),peer('nearby',{trusted:false,nearby:true}),peer('self')]} identityID="self" disabled={false} save={save} />));
    expect(container.querySelector('input[aria-label="分发给 nearby"]')).toBeNull();
    expect(container.querySelector('input[aria-label="分发给 self"]')).toBeNull();
    const revoked = container.querySelector<HTMLInputElement>('input[aria-label="分发给 b"]')!;
    expect(revoked.disabled).toBe(false);
    act(() => revoked.click());
    expect(save).toHaveBeenCalledExactlyOnceWith({ peer_ids: ['a','c'], peer_id: 'a' });
  });

  it('bounds choices, paginates and prevents adding a seventeenth target', () => {
    const ids = Array.from({length:16},(_,index)=>`peer-${index}`);
    const container = mount(root => root.render(<BatchTargets draft={{...draft,peer_id:ids[0],peer_ids:ids}} devices={[peer('extra'),...ids.map(id=>peer(id))]} disabled={false} save={vi.fn()} />));
    expect(container.querySelectorAll('input[type="checkbox"]').length).toBe(10);
    expect(container.querySelector<HTMLInputElement>('input[aria-label="分发给 extra"]')?.disabled).toBe(true);
    expect(container.textContent).toContain('目标下一页');
  });

  it('enqueues exactly one Go batch and clears the draft only after success', async () => {
    backend.EnqueueBatch.mockResolvedValue(batch); backend.SaveDraft.mockResolvedValue({...draft,paths:[],revision:8});
    const workspace: WorkspaceSnapshot = {epoch:'fixture',revision:1,draft,queue:[],tasks:[],queue_paused:false,persistence_available:true};
    const container = mount(root=>root.render(<QueryClientProvider client={createQueryClient()}><TransferPage workspace={workspace} devices={['a','b','c'].map(id=>peer(id))} run={run} op="" controlRun={run} controlOp="" available enqueueIdentity={new EnqueueIdentity(()=>'batch-request')} /></QueryClientProvider>));
    act(()=>Array.from(container.querySelectorAll('button')).find(button=>button.textContent==='分发给 3 台设备')!.click());
    await settle();
    expect(backend.EnqueueBatch).toHaveBeenCalledExactlyOnceWith({request_id:'batch-request',draft_id:'main',draft_revision:7,wait_for_peer:false,expires_at:''});
    expect(backend.Enqueue).not.toHaveBeenCalled();
    expect(backend.SaveDraft).toHaveBeenCalledExactlyOnceWith({...draft,paths:[]});
  });

  it('keeps the draft if the Go preflight or transaction fails', async () => {
    backend.EnqueueBatch.mockRejectedValueOnce(new Error('transaction failed'));
    const workspace: WorkspaceSnapshot = {epoch:'fixture',revision:1,draft,queue:[],tasks:[],queue_paused:false,persistence_available:true};
    const container = mount(root=>root.render(<QueryClientProvider client={createQueryClient()}><TransferPage workspace={workspace} devices={['a','b','c'].map(id=>peer(id))} run={run} op="" controlRun={run} controlOp="" available enqueueIdentity={new EnqueueIdentity(()=>'failed-request')} /></QueryClientProvider>));
    act(()=>Array.from(container.querySelectorAll('button')).find(button=>button.textContent==='分发给 3 台设备')!.click()); await settle();
    expect(backend.SaveDraft).not.toHaveBeenCalled(); expect(container.querySelectorAll('[data-draft-path]').length).toBe(1);
  });

  it('cancels a parent through its revision, leaving independent child controls to Go', async () => {
    backend.CancelBatch.mockResolvedValue({...batch,cancelled:true});
    const item = { id:'child',request_id:'request-child',peer_id:'b',source_summary:'file',state:'waiting_peer',position:2,task_id:'',expires_at:'',revision:1,created_at:'',updated_at:'',last_error:'',wait_for_peer:true,batch_id:'batch',batch_position:2 };
    const container = mount(root=>root.render(<Queue items={[item]} batches={[batch]} devices={[peer('b')]} paused={false} run={run} op="" available />));
    act(()=>Array.from(container.querySelectorAll('button')).find(button=>button.textContent==='取消此批次未完成目标')!.click()); await settle();
    expect(backend.CancelBatch).toHaveBeenCalledExactlyOnceWith('batch',1); expect(backend.GetBatch).not.toHaveBeenCalled();
    expect(container.textContent).toContain('1/3 完成');
  });
});
