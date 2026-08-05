import assert from 'node:assert/strict'
import test from 'node:test'
import {
  defaultAllowedImageHostSuffixes,
  defaultSilentBlockedImageHostSuffixes,
  isAllowedImageURL,
  shouldWarnForBlockedImageURL,
} from '../image-policy.mjs'

test('allows approved marketplace product image hosts', () => {
  assert.equal(isAllowedImageURL('https://cf.shopee.sg/file/product-image', defaultAllowedImageHostSuffixes), true)
  assert.equal(isAllowedImageURL('https://img.lazada.co.th/product.jpg', defaultAllowedImageHostSuffixes), true)
})

test('keeps Lazada mmstat tracking pixels blocked without a user-facing warning', () => {
  const trackingPixel = 'https://sg.mmstat.com/lzdmailer.letter.open?mail_id=example'

  assert.equal(isAllowedImageURL(trackingPixel, defaultAllowedImageHostSuffixes), false)
  assert.equal(shouldWarnForBlockedImageURL(trackingPixel, defaultSilentBlockedImageHostSuffixes), false)
})

test('still warns when an unknown image host is blocked', () => {
  assert.equal(
    shouldWarnForBlockedImageURL('https://untrusted.example.invalid/image.jpg', defaultSilentBlockedImageHostSuffixes),
    true,
  )
})
