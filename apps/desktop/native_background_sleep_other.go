//go:build !windows && !darwin

package main

func acquireNativeSleepRequest() (func() error, error) {
	return nil, errNativeEntryUnsupported
}
