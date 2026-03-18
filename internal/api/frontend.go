// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package api

import "net/http"

// HandleFrontend serves a placeholder for the embedded dashboard (OSS-027).
func HandleFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("Dashboard coming in OSS-027"))
}
