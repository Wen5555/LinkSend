import { QueryClient } from '@tanstack/react-query';
import type { WorkspaceChange, WorkspaceSnapshot } from '../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { mergeTaskSnapshots } from './connection';

export const workspaceKey = ['workspace'] as const;
export const deviceKey = ['devices'] as const;
export const shellKey = ['desktop'] as const;
export const commandOptions = { retry: false } as const;
export class StaleWorkspaceEpochError extends Error {
  constructor() { super('WORKSPACE_STALE_EPOCH'); }
}

/** Draft preparation can hash large files. It must not lock the independent
 * lane used to stop, pause, or decide an existing transfer. */
export class CommandLanes {
  private active = new Set<string>();
  tryStart(lane: string): (() => void) | undefined {
    if (this.active.has(lane)) return;
    this.active.add(lane);
    let released = false;
    return () => { if (!released) { released = true; this.active.delete(lane); } };
  }
}

export async function confirmThenRefresh<T>(command: () => Promise<T>, refresh: () => Promise<unknown>) {
  const value = await command();
  try { await refresh(); return { value, refreshFailed: false }; }
  catch { return { value, refreshFailed: true }; }
}

export function createQueryClient() {
  return new QueryClient({ defaultOptions: {
    queries: { retry: false, staleTime: 1000, refetchOnWindowFocus: 'always', refetchOnReconnect: 'always' },
    mutations: commandOptions,
  } });
}

export function workspaceChange(value: unknown): WorkspaceChange | undefined {
  if (typeof value !== 'object' || value === null || !('epoch' in value) || !('revision' in value)) return;
  if (typeof value.epoch !== 'string' || !value.epoch || typeof value.revision !== 'number' ||
      !Number.isSafeInteger(value.revision) || value.revision < 0) return;
  return { epoch: value.epoch, revision: value.revision };
}

/** A new backend process starts a new epoch; delayed responses from retired
 * processes and lower revisions cannot restore stale drafts or queue states. */
export class WorkspaceGate {
  private retired = new Set<string>();
  private expectedEpoch = '';
  private expectedRevision = 0;
  private current?: WorkspaceSnapshot;
  private readSequence = 0;
  private appliedRead = 0;
  private epochEventReadFence = 0;

  beginRead(): number { return ++this.readSequence; }

  needsRead(): boolean {
    return !!this.expectedEpoch && (this.current?.epoch !== this.expectedEpoch || this.current.revision < this.expectedRevision);
  }

  notice(change: WorkspaceChange): boolean {
    if (this.retired.has(change.epoch)) return false;
    if (this.expectedEpoch !== change.epoch) this.epochEventReadFence = this.readSequence;
    if (this.expectedEpoch && this.expectedEpoch !== change.epoch) {
      this.retired.add(this.expectedEpoch);
      this.expectedRevision = 0;
    }
    this.expectedEpoch = change.epoch;
    this.expectedRevision = Math.max(this.expectedRevision, change.revision);
    return this.current?.epoch !== change.epoch || change.revision > this.current.revision;
  }

  accept(next: WorkspaceSnapshot, readSequence = this.beginRead()): WorkspaceSnapshot {
    if (readSequence < this.appliedRead && this.current) return this.current;
    if (this.expectedEpoch && next.epoch !== this.expectedEpoch && readSequence <= this.epochEventReadFence) {
      if (this.current) return this.current;
      throw new StaleWorkspaceEpochError();
    }
    if (this.retired.has(next.epoch)) {
      if (this.current) return this.current;
      throw new StaleWorkspaceEpochError();
    }
    if (this.current?.epoch === next.epoch && this.current.revision > next.revision) return this.current;
    if (this.expectedEpoch && this.expectedEpoch !== next.epoch) {
      this.retired.add(this.expectedEpoch);
      this.expectedRevision = 0;
    }
    this.expectedEpoch = next.epoch;
    this.appliedRead = readSequence;
    const tasks = this.current?.epoch === next.epoch
      ? mergeTaskSnapshots(this.current.tasks ?? [], next.tasks ?? []) : next.tasks ?? [];
    this.current = { ...next, draft: { ...next.draft, paths: next.draft.paths ?? [] }, queue: next.queue ?? [], tasks };
    return this.current;
  }
}

export function invalidateWorkspace(client: QueryClient, gate: WorkspaceGate, value: unknown): boolean {
  const change = workspaceChange(value);
  if (!change || !gate.notice(change)) return false;
  void client.invalidateQueries({ queryKey: workspaceKey }, { cancelRefetch: false });
  return true;
}

/** Keep a failed/uncertain enqueue's id until the same payload is acknowledged.
 * The backend uses that id to return the committed item after a lost response. */
export class EnqueueIdentity {
  private pending = new Map<string, { payload: string; requestID: string; acknowledged: boolean }>();
  constructor(private readonly makeID: () => string, private readonly limit = 32) {}
  forPayload(payload: unknown, draftRevision = 0): string {
    const logicalPayload = JSON.stringify(payload);
    const key = JSON.stringify([logicalPayload, draftRevision]);
    const existing = this.pending.get(key);
    if (existing) return existing.requestID;
    // Returning to the same target/content after editing must still resolve an
    // uncertain old enqueue, even though saving the draft advanced its revision.
    const uncertain = [...this.pending.values()].find(item => !item.acknowledged && item.payload === logicalPayload);
    if (uncertain) return uncertain.requestID;
    if (this.pending.size >= this.limit) {
      const acknowledged = [...this.pending].find(([, item]) => item.acknowledged);
      if (acknowledged) this.pending.delete(acknowledged[0]);
      else throw new Error('尚有多项加入请求未确认，请先刷新队列核对结果，再继续添加。');
    }
    const item = { payload: logicalPayload, requestID: this.makeID(), acknowledged: false };
    this.pending.set(key, item);
    return item.requestID;
  }
  confirmed(requestID: string): void {
    for (const item of this.pending.values()) if (item.requestID === requestID) item.acknowledged = true;
  }
}
