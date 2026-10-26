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
#ifdef __cplusplus
}
#endif
#endif
