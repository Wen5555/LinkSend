import { useQuery } from '@tanstack/react-query';
import * as Backend from '../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app';
import type { DeviceInfo } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import type { CommandRunner } from '../hooks/useDesktop';
import { deviceName } from '../presentation';

export function ClipboardDevices({ devices, run, op, available }: { devices: DeviceInfo[]; run: CommandRunner; op: string; available: boolean }) {
  return <div className="clipboard-device-list"><h3>同步的设备</h3><p className="field-help">在两台设备上分别开启同步。开启后双向同步新复制的文字、链接和图片。</p>{!devices.length && <p className="field-help">请先到“设备”完成配对。</p>}{devices.map(device => <ClipboardDevice key={device.id} device={device} run={run} op={op} available={available} />)}</div>;
}

export function ClipboardDevice({ device, run, op, available }: { device: DeviceInfo; run: CommandRunner; op: string; available: boolean }) {
  const grants = useQuery({ queryKey: ['clipboard-grants', device.id], enabled: available && device.trusted && !device.blocked, queryFn: () => Backend.ClipboardGrants(device.id), retry: false });
  const contentGrants = (grants.data ?? []).filter(grant => grant.kind !== 'files');
  const enabled = contentGrants.filter(grant => grant.enabled).length === 6;
  const partial = !enabled && contentGrants.some(grant => grant.enabled);
  const filesEnabled = (grants.data ?? []).some(grant => grant.kind === 'files' && grant.enabled);
  const disabled = !!op || !available || grants.isPending || grants.isError || !device.trusted || device.blocked;
  return <div className="clipboard-device-permissions"><div className="toggle-row"><div><strong>{deviceName(device)}</strong><small>{grants.isError ? '权限读取失败，请重试' : partial ? '已自定义部分方向或类型' : enabled ? '已允许双向同步' : '未开启同步'}</small>{grants.isError && <button className="ghost" onClick={() => void grants.refetch()}>重试</button>}</div><input type="checkbox" aria-label={'与 ' + deviceName(device) + ' 自动同步剪贴板'} checked={enabled || partial} disabled={disabled} onChange={event => {
    const next = event.target.checked;
    void run('clipboard-device-' + device.id, async () => {
      // Read fresh revisions, retaining partial changes on failure for retry.
      const current = (await Backend.ClipboardGrants(device.id)) ?? [];
      try {
        for (const direction of ['send', 'receive']) for (const kind of ['text', 'link', 'image']) {
          const saved = current.find(grant => grant.direction === direction && grant.kind === kind);
          if ((saved?.enabled ?? false) !== next) await Backend.SetClipboardGrant({ peer_id: device.id, direction, kind, enabled: next, expected_revision: saved?.revision ?? 0 });
        }
      } finally { await grants.refetch(); }
    }, next ? '已允许此设备同步；请确认另一端也已开启总开关与设备同步' : '已关闭此设备的剪贴板同步');
  }} /></div><label className="toggle-row clipboard-file-permission"><span><strong>同步复制的文件</strong><small>允许此设备自动收发新复制的文件；文件验证完成后可在另一端粘贴。大文件会显示复制进度。</small></span><input type="checkbox" aria-label={'与 ' + deviceName(device) + ' 同步复制的文件'} checked={filesEnabled} disabled={disabled} onChange={event => {
    const next = event.target.checked;
    void run('clipboard-files-' + device.id, async () => {
      const current = (await Backend.ClipboardGrants(device.id)) ?? [];
      try { for (const direction of ['send', 'receive']) {
        const saved = current.find(grant => grant.direction === direction && grant.kind === 'files');
        if ((saved?.enabled ?? false) !== next) await Backend.SetClipboardGrant({ peer_id: device.id, direction, kind: 'files', enabled: next, expected_revision: saved?.revision ?? 0 });
      } } finally { await grants.refetch(); }
    }, next ? '已允许文件剪贴板同步；另一端也需要开启文件同步' : '已关闭文件剪贴板同步');
  }} /></label></div>;
}
