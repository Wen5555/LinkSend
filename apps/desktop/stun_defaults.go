package main

import "strings"

const primaryDesktopSTUN = "stun:stun.oooai.de:3478"
const backupDesktopSTUN = "stun:stun.cloudflare.com:3478"

// The effective display and actual connection must use the same policy.
// Keep explicit overrides, and extend only the old built-in single default.
// Reading this policy never rewrites a user's saved preferences.
func desktopSTUNConfig(environment string, saved []string) ([]string, string) {
	if strings.TrimSpace(environment) != "" {
		if urls := normalizedList(strings.Split(environment, ",")); len(urls) != 0 {
			return urls, "环境变量 LINKSEND_STUN"
		}
		return []string{primaryDesktopSTUN, backupDesktopSTUN}, "默认值"
	}
	urls := normalizedList(saved)
	if len(urls) == 0 || len(urls) == 1 && strings.EqualFold(urls[0], primaryDesktopSTUN) {
		return []string{primaryDesktopSTUN, backupDesktopSTUN}, "默认值"
	}
	return urls, "已保存偏好"
}
