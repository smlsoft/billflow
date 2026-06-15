type PartyLike = {
  code?: string | null
  name?: string | null
}

const TT_TOKEN_RE = /^TT[A-Z0-9]+/i

export function deriveTTPrintPaymentMethod(party?: PartyLike | null): string {
  return firstTTToken(party?.code) || firstTTToken(party?.name)
}

export function paymentMethodMatchesPrefixes(method: string, prefixes: string[]): boolean {
  const normalized = method.trim().toUpperCase()
  if (!normalized) return false
  const cleanPrefixes = prefixes.map((prefix) => prefix.trim().toUpperCase()).filter(Boolean)
  const effectivePrefixes = cleanPrefixes.length > 0 ? cleanPrefixes : ['TT']
  return effectivePrefixes.some((prefix) => normalized.startsWith(prefix))
}

export function isPrintPaymentMethodAllowed(
  method: string,
  allowedMethods: string[],
  prefixEnabled: boolean,
  prefixes: string[],
): boolean {
  const clean = method.trim()
  if (!clean) return true
  if (allowedMethods.includes(clean)) return true
  return prefixEnabled && paymentMethodMatchesPrefixes(clean, prefixes)
}

export function withDerivedPaymentMethodOption(options: string[], derivedMethod: string): string[] {
  const cleanDerived = derivedMethod.trim()
  if (!cleanDerived || options.includes(cleanDerived)) return options
  return [cleanDerived, ...options]
}

function firstTTToken(value?: string | null): string {
  const clean = (value ?? '').trim()
  const match = clean.match(TT_TOKEN_RE)
  return match ? match[0].toUpperCase() : ''
}
