// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package config

// resetPostLoadHooks clears all registered post-load hooks.
// Intended for tests only to ensure isolation between test cases.
func resetPostLoadHooks() {
	postLoadHooksMu.Lock()
	defer postLoadHooksMu.Unlock()
	postLoadHooks = nil
}
