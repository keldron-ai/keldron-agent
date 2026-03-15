package config

// resetPostLoadHooks clears all registered post-load hooks.
// Intended for tests only to ensure isolation between test cases.
func resetPostLoadHooks() {
	postLoadHooksMu.Lock()
	defer postLoadHooksMu.Unlock()
	postLoadHooks = nil
}
