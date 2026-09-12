import { useCallback, useEffect, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Events } from '@wailsio/runtime';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import { humanizeBackendError } from '../connection';
import { CommandLanes, commandOptions, confirmThenRefresh, deviceKey, invalidateWorkspace, shellKey, StaleWorkspaceEpochError, WorkspaceGate, workspaceKey } from '../workspace-cache';

export function bridgeAvailable(): boolean {
  // beta.18 also installs _wails.invoke in an ordinary browser, as a no-op.
  // Check the native endpoints used by that version's runtime/system.js.
  const host = window as Window & {
    chrome?: { webview?: { postMessage?: unknown } };
    webkit?: { messageHandlers?: { external?: { postMessage?: unknown } } };
    wails?: { invoke?: unknown };
  };
  return typeof host.chrome?.webview?.postMessage === 'function' ||
    typeof host.webkit?.messageHandlers?.external?.postMessage === 'function' || typeof host.wails?.invoke === 'function';
}

export function useDesktop() {
  const client = useQueryClient();
  const gate = useRef(new WorkspaceGate());
  const enabled = bridgeAvailable();
  const workspace = useQuery({ queryKey: workspaceKey, enabled, refetchInterval: 10000,
    queryFn: async ({ signal }) => {
      const read = async () => {
        const readSequence = gate.current.beginRead();
        return gate.current.accept(await Backend.Workspace().cancelOn(signal), readSequence);
      };
      try { return await read(); }
      catch (cause) {
        if (!(cause instanceof StaleWorkspaceEpochError)) throw cause;
        return read(); // A single reconciliation read after an epoch event raced startup.
      }
    },
  });
  const devices = useQuery({ queryKey: deviceKey, enabled, refetchInterval: 5000,
    queryFn: async ({ signal }) => (await Backend.Devices().cancelOn(signal)) ?? [],
  });
  const shell = useQuery({ queryKey: shellKey, enabled, refetchInterval: 5000,
    queryFn: async () => {
      const [status, inbox, membership, diagnostics, entries, background] = await Promise.all([
        Backend.Status(), Backend.InboxStatus(), Backend.Membership(), Backend.Diagnostics(), Backend.DesktopEntries(), Backend.Background(),
      ]);
      return { status, inbox, membership, diagnostics, entries, background };
    },
  });
  const preferences = useQuery({ queryKey: ['preferences'], enabled, queryFn: () => Backend.Preferences() });
  const configuration = useQuery({ queryKey: ['configuration'], enabled, queryFn: async () => {
    const [identity, interfaces, effective, preferencesStatus] = await Promise.all([
      Backend.Identity(), Backend.NetworkInterfaces(), Backend.EffectiveConfig(), Backend.PreferencesStatus(),
    ]);
    return { identity, interfaces: interfaces ?? [], effective, preferencesStatus };
  } });
  useEffect(() => {
    if (!enabled) return;
    let timer: number | undefined;
    const unsubscribe = Events.On('workspace:changed', event => {
      const payload: unknown = event.data;
      invalidateWorkspace(client, gate.current, payload);
      if (timer === undefined) timer = window.setTimeout(() => {
        timer = undefined;
        void client.invalidateQueries({ queryKey: ['inbox'] }, { cancelRefetch: false });
      }, 1000);
    });
    return () => { unsubscribe(); if (timer !== undefined) window.clearTimeout(timer); };
  }, [client, enabled]);
  // An invalidation can arrive while a query is already reading SQLite. Once
  // that read finishes, fetch again if its snapshot predates the observed event.
  useEffect(() => {
    if (!enabled || !workspace.data || workspace.isFetching || workspace.isError || !gate.current.needsRead()) return;
    const timer = window.setTimeout(() => void client.invalidateQueries({ queryKey: workspaceKey }, { cancelRefetch: false }), 200);
    return () => window.clearTimeout(timer);
  }, [client, enabled, workspace.data, workspace.dataUpdatedAt, workspace.isFetching, workspace.isError]);
  const refresh = useCallback(async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: workspaceKey }), client.invalidateQueries({ queryKey: deviceKey }),
      client.invalidateQueries({ queryKey: shellKey }),
      client.invalidateQueries({ queryKey: ['preferences'] }), client.invalidateQueries({ queryKey: ['configuration'] }),
      client.invalidateQueries({ queryKey: ['inbox'] }),
    ]);
  }, [client]);
  useEffect(() => {
    if (!enabled) return;
    const focus = () => { void refresh(); };
    window.addEventListener('focus', focus);
    return () => window.removeEventListener('focus', focus);
  }, [enabled, refresh]);
  return { enabled, workspace, devices, shell, preferences, configuration, refresh };
}

export type CommandRunner = (key: string, action: () => Promise<unknown>, message?: string, onError?: (error: unknown) => void) => Promise<boolean>;
type Command = { key: string; action: () => Promise<unknown> };

export function useCommand(refresh: () => Promise<void>, lanes: CommandLanes, lane: string) {
  const latestRun = useRef(0);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const { mutateAsync, isPending, variables } = useMutation({ ...commandOptions, mutationFn: (command: Command) => command.action() });
  const run: CommandRunner = useCallback(async (key, action, message, onError) => {
    const release = lanes.tryStart(lane);
    if (!release) return false;
    const runID = ++latestRun.current;
    setError(''); setNotice('');
    try {
      const result = await confirmThenRefresh(async () => {
        const value = await mutateAsync({ key, action });
        release(); // Refreshing presence must not hold up the next transfer control.
        return value;
      }, refresh);
      if (latestRun.current === runID) {
        if (message) setNotice(message);
        if (result.refreshFailed) setError('操作已由后端确认，但界面刷新失败。请重试读取状态；无需重复提交。');
      }
      return true;
    } catch (cause) {
      if (latestRun.current === runID) setError(humanizeBackendError(cause));
      onError?.(cause);
      return false;
    } finally { release(); }
  }, [mutateAsync, refresh, lanes, lane]);
  return { run, error, notice, clearError: () => setError(''), clearNotice: () => setNotice(''), op: isPending ? variables?.key ?? '' : '' };
}
