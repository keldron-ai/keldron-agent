#!/bin/bash
# Shared helpers for dev scripts.

# Kill stale keldron-agent processes, excluding the current shell.
cleanup_stale_agent() {
    local stale_pids
    local repo_binary="${PWD}/bin/keldron-agent"
    stale_pids=$(pgrep -f "${repo_binary}" 2>/dev/null || true)
    # Filter out current process tree
    local filtered=""
    for pid in $stale_pids; do
        if [ "$pid" != "$$" ] && [ "$pid" != "${BASHPID:-$$}" ]; then
            filtered="$filtered $pid"
        fi
    done
    filtered=$(echo "$filtered" | xargs)
    if [ -n "$filtered" ]; then
        echo "🧹 Killing stale agent process(es): $filtered"
        kill $filtered 2>/dev/null || true
        sleep 1
    fi
}
