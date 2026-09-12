//go:build !darwin

package main

func registerFinderServices(func([]string, string) error) (func(), error) {
	return func() {}, errNativeEntryUnsupported
}
