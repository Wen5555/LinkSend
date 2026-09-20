// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { DeviceInfo, QueueItem, TaskSnapshot, WorkspaceSnapshot } from '../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from './hooks/useDesktop';
import { createQueryClient, EnqueueIdentity } from './workspace-cache';
import { TransferPage } from './pages/TransferPage';
import { Queue } from './components/Queue';
import { TaskList } from './components/Tasks';

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const backend = vi.hoisted(() => ({ Enqueue: vi.fn(), SaveDraft: vi.fn(), PreviewDraft: vi.fn(), ReorderQueue: vi.fn(), CancelTask: vi.fn() }));
vi.mock('../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => backend);

const run: CommandRunner = async (_key, action, _message, onError) => {
  try { await action(); return true; }
  catch (cause) { await onError?.(cause); return false; }
};
const device: DeviceInfo = { id: 'peer', name: '目标设备', group_id: '', public_key_hex: '', admin: false, online: true, trusted: true, always_accept: false, nearby: false, blocked: false, relationship: 'group_paired', service_state: 'membership_synced', lan_control_state: 'unavailable', connection_state: 'not_connected', profile: { peer_id: 'peer', alias: '', my_device: false, pinned: false, position: 0, receive_directory: '', conflict_policy: '', last_used_at: '', revision: 1 } };
const queueItem = (id: string): QueueItem => ({ id, state: 'queued', request_id: id, peer_id: 'peer', source_summary: `source-${id}`, position: 0, task_id: '', expires_at: '', revision: 1, created_at: '', updated_at: '', last_error: '', wait_for_peer: true });
const task = (id: string): TaskSnapshot => ({ id, task_id: id, attempt_id: '', direction: 'send', peer_id: 'peer', state: 'transferring', phase: 'transferring', processed_bytes: 0, total_bytes: 1, started_at: '', updated_at: '', source_summary: id, sent_bytes: 0, received_bytes: 0, retransmitted_bytes: 0, verified_bytes: 0, original_total: 0, selected_files: 1, selected_entries: 1, skipped_files: 0, skipped_entries: 0, skipped_bytes: 0, committed_bytes: 0, committed_files: 0, bilateral_confirmed: false, relay: false, stun_requests_sent: 0, stun_responses_received: 0, signaling_bytes_sent: 0, signaling_bytes_received: 0, connect_timings: { peer_lookup_ms: 0, signaling_connect_ms: 0, endpoint_setup_ms: 0, peer_response_ms: 0, ice_ms: 0, quic_handshake_ms: 0, total_connect_ms: 0 }, can_cancel: true, can_retry: false, can_pause: false, can_resume: false, revision: 1, history_persisted: true, restart_recovery_supported: true, byte_resume_supported: true });
const roots: Root[] = [];
function mount(render: (root: Root) => void) { const container = document.createElement('div'); document.body.append(container); const root = createRoot(container); roots.push(root); act(() => render(root)); return { container, root }; }
async function settle() { await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); }); }

afterEach(() => { for (const root of roots.splice(0)) act(() => root.unmount()); document.body.replaceChildren(); vi.clearAllMocks(); });

describe('有界传输视图', () => {
  it('shows at most ten draft paths but enqueues the complete stable path set', async () => {
    const paths = Array.from({ length: 25 }, (_, index) => `C:\\draft\\file-${index}.txt`);
    const workspace: WorkspaceSnapshot = { epoch: 'test', revision: 1, draft: { id: 'draft', peer_id: 'peer', paths, revision: 1, updated_at: '' }, queue: [], tasks: [], queue_paused: false, persistence_available: true };
    backend.PreviewDraft.mockResolvedValue({ revision: 1, files: 25, directories: 0, bytes: 0, complete: true, problem: '' });
    backend.Enqueue.mockResolvedValue(undefined); backend.SaveDraft.mockResolvedValue(undefined);
    const { container } = mount(root => root.render(<QueryClientProvider client={createQueryClient()}><TransferPage workspace={workspace} devices={[device]} run={run} op="" controlRun={run} controlOp="" available enqueueIdentity={new EnqueueIdentity(() => 'request')} /></QueryClientProvider>));
    expect(container.querySelectorAll('[data-draft-path]')).toHaveLength(10);
    act(() => container.querySelector<HTMLButtonElement>('button.send-button')?.click()); await settle();
    expect(backend.Enqueue).toHaveBeenCalledWith(expect.objectContaining({ paths }));
  });

  it('reorders from a later queue page using all pending IDs and clamps after entries disappear', async () => {
    const items = Array.from({ length: 11 }, (_, index) => queueItem(`queue-${index}`));
    backend.ReorderQueue.mockResolvedValue(undefined);
    const { container, root } = mount(root => root.render(<Queue items={items} devices={[device]} paused={false} run={run} op="" available />));
    act(() => container.querySelector<HTMLButtonElement>('button[aria-label="队列下一页"]')?.click());
    expect(container.querySelectorAll('[data-queue-id]')).toHaveLength(1);
    act(() => container.querySelector<HTMLButtonElement>('button[aria-label="上移 source-queue-10，队列项 queue-10"]')?.click()); await settle();
    expect(backend.ReorderQueue).toHaveBeenCalledWith(expect.arrayContaining(items.map(item => item.id)));
    expect((backend.ReorderQueue.mock.calls[0][0] as string[])).toHaveLength(11);
    act(() => root.render(<Queue items={items.slice(0, 10)} devices={[device]} paused={false} run={run} op="" available />)); await settle();
    expect(container.querySelectorAll('[data-queue-id]')).toHaveLength(10);
    expect(container.querySelector('[data-queue-id="queue-0"]')).toBeTruthy();
    expect(container.querySelector('button[aria-label="队列下一页"]')).toBeNull();
  });

  it('keeps task rows bounded, binds a later-page action to its task ID, and clamps after completion', async () => {
    const tasks = Array.from({ length: 11 }, (_, index) => task(`task-${index}`));
    backend.CancelTask.mockResolvedValue(undefined);
    const { container, root } = mount(root => root.render(<TaskList tasks={tasks} devices={[device]} run={run} op="" />));
    expect(container.querySelectorAll('[data-task-id]')).toHaveLength(10);
    act(() => container.querySelector<HTMLButtonElement>('button[aria-label="任务下一页"]')?.click());
    expect(container.querySelectorAll('[data-task-id]')).toHaveLength(1);
    act(() => container.querySelector<HTMLButtonElement>('button[aria-label="取消传输 task-0"]')?.click()); await settle();
    expect(backend.CancelTask).toHaveBeenCalledWith('task-0');
    act(() => root.render(<TaskList tasks={tasks.slice(1)} devices={[device]} run={run} op="" />)); await settle();
    expect(container.querySelectorAll('[data-task-id]')).toHaveLength(10);
    expect(container.querySelector('[data-task-id="task-10"]')).toBeTruthy();
    expect(container.querySelector('button[aria-label="任务下一页"]')).toBeNull();
  });
});
