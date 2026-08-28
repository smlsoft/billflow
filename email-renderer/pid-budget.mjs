import { readFile } from 'node:fs/promises'

const currentPath = '/sys/fs/cgroup/pids.current'
const maxPath = '/sys/fs/cgroup/pids.max'

export function parsePIDBudget(currentRaw, maxRaw) {
  const current = Number.parseInt(String(currentRaw).trim(), 10)
  const max = Number.parseInt(String(maxRaw).trim(), 10)
  if (!Number.isSafeInteger(current) || current < 0 || !Number.isSafeInteger(max) || max <= 0) return null
  return { current, max }
}

export function isPIDBudgetAtRisk(budget) {
  if (!budget) return false
  const free = budget.max - budget.current
  return budget.current * 100 >= budget.max * 80 || free < 16
}

export async function readPIDBudget(read = readFile) {
  try {
    const [current, max] = await Promise.all([
      read(currentPath, 'utf8'),
      read(maxPath, 'utf8'),
    ])
    return parsePIDBudget(current, max)
  } catch {
    // Non-container local development may not expose cgroup PID files.
    return null
  }
}
