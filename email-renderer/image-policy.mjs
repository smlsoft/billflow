export const defaultAllowedImageHostSuffixes = [
  'shopee.co.th',
  'shopee.sg',
  'susercontent.com',
  'lazada.co.th',
  'alicdn.com',
  'slatic.net',
  'lazcdn.com',
]

// These endpoints are transparent email engagement pixels, not marketplace
// product media. They remain blocked; hiding the warning avoids false alarms.
export const defaultSilentBlockedImageHostSuffixes = ['mmstat.com']

export function parseHostSuffixes(value, defaults) {
  const source = String(value ?? '').trim()
  if (source === '') return defaults
  return source
    .split(',')
    .map((item) => item.trim().toLowerCase())
    .filter(Boolean)
}

export function hostMatchesSuffixes(host, suffixes) {
  const normalizedHost = host.trim().toLowerCase()
  return suffixes.some((suffix) => normalizedHost === suffix || normalizedHost.endsWith(`.${suffix}`))
}

export function imageHost(value) {
  try {
    return new URL(value).hostname.toLowerCase()
  } catch {
    return ''
  }
}

export function isAllowedImageURL(value, allowedSuffixes) {
  if (value.startsWith('data:image/')) return true
  try {
    const url = new URL(value)
    return url.protocol === 'https:' && hostMatchesSuffixes(url.hostname, allowedSuffixes)
  } catch {
    return false
  }
}

export function shouldWarnForBlockedImageURL(value, silentBlockedSuffixes) {
  const host = imageHost(value)
  return host === '' || !hostMatchesSuffixes(host, silentBlockedSuffixes)
}
