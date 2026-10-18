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
 it('explains disk pressure without asking for manual cache management',()=>{
  const container=document.createElement('div');const root=createRoot(container);
  try {
   act(()=>root.render(<ClipboardCopyProgressView copies={[copy({state:'failed',error:'file_copy_disk_full'})]} cancel={vi.fn()}/>));
   expect(container.textContent).toContain('空间不足，暂时无法复制文件。');
   expect(container.textContent).toContain('文件未写入剪贴板');
   expect(container.querySelector('progress')?.hasAttribute('value')).toBe(false);
   expect(container.querySelector('button')?.textContent).toBe('关闭');
  } finally {act(()=>root.unmount());}
 });
 it('does not claim files are unpublished when native completion is uncertain',()=>{
  const container=document.createElement('div');const root=createRoot(container);
  try {
   act(()=>root.render(<ClipboardCopyProgressView copies={[copy({state:'failed',error:'file_copy_publication_uncertain'})]} cancel={vi.fn()}/>));
   expect(container.textContent).toContain('系统剪贴板状态未能确认，文件已保留。');
   expect(container.textContent).not.toContain('文件未写入剪贴板');
   expect(container.querySelector('button')?.textContent).toBe('关闭');
  } finally {act(()=>root.unmount());}
 });
});
