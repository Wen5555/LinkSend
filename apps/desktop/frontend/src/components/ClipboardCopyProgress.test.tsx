// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, expect, it, vi } from 'vitest';
import { activeLargeCopies, ClipboardCopyProgressView } from './ClipboardCopyProgress';
import type { ClipboardFileProgress } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
vi.mock('../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app',()=>({ClipboardWatcher:vi.fn(),CancelClipboardFile:vi.fn()}));
vi.mock('../hooks/useDesktop',()=>({bridgeAvailable:()=>false}));
const copy = (patch: Partial<ClipboardFileProgress> = {}): ClipboardFileProgress => ({id:'copy',peer_id:'peer',direction:'send',state:'transferring',total:64*1024*1024,verified:16*1024*1024,file_count:1,large:true,...patch});
describe('large file clipboard progress',()=>{
 it('keeps small copies and completed/cancelled copies out of the popup',()=>{
  expect(activeLargeCopies([copy({large:false}),copy({state:'completed'}),copy({state:'cancelled'})])).toEqual([]);
  expect(activeLargeCopies([copy({state:'failed'})])).toHaveLength(1);
 });
 it('shows confirmed progress, offers cancellation, and keeps preparation indeterminate',()=>{
  const container=document.createElement('div');const root=createRoot(container);const cancel=vi.fn();
  try {
   act(()=>root.render(<ClipboardCopyProgressView copies={[copy()]} cancel={cancel}/>));
   expect(container.textContent).toContain('正在复制');expect(container.querySelector('progress')?.value).toBe(25);
   act(()=>container.querySelector('button')!.click());expect(cancel).toHaveBeenCalledWith('copy');
   act(()=>root.render(<ClipboardCopyProgressView copies={[copy({state:'preparing'})]} cancel={cancel}/>));
   expect(container.querySelector('progress')?.hasAttribute('value')).toBe(false);
  } finally {act(()=>root.unmount());}
 });
});
