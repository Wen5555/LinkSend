package main

// The resolver may enter the system App Group container. Keep it lazy so
// isolated profiles never inspect a shared system-entry namespace.
func nativeShareRootsForProfile(profile, defaultProfile string, explicitProfile bool, containerPath func() string) []string {
	roots := []string{profile}
	if explicitProfile {
		return roots
	}
	canonical, err := canonicalProfilePath(profile)
	if err != nil {
		return roots
	}
	defaultCanonical, err := canonicalProfilePath(defaultProfile)
	if err != nil || canonical != defaultCanonical {
		return roots
	}
	if container := containerPath(); container != "" && container != profile {
		roots = append(roots, container)
	}
	return roots
}
