import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { DeviceInfo, QueueItem, TaskSnapshot, WorkspaceSnapshot } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { TransferQueue, transferRows } from './TransferQueue';
import { IncomingTransfers } from '../hooks/useIncomingBadge';

vi.mock('../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app', () => ({}));
const task = (id: string, direction = 'receive', state = 'transferring') => ({ id, task_id: id, attempt_id: 'attempt', direction, state, peer_id: 'trusted', started_at: id, total_bytes: 1024, processed_bytes: 512, rate_bytes_per_second: 256, phase: 'transferring', can_cancel: true } as TaskSnapshot);
const device = { id: 'trusted', trusted: true, blocked: false, profile: {}, name: 'Mac' } as DeviceInfo;
const queue = { id: 'q', task_id: 'send', peer_id: 'trusted', created_at: 'a', state: 'running', source_summary: 'report.txt' } as QueueItem;
const workspace = (tasks: TaskSnapshot[], queues: QueueItem[] = []) => ({ epoch: 'app-session', tasks, queue: queues } as WorkspaceSnapshot);

describe('unified transfers', () => {
  it('merges a running sender into its queue row and keeps receiver and completed rows', () => {
    const state = workspace([task('send', 'send'), task('receive'), task('done', 'receive', 'completed')], [queue]);
    expect(transferRows(state)).toHaveLength(3);
    const html = renderToStaticMarkup(createElement(TransferQueue, { workspace: state, devices: [device], run: async () => true, op: '', available: true }));
    expect(html.match(/data-task-id="send"/g)).toHaveLength(1);
    expect(html).toContain('data-task-id="receive"');
    expect(html).toContain('256 B/s · 50%');
    expect(html).toContain('清除已完成记录');
    expect(html).toContain('清除所有任务');
    expect(html).toContain('取消传输 receive');
  });
  it('keeps an incoming count until visiting transfers and does not count retries twice', () => {
    const tracker = new IncomingTransfers();
    expect(tracker.update(workspace([]), [device], true)).toBe(0);
    expect(tracker.update(workspace([task('new')]), [device], false)).toBe(1);
    expect(tracker.update(workspace([{ ...task('new'), attempt_id: 'retry' }]), [device], false)).toBe(1);
    expect(tracker.update(workspace([task('new', 'receive', 'completed'), task('outgoing', 'send')]), [device], false)).toBe(1);
    expect(tracker.update(workspace([task('new')]), [device], true)).toBe(0);
    expect(tracker.update(workspace([task('new')]), [device], false)).toBe(0);
  });
  it('does not badge historical, untrusted, or blacklisted transfers', () => {
    const tracker = new IncomingTransfers();
    expect(tracker.update(workspace([task('historical')]), [device], false)).toBe(0);
    expect(tracker.update(workspace([task('historical'), { ...task('unknown'), peer_id: 'unknown' }]), [device], false)).toBe(0);
    expect(tracker.update(workspace([task('blocked')]), [{ ...device, blocked: true }], false)).toBe(0);
  });
});
