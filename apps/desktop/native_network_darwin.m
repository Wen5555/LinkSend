#include <CoreFoundation/CoreFoundation.h>
#include <SystemConfiguration/SystemConfiguration.h>
#include <pthread.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>

extern void linksendGoNetworkChanged(uintptr_t token);

typedef struct {
    uintptr_t token;
    SCDynamicStoreRef store;
    CFRunLoopSourceRef source;
    CFRunLoopRef runLoop;
    pthread_t thread;
    pthread_mutex_t mutex;
    pthread_cond_t readyCondition;
    bool ready;
    bool active;
} LinkSendNetworkMonitor;

static void linksendNetworkChanged(SCDynamicStoreRef store, CFArrayRef changedKeys, void *info) {
    (void)store;
    (void)changedKeys;
    LinkSendNetworkMonitor *monitor = info;
    pthread_mutex_lock(&monitor->mutex);
    bool active = monitor->active;
    uintptr_t token = monitor->token;
    pthread_mutex_unlock(&monitor->mutex);
    if (active) {
        linksendGoNetworkChanged(token);
    }
}

static void *linksendNetworkMonitorRun(void *opaque) {
    LinkSendNetworkMonitor *monitor = opaque;
    CFRunLoopRef loop = CFRunLoopGetCurrent();
    CFRetain(loop);
    CFRunLoopAddSource(loop, monitor->source, kCFRunLoopDefaultMode);
    pthread_mutex_lock(&monitor->mutex);
    monitor->runLoop = loop;
    monitor->ready = true;
    pthread_cond_signal(&monitor->readyCondition);
    pthread_mutex_unlock(&monitor->mutex);
    CFRunLoopRun();
    CFRunLoopRemoveSource(loop, monitor->source, kCFRunLoopDefaultMode);
    CFRelease(loop);
    return NULL;
}

void *linksendStartNetworkMonitor(uintptr_t token) {
    LinkSendNetworkMonitor *monitor = calloc(1, sizeof(LinkSendNetworkMonitor));
    if (monitor == NULL) {
        return NULL;
    }
    monitor->token = token;
    monitor->active = true;
    pthread_mutex_init(&monitor->mutex, NULL);
    pthread_cond_init(&monitor->readyCondition, NULL);

    SCDynamicStoreContext context = {0, monitor, NULL, NULL, NULL};
    monitor->store = SCDynamicStoreCreate(NULL, CFSTR("LinkSendNetworkMonitor"), linksendNetworkChanged, &context);
    if (monitor->store == NULL) {
        goto fail;
    }
    const void *patterns[] = {
        CFSTR("State:/Network/Global/IPv4"),
        CFSTR("State:/Network/Global/IPv6"),
        CFSTR("State:/Network/Interface/.*/IPv4"),
        CFSTR("State:/Network/Interface/.*/IPv6")
    };
    CFArrayRef patternArray = CFArrayCreate(NULL, patterns, 4, &kCFTypeArrayCallBacks);
    bool notificationsSet = patternArray != NULL && SCDynamicStoreSetNotificationKeys(monitor->store, NULL, patternArray);
    if (patternArray != NULL) {
        CFRelease(patternArray);
    }
    if (!notificationsSet) {
        goto fail;
    }
    monitor->source = SCDynamicStoreCreateRunLoopSource(NULL, monitor->store, 0);
    if (monitor->source == NULL || pthread_create(&monitor->thread, NULL, linksendNetworkMonitorRun, monitor) != 0) {
        goto fail;
    }
    pthread_mutex_lock(&monitor->mutex);
    while (!monitor->ready) {
        pthread_cond_wait(&monitor->readyCondition, &monitor->mutex);
    }
    pthread_mutex_unlock(&monitor->mutex);
    return monitor;

fail:
    if (monitor->source != NULL) CFRelease(monitor->source);
    if (monitor->store != NULL) CFRelease(monitor->store);
    pthread_cond_destroy(&monitor->readyCondition);
    pthread_mutex_destroy(&monitor->mutex);
    free(monitor);
    return NULL;
}

void linksendStopNetworkMonitor(void *opaque) {
    LinkSendNetworkMonitor *monitor = opaque;
    if (monitor == NULL) return;
    pthread_mutex_lock(&monitor->mutex);
    monitor->active = false;
    CFRunLoopRef loop = monitor->runLoop;
    if (loop != NULL) CFRetain(loop);
    pthread_mutex_unlock(&monitor->mutex);
    if (loop != NULL) {
        CFRunLoopStop(loop);
        CFRunLoopWakeUp(loop);
        CFRelease(loop);
    }
    pthread_join(monitor->thread, NULL);
    CFRelease(monitor->source);
    CFRelease(monitor->store);
    pthread_cond_destroy(&monitor->readyCondition);
    pthread_mutex_destroy(&monitor->mutex);
    free(monitor);
}
