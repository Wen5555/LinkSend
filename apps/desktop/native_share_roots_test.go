package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestNativeShareRootsDoNotResolveSharedContainerForIsolatedProfiles(t *testing.T) {
	defaultProfile := t.TempDir()
	for _, test := range []struct {
		name     string
		profile  string
		explicit bool
	}{
		{"explicit default", defaultProfile, true},
		{"explicit separate", t.TempDir(), true},
		{"separate profile without override", t.TempDir(), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			roots := nativeShareRootsForProfile(test.profile, defaultProfile, test.explicit, func() string {
				calls++
				return filepath.Join(defaultProfile, "shared-container")
			})
			if calls != 0 || !reflect.DeepEqual(roots, []string{test.profile}) {
				t.Fatalf("isolated profile touched shared namespace: calls=%d roots=%v", calls, roots)
			}
		})
	}
}

func TestNativeShareRootsDefaultProfileMayUseAvailableContainer(t *testing.T) {
	profile := t.TempDir()
	container := t.TempDir()
	for _, shared := range []string{"", profile, container} {
		calls := 0
		roots := nativeShareRootsForProfile(profile, profile, false, func() string { calls++; return shared })
		want := []string{profile}
		if shared == container {
			want = append(want, container)
		}
		if calls != 1 || !reflect.DeepEqual(roots, want) {
			t.Fatalf("default profile roots=%v calls=%d want=%v", roots, calls, want)
		}
	}
}
