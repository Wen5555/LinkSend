//go:build !darwin

package main

func nativeShareRoots(profile string) []string { return []string{profile} }

func resolveNativeShareActivation(activation fileActivation) (fileActivation, error) {
	return activation, nil
}

func closeNativeShareAccess() {}
