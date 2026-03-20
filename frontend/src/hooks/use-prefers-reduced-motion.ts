import { useSyncExternalStore } from 'react'

let sharedMql: MediaQueryList | null = null

function getMql(): MediaQueryList {
  if (sharedMql === null) {
    sharedMql = window.matchMedia('(prefers-reduced-motion: reduce)')
  }
  return sharedMql
}

function subscribe(onChange: () => void) {
  const mql = getMql()
  mql.addEventListener('change', onChange)
  return () => mql.removeEventListener('change', onChange)
}

function getSnapshot() {
  return getMql().matches
}

function getServerSnapshot() {
  return false
}

export function usePrefersReducedMotion() {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot)
}
