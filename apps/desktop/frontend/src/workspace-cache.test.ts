import { describe, expect, it, vi } from 'vitest';
import type { WorkspaceSnapshot } from '../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { CommandLanes, commandOptions, confirmThenRefresh, createQueryClient, EnqueueIdentity, invalidateWorkspace, WorkspaceGate, workspaceChange, workspaceKey } from './workspace-cache';

const snapshot = (epoch: string, revision: number, peer = ''): WorkspaceSnapshot => ({
  epoch, revision, draft: { id: 'main', revision, peer_id: peer, paths: ['C:\\报告.txt'], updated_at: '' },
  queue: [], tasks: [], queue_paused: false, persistence_available: true,
});

describe('authoritative workspace cache', () => {
  it('rejects a lower revision before it can replace draft or queue controls', () => {
    const gate = new WorkspaceGate();
    const newer = gate.accept({ ...snapshot('process-a', 9, 'new-peer'), queue_paused: true });
    expect(gate.accept(snapshot('process-a', 3, 'old-peer'))).toBe(newer);
  });
  it('allows a fresh backend epoch and rejects delayed responses or events from the old epoch', () => {
    const gate = new WorkspaceGate();
    gate.accept(snapshot('process-a', 100));
    expect(gate.notice({ epoch: 'process-b', revision: 1 })).toBe(true);
    const next = gate.accept(snapshot('process-b', 1));
    expect(gate.accept(snapshot('process-a', 999))).toBe(next);
    expect(gate.notice({ epoch: 'process-a', revision: 1000 })).toBe(false);
  });
  it('does not mistake an older request with a previously unseen epoch for a newer process', () => {
    const gate = new WorkspaceGate();
    const first = gate.beginRead(), second = gate.beginRead();
    const newer = gate.accept(snapshot('new-process', 2), second);
    expect(gate.accept(snapshot('old-process', 99), first)).toBe(newer);
  });
  it('keeps a new epoch event when an earlier first read returns before any snapshot has been accepted', () => {
    const gate = new WorkspaceGate();
    const oldRead = gate.beginRead();
    gate.notice({ epoch: 'new-process', revision: 1 });
    expect(() => gate.accept(snapshot('old-process', 100), oldRead)).toThrow('WORKSPACE_STALE_EPOCH');
    expect(gate.needsRead()).toBe(true);
    const current = gate.accept(snapshot('new-process', 1), gate.beginRead());
    expect(current.epoch).toBe('new-process');
    expect(gate.needsRead()).toBe(false);
  });
  it('can discover a restarted backend from a fresh read even when its epoch event was lost', () => {
    const gate = new WorkspaceGate();
    gate.accept(snapshot('old-process', 50));
    expect(gate.accept(snapshot('new-process', 1), gate.beginRead()).epoch).toBe('new-process');
  });
  it('preserves an event arriving while a snapshot read is in flight', () => {
    const gate = new WorkspaceGate();
    gate.accept(snapshot('process-a', 2));
    gate.notice({ epoch: 'process-a', revision: 5 });
    gate.accept(snapshot('process-a', 3));
    expect(gate.needsRead()).toBe(true);
    gate.accept(snapshot('process-a', 5));
    expect(gate.needsRead()).toBe(false);
  });
  it('invalidates query state only for a validated, newer backend event', () => {
    const client = createQueryClient(), gate = new WorkspaceGate();
    client.setQueryData(workspaceKey, gate.accept(snapshot('process-a', 2)));
    expect(invalidateWorkspace(client, gate, { epoch: 'process-a', revision: 2 })).toBe(false);
    expect(invalidateWorkspace(client, gate, { epoch: 'process-a', revision: '3' })).toBe(false);
    expect(invalidateWorkspace(client, gate, { epoch: 'process-a', revision: 3 })).toBe(true);
    expect(client.getQueryState(workspaceKey)?.isInvalidated).toBe(true);
    client.clear();
  });
  it('rejects malformed events and requests a fresh read on window focus or reconnect', () => {
    for (const value of [null, {}, { epoch: '', revision: 1 }, { epoch: 'a', revision: -1 }, { epoch: 'a', revision: Infinity }]) expect(workspaceChange(value)).toBeUndefined();
    const client = createQueryClient();
    expect(client.getDefaultOptions().queries?.refetchOnWindowFocus).toBe('always');
    expect(client.getDefaultOptions().queries?.refetchOnReconnect).toBe('always');
    client.clear();
  });
});

describe('enqueue commands', () => {
  it('admits transfer cancellation while enqueue is still running but rejects a second draft edit', async () => {
    const lanes = new CommandLanes();
    const finishEnqueue = lanes.tryStart('draft');
    expect(finishEnqueue).toBeTypeOf('function');
    expect(lanes.tryStart('draft')).toBeUndefined();
    const finishControl = lanes.tryStart('transfer-control');
    const cancelTask = vi.fn().mockResolvedValue(undefined);
    expect(finishControl).toBeTypeOf('function');
    await cancelTask();
    finishControl?.();
    expect(cancelTask).toHaveBeenCalledTimes(1);
    expect(lanes.tryStart('draft')).toBeUndefined();
    finishEnqueue?.();
    expect(lanes.tryStart('draft')).toBeTypeOf('function');
    finishEnqueue?.();
    expect(lanes.tryStart('draft')).toBeUndefined();
  });
  it('keeps a confirmed command successful if only the subsequent read fails', async () => {
    const command = vi.fn().mockResolvedValue('committed-item');
    const refresh = vi.fn().mockRejectedValue(new Error('read unavailable'));
    await expect(confirmThenRefresh(command, refresh)).resolves.toEqual({ value: 'committed-item', refreshFailed: true });
    expect(command).toHaveBeenCalledTimes(1);
  });
  it('does not automatically retry a backend mutation after failure', async () => {
    const client = createQueryClient();
    const action = vi.fn().mockRejectedValue(new Error('lost response'));
    const mutation = client.getMutationCache().build(client, { ...commandOptions, mutationFn: action });
    await expect(mutation.execute(undefined)).rejects.toThrow('lost response');
    expect(action).toHaveBeenCalledTimes(1);
    expect(client.getDefaultOptions().mutations?.retry).toBe(false);
    client.clear();
  });
  it('reuses the request id for an explicit retry until enqueue and draft clearing are acknowledged', () => {
    const makeID = vi.fn().mockReturnValueOnce('request-a').mockReturnValueOnce('request-b');
    const identity = new EnqueueIdentity(makeID);
    const payload = { peer_id: 'peer-a', paths: ['C:\\report.txt'], wait_for_peer: true };
    const first = identity.forPayload(payload, 1);
    expect(identity.forPayload({ ...payload }, 1)).toBe(first);
    identity.confirmed('unrelated-request');
    expect(identity.forPayload(payload, 1)).toBe(first);
    identity.confirmed(first);
    expect(identity.forPayload(payload, 1)).toBe(first);
    expect(identity.forPayload(payload, 2)).toBe('request-b');
  });
  it('does not reuse an id for an edited target or changed waiting choice', () => {
    let id = 0;
    const identity = new EnqueueIdentity(() => String(++id));
    expect(identity.forPayload({ peer: 'a', wait: false })).toBe('1');
    expect(identity.forPayload({ peer: 'a', wait: false }, 9)).toBe('1');
    expect(identity.forPayload({ peer: 'a', wait: true })).toBe('2');
    expect(identity.forPayload({ peer: 'b', wait: true })).toBe('3');
    expect(identity.forPayload({ peer: 'a', wait: false })).toBe('1');
  });
  it('never evicts an uncertain request silently when the bounded registry is full', () => {
    let id = 0;
    const identity = new EnqueueIdentity(() => String(++id), 2);
    identity.forPayload('a'); identity.forPayload('b');
    expect(() => identity.forPayload('c')).toThrow('未确认');
    expect(identity.forPayload('a')).toBe('1');
    identity.confirmed('2');
    expect(identity.forPayload('c')).toBe('3');
    expect(identity.forPayload('a')).toBe('1');
  });
});
