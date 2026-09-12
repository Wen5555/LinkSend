//go:build !windows

package identity

func replaceAtomicFile(string, string) (bool, error)        { return false, nil }
func alignAtomicReplaceSource(string, string) (bool, error) { return false, nil }
