// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { DeviceInfo, SendDraft } from '../../bindings/github.com/Wen5555/LinkSend/internal/app/models';
import { QuickSendDialog } from './QuickSendDialog';
import { CommandLanes, createQueryClient } from '../workspace-cache';

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
const backend = vi.hoisted(()=>({QuickSendDraft:vi.fn(),SaveQuickSendDraft:vi.fn(),EnqueueQuickSend:vi.fn(),PickFiles:vi.fn(),PickSourceDirectory:vi.fn()}));
vi.mock('../../bindings/github.com/Wen5555/LinkSend/apps/desktop/app',()=>backend);
vi.mock('@wailsio/runtime',()=>({Events:{On:vi.fn(()=>()=>{})}}));
const draft: SendDraft={id:'quick',peer_id:'paired',paths:['C:\\fixture\\快速.txt'],revision:7,updated_at:''};
const device={id:'paired',name:'常用设备',trusted:true,blocked:false,online:true,profile:{alias:''}} as DeviceInfo;
const roots: Root[]=[];
afterEach(()=>{for(const root of roots.splice(0))act(()=>root.unmount());document.body.replaceChildren();vi.resetAllMocks();});
async function mount(devices=[device]) {
  backend.QuickSendDraft.mockResolvedValue(draft);
  const close=vi.fn();const container=document.createElement('div');document.body.append(container);const root=createRoot(container);roots.push(root);
  const client=createQueryClient();
  await act(async()=>{root.render(<QueryClientProvider client={client}><QuickSendDialog devices={devices} available busy={false} lanes={new CommandLanes()} refresh={async()=>{}} close={close}/></QueryClientProvider>);await new Promise(resolve=>setTimeout(resolve,10));});
  await act(async()=>{await new Promise(resolve=>setTimeout(resolve,10));});
  return {container,close};
}
const button=(container:HTMLElement,label:string)=>Array.from(container.querySelectorAll('button')).find(button=>button.textContent===label)!;
async function click(container:HTMLElement,label:string){await act(async()=>{button(container,label).click();await new Promise(resolve=>setTimeout(resolve,10));});}

describe('独立快速发送面板',()=>{
  it('cancels without mutating any persistent draft and restores focus',async()=>{
    const previous=document.createElement('button');document.body.append(previous);previous.focus();
    const {container,close}=await mount();
    expect(document.activeElement?.getAttribute('aria-label')).toBe('查找快速发送设备');
    await click(container,'取消');expect(close).toHaveBeenCalledOnce();expect(backend.SaveQuickSendDraft).not.toHaveBeenCalled();expect(backend.EnqueueQuickSend).not.toHaveBeenCalled();
    act(()=>roots.pop()!.unmount());expect(document.activeElement).toBe(previous);
  });
  it('retains source paths when the system picker is cancelled',async()=>{
    backend.PickFiles.mockResolvedValue([]);const {container}=await mount();await click(container,'选择文件');
    expect(backend.SaveQuickSendDraft).not.toHaveBeenCalled();expect(container.textContent).toContain('快速.txt');
  });
  it('submits only the saved revision and clears only quick after Go success',async()=>{
    backend.EnqueueQuickSend.mockResolvedValue({id:'queue'});backend.SaveQuickSendDraft.mockResolvedValue({...draft,paths:[],revision:8});
    const {container,close}=await mount();await click(container,'加入发送队列');
    expect(backend.EnqueueQuickSend).toHaveBeenCalledExactlyOnceWith(7,false);
    expect(backend.SaveQuickSendDraft).toHaveBeenCalledExactlyOnceWith({...draft,paths:[]});expect(close).toHaveBeenCalledOnce();
    expect(container.textContent).not.toContain('快速.txt'); // late old query cannot restore cleared paths
  });
  it('retains the draft on rejection and requires explicit offline waiting',async()=>{
    const {container,close}=await mount([{...device,online:false}]);
    expect(button(container,'加入发送队列').disabled).toBe(true);
    act(()=>container.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click());
    backend.EnqueueQuickSend.mockRejectedValueOnce(new Error('DEVICE_REVOKED'));await click(container,'加入发送队列');
    expect(backend.EnqueueQuickSend).toHaveBeenCalledExactlyOnceWith(7,true);expect(backend.SaveQuickSendDraft).not.toHaveBeenCalled();expect(close).not.toHaveBeenCalled();
  });
  it('does not enable sending to a revoked saved target',async()=>{
    const {container}=await mount([{...device,trusted:false,blocked:true}]);expect(button(container,'加入发送队列').disabled).toBe(true);expect(container.textContent).toContain('当前不可发送');
  });
  it('captures the selected identity before the controlled select resets during async mutation',async()=>{
    backend.SaveQuickSendDraft.mockResolvedValueOnce({...draft,peer_id:'other',revision:8});
    const {container}=await mount([device,{...device,id:'other',name:'另一台'}]);
    const select=container.querySelector<HTMLSelectElement>('select')!;
    await act(async()=>{select.value='other';select.dispatchEvent(new Event('change',{bubbles:true}));await new Promise(resolve=>setTimeout(resolve,10));});
    expect(backend.SaveQuickSendDraft).toHaveBeenCalledExactlyOnceWith({...draft,peer_id:'other'});
    expect(select.value).toBe('other');
  });
  it('coalesces rapid duplicate clicks before React disables the submit control',async()=>{
    let finish!:()=>void;
    backend.EnqueueQuickSend.mockImplementationOnce(()=>new Promise<void>(resolve=>{finish=resolve}));
    backend.SaveQuickSendDraft.mockResolvedValue({...draft,paths:[],revision:8});
    const {container}=await mount();
    await act(async()=>{button(container,'加入发送队列').click();button(container,'加入发送队列').click();await new Promise(resolve=>setTimeout(resolve,10));});
    expect(backend.EnqueueQuickSend).toHaveBeenCalledExactlyOnceWith(7,false);
    await act(async()=>{finish();await new Promise(resolve=>setTimeout(resolve,10));});
  });
});
