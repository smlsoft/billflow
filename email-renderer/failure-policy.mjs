export function requiresRendererRestart(error) {
  if (!(error instanceof Error)) return false
  return /\bEAGAIN\b|\bCannot fork\b|\bresource temporarily unavailable\b/i.test(error.message)
}
