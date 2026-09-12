//go:build darwin

package main

/*
#cgo CFLAGS: -mmacosx-version-min=13.0 -x objective-c
#cgo LDFLAGS: -mmacosx-version-min=13.0 -framework IOKit -framework CoreFoundation
#include <stdint.h>
int linksendAcquireSleepAssertion(uint32_t *assertion);
int linksendReleaseSleepAssertion(uint32_t assertion);
*/
import "C"

import "fmt"

func acquireNativeSleepRequest() (func() error, error) {
	var assertion C.uint32_t
	if code := C.linksendAcquireSleepAssertion(&assertion); code != 0 {
		return nil, fmt.Errorf("SLEEP_INHIBITOR_FAILED: IOPMAssertionCreateWithName code=%d", int32(code))
	}
	return func() error {
		if code := C.linksendReleaseSleepAssertion(assertion); code != 0 {
			return fmt.Errorf("SLEEP_INHIBITOR_FAILED: IOPMAssertionRelease code=%d", int32(code))
		}
		return nil
	}, nil
}
