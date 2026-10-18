import { useEffect, useRef, useState } from 'react';
import type { DeviceInfo, WorkspaceSnapshot } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';

export class IncomingTransfers {
  private epoch?: string;
  private seen = new Set<string>();
  private unread = new Set<string>();
  update(workspace: WorkspaceSnapshot, devices: DeviceInfo[], viewing: boolean) {
    const first = this.epoch !== workspace.epoch;
    if (first) { this.epoch = workspace.epoch; this.seen.clear(); this.unread.clear(); }
    for (const task of workspace.tasks ?? []) {
      const key = task.id; // reconnections/attempts belong to one transfer
      if (!first && !viewing && !this.seen.has(key) && task.direction === 'receive' && devices.some(device => device.id === task.peer_id && device.trusted && !device.blocked)) this.unread.add(key);
      this.seen.add(key);
    }
    if (viewing) this.unread.clear();
    return this.unread.size;
  }
}

export function useIncomingBadge(workspace: WorkspaceSnapshot | undefined, devices: DeviceInfo[], viewing: boolean) {
  const tracker = useRef(new IncomingTransfers());
  const [count, setCount] = useState(0);
  useEffect(() => { if (workspace) setCount(tracker.current.update(workspace, devices, viewing)); }, [workspace, devices, viewing]);
  return count;
}
