export function requiresRendererRestart(error) {
  if (!(error instanceof Error)) return false
  return /\bEAGAIN\b|\bCannot fork\b|\bresource temporarily unavailable\b|Chromium launch timed out|PDF render exceeded hard deadline|สร้าง PDF ใช้เวลานานเกินกำหนด|Playwright (?:context|browser) cleanup timed out/i.test(error.message)
}
