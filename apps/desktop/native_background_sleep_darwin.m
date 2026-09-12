#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/pwr_mgt/IOPMLib.h>
#include <stdint.h>

int linksendAcquireSleepAssertion(uint32_t *assertion) {
    return IOPMAssertionCreateWithName(kIOPMAssertionTypePreventUserIdleSystemSleep,
        kIOPMAssertionLevelOn, CFSTR("LinkSend active file transfer"), assertion);
}

int linksendReleaseSleepAssertion(uint32_t assertion) {
    return IOPMAssertionRelease(assertion);
}
