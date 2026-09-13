import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { QueryClientProvider } from '@tanstack/react-query';
import { createQueryClient } from './workspace-cache';
import { describe, expect, it, vi } from 'vitest';
import type { DeviceInfo, QueueItem, TaskSnapshot, WorkspaceSnapshot } from '../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from './hooks/useDesktop';
import { TransferPage } from './pages/TransferPage';
import { Queue } from './components/Queue';
import { TaskList } from './components/Tasks';
import { LANPairPrompt } from './components/LANPairPrompt';
import { deviceName, movedQueueIDs } from './presentation';
import { EnqueueIdentity } from './workspace-cache';
import { mergePreferences, mergeSavedPreferences } from './pages/SettingsPage';
import { lanPairServerMessage } from './pages/DevicesPage';
import type { DesktopPreferences } from '../bindings/github.com/Wen5555/LinkSend/apps/desktop/models';

vi.mock('../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => ({}));

const run: CommandRunner = async () => true;
const device: DeviceInfo = { id: 'peer', name: '对方远端名称', group_id: '', public_key_hex: '', admin: false, online: true, trusted: true, always_accept: false, nearby: false, blocked: false, relationship: 'group_paired', service_state: 'membership_synced', lan_control_state: 'unavailable', connection_state: 'not_connected',
  profile: { peer_id: 'peer', alias: '我的电脑', my_device: true, pinned: true, position: 1, receive_directory: '', conflict_policy: '', last_used_at: '', revision: 1 } };
const task: TaskSnapshot = {
  id: 'task', task_id: 'task', attempt_id: 'attempt', direction: 'send', peer_id: 'peer', state: 'transferring', phase: 'transferring',
  processed_bytes: 0, started_at: '', updated_at: '', sent_bytes: 0, received_bytes: 0, retransmitted_bytes: 0, verified_bytes: 0,
  original_total: 0, selected_files: 1, selected_entries: 1, skipped_files: 0, skipped_entries: 0, skipped_bytes: 0,
  committed_bytes: 0, committed_files: 0, bilateral_confirmed: false, relay: false, stun_requests_sent: 0, stun_responses_received: 0,
  signaling_bytes_sent: 0, signaling_bytes_received: 0, connect_timings: { peer_lookup_ms: 0, signaling_connect_ms: 0, endpoint_setup_ms: 0, peer_response_ms: 0, ice_ms: 0, quic_handshake_ms: 0, total_connect_ms: 0 },
  can_cancel: true, can_retry: false, can_pause: true, can_resume: false, revision: 1, history_persisted: true, restart_recovery_supported: true, byte_resume_supported: true,
};
const workspace: WorkspaceSnapshot = { epoch: 'test', revision: 1, draft: { id: 'main', peer_id: 'peer', paths: ['C:\\report.txt'], revision: 1, updated_at: '' }, queue: [], tasks: [task], queue_paused: false, persistence_available: true };
const queueItem = (id: string, state: string): QueueItem => ({ id, state, request_id: id, peer_id: 'peer', source_summary: 'report.txt', position: 0, task_id: '', expires_at: '', revision: 1, created_at: '', updated_at: '', last_error: '', wait_for_peer: true });

describe('workspace operation surfaces', () => {
  it('keeps pause and cancel enabled while a different enqueue command is preparing', () => {
    const markup = renderToStaticMarkup(createElement(QueryClientProvider, { client: createQueryClient() }, createElement(TransferPage, { workspace, devices: [device], run, op: 'enqueue', controlRun: run, controlOp: '', available: true, enqueueIdentity: new EnqueueIdentity(() => 'request') })));
    expect(markup).toContain('正在核对内容并加入');
    expect(markup).toContain('进行中');
    expect(markup).not.toContain('保存内容草稿');
    expect(markup).not.toContain('内容类型');
    const controls = renderToStaticMarkup(createElement(TaskList, { tasks: [task], devices: [device], run, op: '' }));
    expect(controls).toMatch(/<button class="secondary">暂停传输<\/button>/);
    expect(controls).toMatch(/<button class="ghost">取消<\/button>/);
  });
  it('requires an explicit waiting choice for an offline destination', () => {
    const markup = renderToStaticMarkup(createElement(QueryClientProvider, { client: createQueryClient() }, createElement(TransferPage, { workspace, devices: [{ ...device, online: false }], run, op: '', controlRun: run, controlOp: '', available: true, enqueueIdentity: new EnqueueIdentity(() => 'request') })));
    expect(markup).toContain('勾选等待后可加入队列');
    expect(markup).toMatch(/<button class="primary full send-button" disabled="">发送文件<\/button>/);
  });
  it('treats a LAN-only peer as reachable and mounts LAN consent outside the devices page', () => {
    const lan = { ...device, online: false, nearby: true };
    const markup = renderToStaticMarkup(createElement(QueryClientProvider, { client: createQueryClient() }, createElement(TransferPage, { workspace, devices: [lan], run, op: '', controlRun: run, controlOp: '', available: true, enqueueIdentity: new EnqueueIdentity(() => 'request') })));
    expect(markup).not.toContain('对方暂时离线');
    expect(markup).toMatch(/<button class="primary full send-button">发送文件<\/button>/);
    const client = createQueryClient();
    client.setQueryData(['lan-pair-pending'], [{ request_id: 'request', peer_id: 'peer', peer_name: '另一视图中的设备', expires_at: '' }]);
    const prompt = renderToStaticMarkup(createElement(QueryClientProvider, { client }, createElement(LANPairPrompt, { run, op: '', available: true })));
    expect(prompt).toContain('另一视图中的设备 想添加此设备');
    expect(prompt).toContain('添加设备');
  });
  it('shows restart confirmation separately from actual running transfer states', () => {
    const markup = renderToStaticMarkup(createElement(Queue, { items: [queueItem('pending', 'needs_attention')], devices: [device], paused: false, run, op: '', available: true }));
    expect(markup).toContain('确认继续');
    expect(markup).toContain('需要确认');
    expect(markup).not.toContain('已完成');
  });
  it('uses local aliases without changing identity and excludes running items from reordering', () => {
    expect(deviceName(device)).toBe('我的电脑');
    expect(deviceName({ ...device, name: '远端改名' })).toBe('我的电脑');
    expect(movedQueueIDs([queueItem('a', 'queued'), queueItem('running', 'running'), queueItem('b', 'waiting_peer')], 'b', -1)).toEqual(['b', 'a']);
  });
  it('preserves dirty category input while rebasing clean settings fields', () => {
    const current: DesktopPreferences = { format_version: 1, revision: 2, device_name: '正在输入', receive_directory: 'C:\\old', conflict_policy: 'keep_both', server_url: 'https://old.example', bind_address: '', interface_priority: [], excluded_interfaces: [], stun_urls: [], background: { close_mode: '', notifications: false, prevent_sleep: false } };
    const latest: DesktopPreferences = { ...current, revision: 3, device_name: '后台名称', receive_directory: 'C:\\new', server_url: 'https://new.example' };
    const merged = mergePreferences(current, latest, new Set(['device_name' as const]));
    expect(merged.device_name).toBe('正在输入');
    expect(merged.receive_directory).toBe('C:\\new');
    expect(merged.server_url).toBe('https://new.example');
    const sameSection = mergePreferences({ ...current, conflict_policy: 'skip' }, latest, new Set(['conflict_policy' as const]));
    expect(sameSection.conflict_policy).toBe('skip');
    expect(sameSection.receive_directory).toBe('C:\\new');
    const savedReceive = { ...latest, revision: 4, conflict_policy: 'skip' };
    const afterSave = mergeSavedPreferences({ ...current, device_name: '未保存名称', conflict_policy: 'skip' }, savedReceive, new Set(['device_name' as const, 'conflict_policy' as const]), 'receive');
    expect(afterSave.preferences.device_name).toBe('未保存名称');
    expect(afterSave.preferences.conflict_policy).toBe('skip');
    expect([...afterSave.dirty]).toEqual(['device_name']);
  });
  it('maps every LAN pairing completion state to an actionable result', () => {
    expect(lanPairServerMessage('joined')).toContain('同步完成');
    expect(lanPairServerMessage('switch_required')).toContain('配对码');
    expect(lanPairServerMessage('pending')).toContain('服务恢复');
    expect(lanPairServerMessage('not_joined')).toContain('局域网信任');
  });
});
