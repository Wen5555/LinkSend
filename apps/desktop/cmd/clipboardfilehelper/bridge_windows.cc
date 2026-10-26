//go:build windows && cgo

// The private receipt-only broker and the independent, not-yet-wired Shell
// publisher share native ownership primitives. Only shell_run may register
// standard formats or touch the system clipboard. Neither path accepts raw
// paths, reopens a source, accesses a profile key/database, or deletes a file.
#define UNICODE
#define _UNICODE
#define WIN32_LEAN_AND_MEAN
#include "bridge.h"
#include <windows.h>
#include <ole2.h>
#include <shlobj.h>
#include <shldisp.h>
#include <algorithm>
#include <array>
#include <atomic>
#include <cstring>
#include <memory>
#include <mutex>
#include <new>
#include <set>
#include <stdexcept>
#include <string>
#include <thread>
#include <vector>

namespace {
constexpr CLIPFORMAT descriptor=CF_PRIVATEFIRST,contents=CF_PRIVATEFIRST+1,effect=CF_PRIVATEFIRST+2;
struct Formats { CLIPFORMAT descriptor,contents,effect; };
constexpr Formats privateFormats{descriptor,contents,effect};
struct Pin {
    HANDLE handle=INVALID_HANDLE_VALUE;
    std::wstring name;
    uint64_t size=0;
    bool directory=false,published=false;
    std::mutex mutex;
    ~Pin() noexcept { (void)close(); }
    bool close() noexcept {
        try { std::lock_guard<std::mutex> lock(mutex);if(handle==INVALID_HANDLE_VALUE)return true;
            if(!CloseHandle(handle))return false;
            handle=INVALID_HANDLE_VALUE;return true;
        } catch(...) {return false;}
    }
    HRESULT read(uint64_t offset,void* data,ULONG count,ULONG* got) noexcept {
        try {std::lock_guard<std::mutex> lock(mutex);*got=0;if(handle==INVALID_HANDLE_VALUE||directory)return STG_E_REVERTED;
            if(offset>MAXLONGLONG)return STG_E_INVALIDFUNCTION;
            LARGE_INTEGER position{};position.QuadPart=static_cast<LONGLONG>(offset);
            if(!SetFilePointerEx(handle,position,nullptr,FILE_BEGIN))return HRESULT_FROM_WIN32(GetLastError());
            DWORD actual=0;if(!ReadFile(handle,data,count,&actual,nullptr))return HRESULT_FROM_WIN32(GetLastError());
            *got=actual;return actual==count?S_OK:S_FALSE;
        }catch(...){return E_FAIL;}
    }
};
struct Owner {
    std::vector<std::shared_ptr<Pin>> all,published;
    std::mutex mutex;
    std::atomic<bool> failed{false};
    std::atomic<uint32_t> checks{0},closed{0};
    int64_t providers=0,streams=0,operations=0,modes=0,calls=0;
    bool exposed=false,factoryClosed=false,closing=false,settled=false,quiescent=false;
    unsigned witnesses=0;
    bool eligible() noexcept {
        try{std::lock_guard<std::mutex> lock(mutex);return !failed&&quiescent&&witnesses==1&&providers==0&&streams==0&&operations==0&&modes==0&&calls==0&&closed==all.size();}
        catch(...){failed=true;return false;}
    }
    bool protect(unsigned bit) noexcept {
        if(eligible()){failed=true;return false;}checks.fetch_or(1u<<bit);return !failed;
    }
    void closeFactory() noexcept {try{std::lock_guard<std::mutex> lock(mutex);factoryClosed=true;}catch(...){failed=true;}}
    bool change(int p=0,int s=0,int o=0,int m=0,int c=0) noexcept {
        try {
            bool finish=false;
            {std::lock_guard<std::mutex> lock(mutex);
                // Failure suppresses every witness, not the accounting of
                // actual constructions/releases. A rejected object/call still
                // has a matching destructor and must not leave phantom refs.
                if((p>0&&(factoryClosed||closing||settled))||((s>0||o>0||m>0||c>0)&&(closing||settled)))failed=true;
                providers+=p;streams+=s;operations+=o;modes+=m;calls+=c;
                if(providers<0||streams<0||operations<0||modes<0||calls<0){failed=true;return false;}
                if(p>0)exposed=true;
                if(exposed&&factoryClosed&&!closing&&!settled&&providers==0&&streams==0&&operations==0&&modes==0&&calls==0){closing=true;finish=true;}
            }
            if(finish){
                if(!failed)(void)protect(6);
                // All actual refs have ended. Close every pin even if a prior
                // I/O or one CloseHandle failed, but never mint quiescence on
                // that failure path. Do not stop accounting at the first error.
                for(auto& pin:all){if(pin->close())closed.fetch_add(1);else failed=true;}
                std::lock_guard<std::mutex> lock(mutex);closing=false;settled=true;
                if(!failed&&closed==all.size()){quiescent=true;++witnesses;}
            }
            return !failed;
        }catch(...){failed=true;return false;}
    }
};
struct Call {
    IUnknown* object;
    std::shared_ptr<Owner> owner;
    bool valid;
    Call(IUnknown* objectIn,const std::shared_ptr<Owner>& ownerIn) noexcept:object(objectIn),owner(ownerIn),valid(false){object->AddRef();valid=owner->change(0,0,0,0,1);}
    ~Call() noexcept {(void)owner->change(0,0,0,0,-1);object->Release();}
};
class Stream final:public IStream {
    std::atomic<ULONG> refs{1};std::shared_ptr<Owner> owner;std::shared_ptr<Pin> pin;uint64_t position=0;std::mutex mutex;
public:
    Stream(const std::shared_ptr<Owner>& o,const std::shared_ptr<Pin>& p,uint64_t at=0) noexcept:owner(o),pin(p),position(at){(void)owner->change(0,1);}
    ~Stream() noexcept {pin.reset();(void)owner->change(0,-1);}
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID iid,void** out) noexcept override {if(!out)return E_POINTER;*out=nullptr;if(iid!=IID_IUnknown&&iid!=IID_ISequentialStream&&iid!=IID_IStream)return E_NOINTERFACE;*out=static_cast<IStream*>(this);AddRef();return S_OK;}
    ULONG STDMETHODCALLTYPE AddRef() noexcept override{return ++refs;}
    ULONG STDMETHODCALLTYPE Release() noexcept override{const auto n=--refs;if(!n)delete this;return n;}
    HRESULT STDMETHODCALLTYPE Read(void* data,ULONG count,ULONG* actual) noexcept override {
        try{Call active(this,owner);if(actual)*actual=0;if(!active.valid)return E_FAIL;if(!data&&count)return STG_E_INVALIDPOINTER;
            std::lock_guard<std::mutex> lock(mutex);ULONG got=0;const auto hr=pin->read(position,data,count,&got);position+=got;if(actual)*actual=got;if(FAILED(hr))owner->failed=true;return hr;
        }catch(...){owner->failed=true;return E_FAIL;}
    }
    HRESULT STDMETHODCALLTYPE Write(const void*,ULONG,ULONG* n) noexcept override{if(n)*n=0;return STG_E_ACCESSDENIED;}
    HRESULT STDMETHODCALLTYPE Seek(LARGE_INTEGER move,DWORD origin,ULARGE_INTEGER* out) noexcept override {
        try{Call active(this,owner);if(!active.valid)return E_FAIL;std::lock_guard<std::mutex> lock(mutex);
            uint64_t base=0;if(origin==STREAM_SEEK_CUR)base=position;else if(origin==STREAM_SEEK_END)base=pin->size;else if(origin!=STREAM_SEEK_SET)return STG_E_INVALIDFUNCTION;
            if(move.QuadPart<0){const auto magnitude=static_cast<uint64_t>(-(move.QuadPart+1))+1;if(magnitude>base)return STG_E_INVALIDFUNCTION;position=base-magnitude;}
            else{if(base>static_cast<uint64_t>(MAXLONGLONG)-static_cast<uint64_t>(move.QuadPart))return STG_E_INVALIDFUNCTION;position=base+static_cast<uint64_t>(move.QuadPart);}
            if(out)out->QuadPart=position;
            return S_OK;
        }catch(...){owner->failed=true;return E_FAIL;}
    }
    HRESULT STDMETHODCALLTYPE SetSize(ULARGE_INTEGER) noexcept override{return STG_E_ACCESSDENIED;}
    HRESULT STDMETHODCALLTYPE CopyTo(IStream* target,ULARGE_INTEGER count,ULARGE_INTEGER* read,ULARGE_INTEGER* written) noexcept override {
        Call active(this,owner);if(read)read->QuadPart=0;if(written)written->QuadPart=0;if(!active.valid)return E_FAIL;if(!target)return E_POINTER;
        std::array<unsigned char,65536> buffer{};uint64_t total=0,put=0;HRESULT result=S_OK;
        while(total<count.QuadPart){ULONG got=0;result=Read(buffer.data(),static_cast<ULONG>(std::min<uint64_t>(buffer.size(),count.QuadPart-total)),&got);if(FAILED(result))break;total+=got;
            if(got){ULONG out=0;result=target->Write(buffer.data(),got,&out);put+=out;if(FAILED(result)||out!=got){if(SUCCEEDED(result))result=STG_E_MEDIUMFULL;break;}}if(!got)break;}
        if(read)read->QuadPart=total;
        if(written)written->QuadPart=put;
        return result;
    }
    HRESULT STDMETHODCALLTYPE Commit(DWORD) noexcept override{return STG_E_ACCESSDENIED;}
    HRESULT STDMETHODCALLTYPE Revert() noexcept override{return STG_E_INVALIDFUNCTION;}
    HRESULT STDMETHODCALLTYPE LockRegion(ULARGE_INTEGER,ULARGE_INTEGER,DWORD) noexcept override{return STG_E_INVALIDFUNCTION;}
    HRESULT STDMETHODCALLTYPE UnlockRegion(ULARGE_INTEGER,ULARGE_INTEGER,DWORD) noexcept override{return STG_E_INVALIDFUNCTION;}
    HRESULT STDMETHODCALLTYPE Stat(STATSTG* out,DWORD flags) noexcept override {
        Call active(this,owner);if(!out)return E_POINTER;*out={};if(!active.valid)return E_FAIL;out->type=STGTY_STREAM;out->cbSize.QuadPart=pin->size;out->grfMode=STGM_READ;
        if(!(flags&STATFLAG_NONAME)){const auto n=(pin->name.size()+1)*sizeof(wchar_t);out->pwcsName=static_cast<wchar_t*>(CoTaskMemAlloc(n));if(!out->pwcsName)return E_OUTOFMEMORY;std::memcpy(out->pwcsName,pin->name.c_str(),n);}return S_OK;
    }
    HRESULT STDMETHODCALLTYPE Clone(IStream** out) noexcept override {
        try{Call active(this,owner);if(!out)return E_POINTER;*out=nullptr;if(!active.valid)return E_FAIL;std::lock_guard<std::mutex> lock(mutex);auto* copy=new(std::nothrow) Stream(owner,pin,position);if(!copy)return E_OUTOFMEMORY;if(owner->failed){copy->Release();return E_FAIL;}*out=copy;return S_OK;
        }catch(...){owner->failed=true;return E_FAIL;}
    }
};
class Enumerator final:public IEnumFORMATETC {
    std::atomic<ULONG> refs{1};std::vector<FORMATETC> entries;size_t at=0;
public:
    explicit Enumerator(std::vector<FORMATETC> input,size_t index=0):entries(std::move(input)),at(index){}
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID iid,void** out) noexcept override{if(!out)return E_POINTER;*out=nullptr;if(iid!=IID_IUnknown&&iid!=IID_IEnumFORMATETC)return E_NOINTERFACE;*out=this;AddRef();return S_OK;}
    ULONG STDMETHODCALLTYPE AddRef() noexcept override{return ++refs;}
    ULONG STDMETHODCALLTYPE Release() noexcept override{auto n=--refs;if(!n)delete this;return n;}
    HRESULT STDMETHODCALLTYPE Next(ULONG n,FORMATETC* out,ULONG* fetched) noexcept override{if(!out||(!fetched&&n!=1))return E_POINTER;ULONG got=0;while(got<n&&at<entries.size())out[got++]=entries[at++];if(fetched)*fetched=got;return got==n?S_OK:S_FALSE;}
    HRESULT STDMETHODCALLTYPE Skip(ULONG n) noexcept override{const auto skip=std::min<size_t>(n,entries.size()-at);at+=skip;return skip==n?S_OK:S_FALSE;}
    HRESULT STDMETHODCALLTYPE Reset() noexcept override{at=0;return S_OK;}
    HRESULT STDMETHODCALLTYPE Clone(IEnumFORMATETC** out) noexcept override{if(!out)return E_POINTER;*out=nullptr;try{*out=new Enumerator(entries,at);return S_OK;}catch(...){return E_OUTOFMEMORY;}}
};
class Data final:public IDataObject,public IDataObjectAsyncCapability {
    std::atomic<ULONG> refs{1};std::shared_ptr<Owner> owner;
    const CLIPFORMAT descriptor,contents,effect;
    bool async=false,modeHold=false;
    uint32_t operations=0;
    void dropMode() noexcept{if(modeHold){modeHold=false;(void)owner->change(0,0,0,-1);Release();}}
public:
    explicit Data(const std::shared_ptr<Owner>& o,Formats formats=privateFormats) noexcept:owner(o),descriptor(formats.descriptor),contents(formats.contents),effect(formats.effect){(void)owner->change(1);}
    ~Data() noexcept{(void)owner->change(-1);}
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID iid,void** out) noexcept override{if(!out)return E_POINTER;*out=nullptr;if(iid==IID_IUnknown||iid==IID_IDataObject)*out=static_cast<IDataObject*>(this);else if(iid==IID_IDataObjectAsyncCapability)*out=static_cast<IDataObjectAsyncCapability*>(this);else return E_NOINTERFACE;AddRef();return S_OK;}
    ULONG STDMETHODCALLTYPE AddRef() noexcept override{return ++refs;}
    ULONG STDMETHODCALLTYPE Release() noexcept override{const auto n=--refs;if(!n)delete this;return n;}
    HRESULT STDMETHODCALLTYPE QueryGetData(FORMATETC* f) noexcept override{if(!f)return E_POINTER;if(f->ptd||f->dwAspect!=DVASPECT_CONTENT)return DV_E_FORMATETC;if(f->cfFormat==descriptor||f->cfFormat==effect){if(f->lindex!=-1)return DV_E_LINDEX;return(f->tymed&TYMED_HGLOBAL)?S_OK:DV_E_TYMED;}if(f->cfFormat==contents){if(f->lindex<0||static_cast<size_t>(f->lindex)>=owner->published.size()||owner->published[f->lindex]->directory)return DV_E_LINDEX;return(f->tymed&TYMED_ISTREAM)?S_OK:DV_E_TYMED;}return DV_E_FORMATETC;}
    HRESULT STDMETHODCALLTYPE GetData(FORMATETC* f,STGMEDIUM* out) noexcept override{
        try{Call active(static_cast<IDataObject*>(this),owner);if(!out)return E_POINTER;*out={};if(!active.valid)return E_FAIL;const auto hr=QueryGetData(f);if(FAILED(hr))return hr;
            if(f->cfFormat==contents){auto* stream=new(std::nothrow) Stream(owner,owner->published[f->lindex]);if(!stream)return E_OUTOFMEMORY;if(owner->failed){stream->Release();return E_FAIL;}out->tymed=TYMED_ISTREAM;out->pstm=stream;return S_OK;}
            const auto size=f->cfFormat==descriptor?sizeof(UINT)+owner->published.size()*sizeof(FILEDESCRIPTORW):sizeof(DWORD);
            auto memory=GlobalAlloc(GMEM_MOVEABLE|GMEM_ZEROINIT,size);if(!memory)return E_OUTOFMEMORY;auto* bytes=GlobalLock(memory);if(!bytes){GlobalFree(memory);return E_OUTOFMEMORY;}
            if(f->cfFormat==descriptor){auto* group=static_cast<FILEGROUPDESCRIPTORW*>(bytes);group->cItems=static_cast<UINT>(owner->published.size());for(size_t i=0;i<owner->published.size();++i){const auto& pin=owner->published[i];auto& e=group->fgd[i];e.dwFlags=FD_ATTRIBUTES|FD_UNICODE;e.dwFileAttributes=pin->directory?FILE_ATTRIBUTE_DIRECTORY:FILE_ATTRIBUTE_NORMAL;if(!pin->directory){e.dwFlags|=FD_FILESIZE;e.nFileSizeHigh=static_cast<DWORD>(pin->size>>32);e.nFileSizeLow=static_cast<DWORD>(pin->size);}std::memcpy(e.cFileName,pin->name.c_str(),(pin->name.size()+1)*sizeof(wchar_t));}}
            else *static_cast<DWORD*>(bytes)=DROPEFFECT_COPY;
            GlobalUnlock(memory);out->tymed=TYMED_HGLOBAL;out->hGlobal=memory;return S_OK;
        }catch(...){owner->failed=true;return E_OUTOFMEMORY;}
    }
    HRESULT STDMETHODCALLTYPE GetDataHere(FORMATETC*,STGMEDIUM*) noexcept override{return DV_E_FORMATETC;}
    HRESULT STDMETHODCALLTYPE GetCanonicalFormatEtc(FORMATETC*,FORMATETC* out) noexcept override{if(out)out->ptd=nullptr;return E_NOTIMPL;}
    HRESULT STDMETHODCALLTYPE SetData(FORMATETC*,STGMEDIUM*,BOOL) noexcept override{return E_NOTIMPL;}
    HRESULT STDMETHODCALLTYPE EnumFormatEtc(DWORD direction,IEnumFORMATETC** out) noexcept override{if(!out)return E_POINTER;*out=nullptr;if(direction!=DATADIR_GET)return E_NOTIMPL;try{std::vector<FORMATETC> list{{descriptor,nullptr,DVASPECT_CONTENT,-1,TYMED_HGLOBAL},{effect,nullptr,DVASPECT_CONTENT,-1,TYMED_HGLOBAL}};for(size_t i=0;i<owner->published.size();++i)if(!owner->published[i]->directory)list.push_back(FORMATETC{contents,nullptr,DVASPECT_CONTENT,static_cast<LONG>(i),TYMED_ISTREAM});*out=new Enumerator(std::move(list));return S_OK;}catch(...){owner->failed=true;return E_OUTOFMEMORY;}}
    HRESULT STDMETHODCALLTYPE DAdvise(FORMATETC*,DWORD,IAdviseSink*,DWORD*) noexcept override{return OLE_E_ADVISENOTSUPPORTED;}
    HRESULT STDMETHODCALLTYPE DUnadvise(DWORD) noexcept override{return OLE_E_ADVISENOTSUPPORTED;}
    HRESULT STDMETHODCALLTYPE EnumDAdvise(IEnumSTATDATA**) noexcept override{return OLE_E_ADVISENOTSUPPORTED;}
    HRESULT STDMETHODCALLTYPE SetAsyncMode(BOOL value) noexcept override{Call active(static_cast<IDataObject*>(this),owner);if(!active.valid&&value)return E_FAIL;if(operations&&!value)return E_UNEXPECTED;async=value!=FALSE;if(async&&!modeHold){modeHold=true;AddRef();(void)owner->change(0,0,0,1);}if(!async)dropMode();return owner->failed?E_FAIL:S_OK;}
    HRESULT STDMETHODCALLTYPE GetAsyncMode(BOOL* out) noexcept override{if(!out)return E_POINTER;*out=async?TRUE:FALSE;return S_OK;}
    HRESULT STDMETHODCALLTYPE StartOperation(IBindCtx* reserved) noexcept override{Call active(static_cast<IDataObject*>(this),owner);if(!active.valid)return E_FAIL;if(reserved||!async||operations==UINT32_MAX)return E_UNEXPECTED;if(!modeHold){modeHold=true;AddRef();(void)owner->change(0,0,0,1);}++operations;(void)owner->change(0,0,1);return owner->failed?E_FAIL:S_OK;}
    HRESULT STDMETHODCALLTYPE InOperation(BOOL* out) noexcept override{if(!out)return E_POINTER;*out=operations?TRUE:FALSE;return S_OK;}
    HRESULT STDMETHODCALLTYPE EndOperation(HRESULT result,IBindCtx* reserved,DWORD drop) noexcept override{Call active(static_cast<IDataObject*>(this),owner);if(reserved||!operations||(drop&DROPEFFECT_MOVE))return E_UNEXPECTED;if(FAILED(result))owner->failed=true;--operations;(void)owner->change(0,0,-1);if(!operations)dropMode();return owner->failed?E_FAIL:S_OK;}
};

struct Consumer {
    HANDLE ready=nullptr,advance=nullptr;
    std::atomic<unsigned> phase{0};std::atomic<bool> failed{false},done{false};
    std::shared_ptr<Owner> owner;
    std::vector<unsigned char> packet;
    ULONGLONG deadline=0;
    bool stage(unsigned next) noexcept{
        phase=next;const auto now=GetTickCount64();
        if(now>=deadline||!SetEvent(ready)||WaitForSingleObject(advance,static_cast<DWORD>(deadline-now))!=WAIT_OBJECT_0){failed=true;return false;}
        return !owner->failed;
    }
    void run() noexcept{
        IDataObject* object=nullptr;IDataObjectAsyncCapability* async=nullptr;IStream* transport=nullptr;bool initialized=false;
        std::vector<IStream*> streams,clones;
        try{
            if(FAILED(CoInitializeEx(nullptr,COINIT_MULTITHREADED))) {failed=true;done=true;SetEvent(ready);return;}initialized=true;
            HRESULT hr=CreateStreamOnHGlobal(nullptr,TRUE,&transport);ULONG put=0;
            if(SUCCEEDED(hr))hr=transport->Write(packet.data(),static_cast<ULONG>(packet.size()),&put);
            LARGE_INTEGER zero{};
            if(SUCCEEDED(hr)&&put==packet.size())hr=transport->Seek(zero,STREAM_SEEK_SET,nullptr);else hr=E_FAIL;
            if(SUCCEEDED(hr))hr=CoUnmarshalInterface(transport,IID_IDataObject,reinterpret_cast<void**>(&object));
            if(transport){transport->Release();transport=nullptr;}
            if(SUCCEEDED(hr))hr=object->QueryInterface(IID_IDataObjectAsyncCapability,reinterpret_cast<void**>(&async));
            if(SUCCEEDED(hr))hr=async->SetAsyncMode(TRUE);
            if(SUCCEEDED(hr))hr=async->StartOperation(nullptr);
            // Descriptor requests after factory-close must stay valid.
            if(SUCCEEDED(hr)){FORMATETC format{descriptor,nullptr,DVASPECT_CONTENT,-1,TYMED_HGLOBAL};STGMEDIUM medium{};hr=object->GetData(&format,&medium);if(SUCCEEDED(hr)){auto* group=static_cast<FILEGROUPDESCRIPTORW*>(GlobalLock(medium.hGlobal));if(!group||group->cItems!=owner->published.size())hr=E_FAIL;if(group)GlobalUnlock(medium.hGlobal);ReleaseStgMedium(&medium);}}
            for(size_t i=0;SUCCEEDED(hr)&&i<owner->published.size();++i){if(owner->published[i]->directory)continue;FORMATETC f{contents,nullptr,DVASPECT_CONTENT,static_cast<LONG>(i),TYMED_ISTREAM};STGMEDIUM medium{};hr=object->GetData(&f,&medium);if(FAILED(hr))break;IStream* stream=medium.pstm;stream->AddRef();ReleaseStgMedium(&medium);streams.push_back(stream);IStream* clone=nullptr;hr=stream->Clone(&clone);if(SUCCEEDED(hr))clones.push_back(clone);}
            if(FAILED(hr)||streams.empty()||!stage(1)){failed=true;}else{
                object->Release();object=nullptr;
                // Read every file and clone to EOF; independently compare
                // actual bytes to the same immutable, inherited source pin.
                size_t index=0;std::array<unsigned char,4096> observed{},expected{};
                for(const auto& pin:owner->published){if(pin->directory)continue;for(auto* stream:{streams[index],clones[index]}){uint64_t offset=0;for(;;){ULONG got=0,want=0;hr=stream->Read(observed.data(),static_cast<ULONG>(observed.size()),&got);const auto check=pin->read(offset,expected.data(),static_cast<ULONG>(expected.size()),&want);if(FAILED(hr)||FAILED(check)||got!=want||std::memcmp(observed.data(),expected.data(),got)!=0){failed=true;break;}offset+=got;if(got<observed.size())break;}if(offset!=pin->size)failed=true;}++index;}
                if(!failed&&!stage(2))failed=true;
                for(auto* stream:streams)stream->Release();
                streams.clear();
                if(!failed&&!stage(3))failed=true;
                for(auto* clone:clones)clone->Release();
                clones.clear();
                if(!failed&&!stage(4))failed=true;
                if(!failed){hr=async->EndOperation(S_OK,nullptr,DROPEFFECT_COPY);if(FAILED(hr))failed=true;}
            }
        }catch(...){failed=true;}
        // Unexpected cleanup must never turn an error into a witness.
        if(failed)owner->failed=true;
        for(auto* stream:streams)stream->Release();
        for(auto* clone:clones)clone->Release();
        if(async)async->Release();
        if(object)object->Release();
        if(transport)transport->Release();
        if(initialized)CoUninitialize();
        done=true;SetEvent(ready);
    }
};
bool pumpUntil(Consumer& c,unsigned phase) noexcept {
    const auto deadline=c.deadline;
    for(;;){MSG message{};while(PeekMessageW(&message,nullptr,0,0,PM_REMOVE)){if(message.message==WM_QUIT)return false;TranslateMessage(&message);DispatchMessageW(&message);}
        if(c.failed||c.owner->failed)return false;
        if(c.done)return phase==5;
        if(c.phase>=phase)return true;
        if(GetTickCount64()>=deadline)return false;
        if(MsgWaitForMultipleObjects(1,&c.ready,FALSE,10,QS_ALLINPUT)==WAIT_FAILED)return false;
    }
}
}

struct BrokerWitness{unsigned value=1;};
struct BrokerSession{
    std::shared_ptr<Owner> owner;
    std::array<unsigned char,32> binding{};
    BrokerWitness witness;
    bool started=false,taken=false,consumed=false;
    std::atomic<bool> consumerHeld{false};
    ULONGLONG deadline=0;
};
static std::atomic<BrokerSession*> current{nullptr};
// One helper instance cannot mix its controlled and system-publisher contracts.
static std::atomic<unsigned> nativeMode{0};

extern "C" BrokerSession* broker_create(const BrokerEntry* entries,size_t count,const unsigned char* binding,size_t size){
    if(!entries||count==0||count>65||!binding||size!=32)return nullptr;
    unsigned unclaimed=0;if(!nativeMode.compare_exchange_strong(unclaimed,1))return nullptr;
    try{
        auto session=std::make_unique<BrokerSession>();session->owner=std::make_shared<Owner>();
        for(size_t i=0;i<count;++i){const auto& entry=entries[i];if(!entry.handle||!entry.name)return nullptr;
            auto pin=std::make_shared<Pin>();pin->handle=reinterpret_cast<HANDLE>(entry.handle);pin->name=entry.name;pin->size=entry.size;pin->directory=entry.directory!=0;pin->published=entry.published!=0;
            BY_HANDLE_FILE_INFORMATION info{};if(!GetFileInformationByHandle(pin->handle,&info)||(info.dwFileAttributes&FILE_ATTRIBUTE_REPARSE_POINT)||((info.dwFileAttributes&FILE_ATTRIBUTE_DIRECTORY)!=0)!=pin->directory)return nullptr;
            if(pin->published&&(pin->name.empty()||pin->name.size()>=MAX_PATH||pin->name[0]==L'\\'||pin->name.find(L':')!=std::wstring::npos))return nullptr;
            session->owner->all.push_back(pin);if(pin->published)session->owner->published.push_back(pin);
        }
        if(session->owner->published.empty())return nullptr;
        std::copy(binding,binding+size,session->binding.begin());auto* result=session.release();current=result;return result;
    }catch(...){return nullptr;}
}

extern "C" int broker_run(BrokerSession* session,uintptr_t launcherRaw,uint32_t remainingMilliseconds,int waitExit,BrokerResult* out){
    if(!session||current!=session||!out||session->started||remainingMilliseconds==0||remainingMilliseconds>30000)return 0;
    session->started=true;*out={};
    session->deadline=GetTickCount64()+remainingMilliseconds;
    IDataObject* object=nullptr;IStream* marshal=nullptr;bool initialized=false;Consumer consumer;std::thread worker;
    try{
        auto owner=session->owner;consumer.owner=owner;consumer.deadline=session->deadline;
        if(FAILED(OleInitialize(nullptr)))return 0;
        initialized=true;
        object=new(std::nothrow) Data(owner);if(!object||owner->failed)goto cleanup;
        owner->closeFactory();if(!owner->protect(0))goto cleanup;
        if(FAILED(CreateStreamOnHGlobal(nullptr,TRUE,&marshal)))goto cleanup;
        if(FAILED(CoMarshalInterface(marshal,IID_IDataObject,object,MSHCTX_INPROC,nullptr,MSHLFLAGS_TABLESTRONG)))goto cleanup;
        object->Release();object=nullptr;if(!owner->protect(1))goto cleanup;
        {STATSTG stat{};if(FAILED(marshal->Stat(&stat,STATFLAG_NONAME))||stat.cbSize.QuadPart==0||stat.cbSize.QuadPart>65536)goto cleanup;
            consumer.packet.resize(static_cast<size_t>(stat.cbSize.QuadPart));LARGE_INTEGER zero{};ULONG got=0;
            if(FAILED(marshal->Seek(zero,STREAM_SEEK_SET,nullptr))||FAILED(marshal->Read(consumer.packet.data(),static_cast<ULONG>(consumer.packet.size()),&got))||got!=consumer.packet.size())goto cleanup;}
        consumer.ready=CreateEventW(nullptr,FALSE,FALSE,nullptr);consumer.advance=CreateEventW(nullptr,FALSE,FALSE,nullptr);if(!consumer.ready||!consumer.advance)goto cleanup;
        worker=std::thread([&consumer](){consumer.run();});
        if(!pumpUntil(consumer,1)||!owner->protect(2))goto cleanup;
        {LARGE_INTEGER zero{};if(FAILED(marshal->Seek(zero,STREAM_SEEK_SET,nullptr))||FAILED(CoReleaseMarshalData(marshal)))goto cleanup;marshal->Release();marshal=nullptr;}
        if(waitExit){
            // Existing real consumer/provider/streams remain usable. The
            // only release gate is the original stable launcher HANDLE's
            // actual successful process exit, within the original deadline.
            HANDLE launcher=reinterpret_cast<HANDLE>(launcherRaw);
            if(!launcher||GetProcessId(launcher)==0||GetProcessId(launcher)==GetCurrentProcessId())goto cleanup;
            session->consumerHeld=true;
            for(;;){
                if(GetTickCount64()>=session->deadline||consumer.failed||owner->failed)goto cleanup;
                const auto state=WaitForSingleObject(launcher,0);
                if(state==WAIT_OBJECT_0){DWORD exit=STILL_ACTIVE;if(!GetExitCodeProcess(launcher,&exit)||exit!=0)goto cleanup;break;}
                if(state!=WAIT_TIMEOUT)goto cleanup;
                MSG message{};while(PeekMessageW(&message,nullptr,0,0,PM_REMOVE)){if(message.message==WM_QUIT)goto cleanup;TranslateMessage(&message);DispatchMessageW(&message);}
                if(MsgWaitForMultipleObjects(1,&launcher,FALSE,10,QS_ALLINPUT)==WAIT_FAILED)goto cleanup;
            }
        }
        if(!SetEvent(consumer.advance)||!pumpUntil(consumer,2)||!owner->protect(3))goto cleanup;
        if(!SetEvent(consumer.advance)||!pumpUntil(consumer,3)||!owner->protect(4))goto cleanup;
        if(!SetEvent(consumer.advance)||!pumpUntil(consumer,4)||!owner->protect(5))goto cleanup;
        if(!SetEvent(consumer.advance)||!pumpUntil(consumer,5))goto cleanup;
        worker.join();
        if(!owner->eligible()||owner->checks!=0x7f)goto cleanup;
        out->protected_checks=owner->checks;out->sources_closed=owner->closed;
        CloseHandle(consumer.ready);CloseHandle(consumer.advance);OleUninitialize();return 1;
    }catch(...){session->owner->failed=true;}
cleanup:
    session->owner->failed=true;
    if(consumer.advance)SetEvent(consumer.advance);
    if(worker.joinable()){
        // Pump the STA while the failing MTA releases standard COM proxies.
        const auto deadline=GetTickCount64()+16000;while(!consumer.done&&GetTickCount64()<deadline){MSG msg{};while(PeekMessageW(&msg,nullptr,0,0,PM_REMOVE)){TranslateMessage(&msg);DispatchMessageW(&msg);}Sleep(1);}worker.join();
    }
    if(marshal){LARGE_INTEGER zero{};(void)marshal->Seek(zero,STREAM_SEEK_SET,nullptr);(void)CoReleaseMarshalData(marshal);marshal->Release();}
    if(object)object->Release();
    if(consumer.ready)CloseHandle(consumer.ready);
    if(consumer.advance)CloseHandle(consumer.advance);
    if(initialized)OleUninitialize();
    return 0;
}
extern "C" BrokerWitness* broker_take(BrokerSession* session,const unsigned char* binding,size_t size){
    if(!session||current!=session||!binding||size!=32)return nullptr;
    if(GetTickCount64()>=session->deadline||!std::equal(session->binding.begin(),session->binding.end(),binding)||session->taken||session->consumed||!session->owner->eligible())return nullptr;
    session->taken=true;return &session->witness;
}
extern "C" int broker_consume(BrokerSession* session,BrokerWitness* witness,const unsigned char* binding,size_t size){
    if(!session||current!=session||!binding||size!=32)return 0;
    if(GetTickCount64()>=session->deadline||witness!=&session->witness||!session->taken||session->consumed||!std::equal(session->binding.begin(),session->binding.end(),binding)||!session->owner->eligible())return 0;
    session->consumed=true;return 1;
}
extern "C" void broker_destroy(BrokerSession* session){if(session&&current==session){current=nullptr;delete session;}}
extern "C" int broker_consumer_held(BrokerSession* session){
    return session&&current==session&&session->consumerHeld&&!session->owner->failed&&!session->owner->eligible()?1:0;
}
extern "C" void broker_abort(BrokerSession* session){if(session&&current==session)session->owner->failed=true;}

// This independent constructor is deliberately not called by the existing Go
// receipt-only entry point. A future verified v3 handoff owns authorization;
// these primitives do not accept or generate a signing request.
struct ShellWitness { unsigned value=1; };
struct ShellSession {
    std::shared_ptr<Owner> owner;
    std::array<unsigned char,32> binding{};
    ShellWitness witness;
    bool taken=false,consumed=false;
    std::atomic<bool> started{false},exportPossible{false},published{false};
    std::atomic<bool> admissionClosed{false},finished{false};
    std::atomic<uint32_t> generation{0};
    std::atomic<int32_t> publishResult{static_cast<int32_t>(E_PENDING)};
};

namespace {
std::mutex shellRegistry;
ShellSession* currentShell=nullptr;
constexpr size_t shellMaxSources=20003,shellMaxPublished=10000;
constexpr size_t shellMaxNameUnits=4u*1024u*1024u;

struct WindowsNameLess {
    bool operator()(const std::wstring& a,const std::wstring& b) const {
        const auto result=CompareStringOrdinal(a.data(),static_cast<int>(a.size()),b.data(),static_cast<int>(b.size()),TRUE);
        if(!result)throw std::runtime_error("native name comparison failed");
        return result==CSTR_LESS_THAN;
    }
};

bool shellRelativeName(const std::wstring& name,bool published) noexcept {
    if(name.empty()||name.size()>32767)return false;
    if(name==L".")return !published;
    if(published&&name.size()>=MAX_PATH)return false;
    size_t start=0;
    for(size_t i=0;i<=name.size();++i){
        if(i<name.size()){
            const auto c=static_cast<uint16_t>(name[i]);
            if(c<32||c==127||c==L'/'||c==L':'||c==L'<'||c==L'>'||c==L'"'||c==L'|'||c==L'?'||c==L'*')return false;
            if(c>=0xd800&&c<=0xdbff){
                if(i+1>=name.size())return false;
                const auto low=static_cast<uint16_t>(name[i+1]);
                if(low<0xdc00||low>0xdfff)return false;
                ++i;continue;
            }
            if(c>=0xdc00&&c<=0xdfff)return false;
            if(c!=L'\\')continue;
        }
        const auto length=i-start;
        if(!length||(length==1&&name[start]==L'.')||(length==2&&name[start]==L'.'&&name[start+1]==L'.')||name[i-1]==L'.'||name[i-1]==L' ')return false;
        start=i+1;
    }
    return true;
}

bool shellSettled(const std::shared_ptr<Owner>& owner) noexcept {
    try{
        std::lock_guard<std::mutex> lock(owner->mutex);
        return owner->settled&&!owner->closing&&owner->providers==0&&owner->streams==0&&owner->operations==0&&owner->modes==0&&owner->calls==0;
    }catch(...){owner->failed=true;return false;}
}

void shellPump(const std::shared_ptr<Owner>& owner) noexcept {
    try{
        // A queued WM_QUIT is a failure observation, never permission to kill
        // an exported object. Bound each batch so ownership is still observed.
        MSG message{};
        for(unsigned n=0;n<256&&PeekMessageW(&message,nullptr,0,0,PM_REMOVE);++n){
            if(message.message==WM_QUIT){owner->failed=true;continue;}
            TranslateMessage(&message);DispatchMessageW(&message);
        }
        if(MsgWaitForMultipleObjectsEx(0,nullptr,25,QS_ALLINPUT,MWMO_INPUTAVAILABLE)==WAIT_FAILED){owner->failed=true;Sleep(10);}
    }catch(...){owner->failed=true;Sleep(10);}
}
}

extern "C" ShellSession* shell_create(const BrokerEntry* entries,size_t count,const unsigned char* binding,size_t size){
    if(!entries||count==0||count>shellMaxSources||!binding||size!=32)return nullptr;
    unsigned unclaimed=0;if(!nativeMode.compare_exchange_strong(unclaimed,2))return nullptr;
    try{
        auto session=std::make_unique<ShellSession>();session->owner=std::make_shared<Owner>();
        std::set<uintptr_t> handles;
        std::set<std::wstring,WindowsNameLess> names;
        std::set<std::wstring,WindowsNameLess> publishedDirectories;
        size_t nameUnits=0;
        bool rootSeen=false;
        for(size_t i=0;i<count;++i){
            const auto& entry=entries[i];
            if(!entry.handle||entry.handle==reinterpret_cast<uintptr_t>(INVALID_HANDLE_VALUE)||!entry.name||!handles.insert(entry.handle).second||(entry.directory!=0&&entry.directory!=1)||(entry.published!=0&&entry.published!=1))return nullptr;
            const auto length=wcsnlen(entry.name,32768);
            if(length>=32768||length>shellMaxNameUnits-nameUnits)return nullptr;
            nameUnits+=length;
            std::wstring name(entry.name,length);
            if(!shellRelativeName(name,entry.published!=0)||!names.insert(name).second)return nullptr;
            if(name==L"."){if(!entry.directory||entry.published||rootSeen)return nullptr;rootSeen=true;}
            auto pin=std::make_shared<Pin>();
            const DWORD access=FILE_READ_ATTRIBUTES|SYNCHRONIZE|(entry.directory?0:FILE_READ_DATA);
            if(!DuplicateHandle(GetCurrentProcess(),reinterpret_cast<HANDLE>(entry.handle),GetCurrentProcess(),&pin->handle,access,FALSE,0))return nullptr;
            BY_HANDLE_FILE_INFORMATION info{};
            if(GetFileType(pin->handle)!=FILE_TYPE_DISK||!GetFileInformationByHandle(pin->handle,&info)||(info.dwFileAttributes&FILE_ATTRIBUTE_REPARSE_POINT)||((info.dwFileAttributes&FILE_ATTRIBUTE_DIRECTORY)!=0)!=(entry.directory!=0))return nullptr;
            const auto actualSize=(static_cast<uint64_t>(info.nFileSizeHigh)<<32)|info.nFileSizeLow;
            if((entry.directory&&entry.size!=0)||(!entry.directory&&(actualSize!=entry.size||actualSize>static_cast<uint64_t>(MAXLONGLONG))))return nullptr;
            pin->name=std::move(name);pin->size=entry.size;pin->directory=entry.directory!=0;pin->published=entry.published!=0;
            session->owner->all.push_back(pin);
            if(pin->published){
                if(session->owner->published.size()>=shellMaxPublished)return nullptr;
                session->owner->published.push_back(pin);
                if(pin->directory)publishedDirectories.insert(pin->name);
            }
        }
        if(!rootSeen||session->owner->published.empty())return nullptr;
        for(const auto& pin:session->owner->published){
            auto at=pin->name.find(L'\\');
            while(at!=std::wstring::npos){
                if(publishedDirectories.find(pin->name.substr(0,at))==publishedDirectories.end())return nullptr;
                at=pin->name.find(L'\\',at+1);
            }
        }
        std::copy(binding,binding+size,session->binding.begin());
        std::lock_guard<std::mutex> lock(shellRegistry);
        if(currentShell)return nullptr;
        currentShell=session.get();return session.release();
    }catch(...){return nullptr;}
}

extern "C" int shell_run(ShellSession* session,uint32_t expectedGeneration,uint64_t admissionRemainingMs){
    try{
        std::lock_guard<std::mutex> lock(shellRegistry);
        if(!session||currentShell!=session||session->started)return 0;
        session->started=true;
    }catch(...){return 0;}
    const auto owner=session->owner;
    IDataObject* control=nullptr;
    bool initialized=false;
    const auto started=GetTickCount64();
    const auto deadline=started+admissionRemainingMs;
    try{
        const auto admission=[&]() -> HRESULT {
            if(!expectedGeneration||!admissionRemainingMs||deadline<started||GetTickCount64()>=deadline)return E_INVALIDARG;
            const auto ole=OleInitialize(nullptr);
            if(FAILED(ole))return ole;
            initialized=true;
            if(GetClipboardSequenceNumber()!=expectedGeneration)return HRESULT_FROM_WIN32(ERROR_RETRY);
            const Formats formats{
                static_cast<CLIPFORMAT>(RegisterClipboardFormatW(CFSTR_FILEDESCRIPTORW)),
                static_cast<CLIPFORMAT>(RegisterClipboardFormatW(CFSTR_FILECONTENTS)),
                static_cast<CLIPFORMAT>(RegisterClipboardFormatW(CFSTR_PREFERREDDROPEFFECT))};
            if(!formats.descriptor||!formats.contents||!formats.effect||formats.descriptor==formats.contents||formats.descriptor==formats.effect||formats.contents==formats.effect)return E_FAIL;
            control=new(std::nothrow) Data(owner,formats);
            if(!control||owner->failed)return E_OUTOFMEMORY;
            owner->closeFactory();
            if(owner->failed||GetTickCount64()>=deadline||GetClipboardSequenceNumber()!=expectedGeneration)return HRESULT_FROM_WIN32(ERROR_RETRY);
            // Irreversible BEFORE the call: even a failed or unacknowledged
            // OleSetClipboard may have exported a live provider/marshal ref.
            session->exportPossible=true;
            return OleSetClipboard(control);
        };
        const auto result=admission();session->publishResult=static_cast<int32_t>(result);
        if(FAILED(result))owner->failed=true;
        if(session->exportPossible&&SUCCEEDED(result)){
            // Only an observed stable sequence belonging to this exact object
            // is an ack. A later unrelated copy must never supply its number.
            const auto generation=GetClipboardSequenceNumber();
            if(generation&&generation!=expectedGeneration&&OleIsCurrentClipboard(control)==S_OK&&GetClipboardSequenceNumber()==generation&&OleIsCurrentClipboard(control)==S_OK){
                session->generation=generation;
                session->published=true;
            }
        }
    }catch(...){owner->failed=true;session->publishResult=static_cast<int32_t>(E_FAIL);}
    if(!session->exportPossible){
        owner->failed=true;owner->closeFactory();
        if(control){control->Release();control=nullptr;}
        session->admissionClosed=true;
        if(initialized)OleUninitialize();
        session->finished=true;return 0;
    }
    // No deadline, IPC/launcher test, abort or kill path exists below here.
    // OLE still owns the STA; do not uninitialize it while a consumer can call.
    for(;;){
        if(control&&OleIsCurrentClipboard(control)==S_FALSE){
            session->admissionClosed=true;
            control->Release();control=nullptr;
        }
        if(!control&&shellSettled(owner)){
            const int result=owner->eligible()?1:0;
            if(initialized)OleUninitialize();
            session->finished=true;return result;
        }
        shellPump(owner);
    }
}

extern "C" int shell_status(ShellSession* session,ShellStatus* out){
    if(!out)return 0;
    *out={};
    try{
        std::lock_guard<std::mutex> lock(shellRegistry);
        if(!session||currentShell!=session)return 0;
        out->version=1;out->export_possible=session->exportPossible;out->published=session->published;out->generation=session->generation;
        out->admission_closed=session->admissionClosed;out->failed=session->owner->failed;out->finished=session->finished;
        out->sources_closed=session->owner->closed;out->quiescent=session->owner->eligible();out->publish_hresult=session->publishResult;
        return 1;
    }catch(...){return 0;}
}

extern "C" ShellWitness* shell_take(ShellSession* session,const unsigned char* binding,size_t size){
    if(!binding||size!=32)return nullptr;
    try{
        std::lock_guard<std::mutex> lock(shellRegistry);
        if(!session||currentShell!=session||!session->finished||!session->exportPossible||session->taken||session->consumed||!std::equal(session->binding.begin(),session->binding.end(),binding)||!session->owner->eligible())return nullptr;
        session->taken=true;return &session->witness;
    }catch(...){return nullptr;}
}

extern "C" int shell_consume(ShellSession* session,ShellWitness* witness,const unsigned char* binding,size_t size){
    if(!binding||size!=32)return 0;
    try{
        std::lock_guard<std::mutex> lock(shellRegistry);
        if(!session||currentShell!=session||!session->finished||witness!=&session->witness||!session->taken||session->consumed||!std::equal(session->binding.begin(),session->binding.end(),binding)||!session->owner->eligible())return 0;
        session->consumed=true;return 1;
    }catch(...){return 0;}
}

extern "C" void shell_destroy(ShellSession* session){
    try{
        std::lock_guard<std::mutex> lock(shellRegistry);
        if(!session||currentShell!=session||(session->started&&!session->finished))return;
        currentShell=nullptr;delete session;
    }catch(...){/* Unknown ownership retains the live session; no force path. */}
}
