//go:build !darwin

package main

func registerFinderServices(func([]string, string)) (func(), error) {
	return func() {}, errNativeEntryUnsupported
}
