#ifndef LINKSEND_PRIVATE_NATIVE_BROKER_H
#define LINKSEND_PRIVATE_NATIVE_BROKER_H
#include <stddef.h>
#include <stdint.h>
#include <wchar.h>
#ifdef __cplusplus
extern "C" {
#endif
typedef struct BrokerSession BrokerSession;
typedef struct BrokerWitness BrokerWitness;
typedef struct { uintptr_t handle; const wchar_t* name; uint64_t size; int directory; int published; } BrokerEntry;
typedef struct { uint32_t protected_checks; uint32_t sources_closed; } BrokerResult;
BrokerSession* broker_create(const BrokerEntry*,size_t,const unsigned char*,size_t);
int broker_run(BrokerSession*,uintptr_t,uint32_t,int,BrokerResult*);
int broker_consumer_held(BrokerSession*);
void broker_abort(BrokerSession*);
BrokerWitness* broker_take(BrokerSession*,const unsigned char*,size_t);
int broker_consume(BrokerSession*,BrokerWitness*,const unsigned char*,size_t);
void broker_destroy(BrokerSession*);

/* Separate, not-yet-wired system publisher. The receipt-only broker never
 * invokes this API. No function accepts a path, caller reference count, trace
 * or arbitrary signing payload.
 *
 * shell_create borrows all offered handles. It duplicates only native read
 * access into its own pins; success AND failure leave every caller handle
 * owned by the caller. The caller must close its originals after this call.
 * The constructor does not initialize OLE, register formats or publish.
 */
typedef struct ShellSession ShellSession;
typedef struct ShellWitness ShellWitness;
typedef struct {
    uint32_t version;
    uint32_t export_possible;
    uint32_t published;
    uint32_t generation;
    uint32_t admission_closed;
    uint32_t failed;
    uint32_t finished;
    uint32_t sources_closed;
    uint32_t quiescent;
    int32_t publish_hresult;
} ShellStatus;
ShellSession* shell_create(const BrokerEntry*,size_t,const unsigned char*,size_t);
/* Run exactly once on the caller's locked STA. expected_generation is checked
 * immediately before mutation. admission_remaining_ms limits preparation and
 * first mutation only, with overflow rejection; it is never a source TTL.
 * Once export may occur this call has no deadline/cancel/abort input. It pumps
 * until clipboard ownership is definitely lost AND all real native references
 * end. Unknown ownership/async references keep the helper alive indefinitely.
 */
int shell_run(ShellSession*,uint32_t expected_generation,uint64_t admission_remaining_ms);
int shell_status(ShellSession*,ShellStatus*);
ShellWitness* shell_take(ShellSession*,const unsigned char*,size_t);
int shell_consume(ShellSession*,ShellWitness*,const unsigned char*,size_t);
/* Active destroy requests are ignored. A session can be destroyed only before
 * run starts or after it finishes; it never forcibly releases live consumers.
 */
void shell_destroy(ShellSession*);
#ifdef __cplusplus
}
#endif
#endif
