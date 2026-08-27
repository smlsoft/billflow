import http from 'node:http'
import { chromium } from 'playwright'
import { requiresRendererRestart } from './failure-policy.mjs'
import { createSerialQueue } from './render-queue.mjs'
import { withTimeout } from './with-timeout.mjs'
import {
  defaultAllowedImageHostSuffixes,
  defaultSilentBlockedImageHostSuffixes,
  imageHost,
  isAllowedImageURL,
  parseHostSuffixes,
  shouldWarnForBlockedImageURL,
} from './image-policy.mjs'

const port = Number.parseInt(process.env.PORT ?? '8080', 10)
const token = String(process.env.EMAIL_PDF_RENDERER_TOKEN ?? '').trim()
const maxHTMLBytes = 10 * 1024 * 1024
const maxPDFBytes = 20 * 1024 * 1024
const allowedSuffixes = parseHostSuffixes(
  process.env.EMAIL_PDF_ALLOWED_IMAGE_HOST_SUFFIXES,
  defaultAllowedImageHostSuffixes,
)
const silentBlockedSuffixes = parseHostSuffixes(
  process.env.EMAIL_PDF_SILENT_BLOCKED_IMAGE_HOST_SUFFIXES,
  defaultSilentBlockedImageHostSuffixes,
)
const renderQueue = createSerialQueue()
let activeBrowser = null
let shuttingDown = false

function json(res, status, body) {
  const data = Buffer.from(JSON.stringify(body))
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8', 'Content-Length': data.length })
  res.end(data)
}

function isAuthorized(req) {
  return token !== '' && req.headers['x-billflow-renderer-token'] === token
}

function warningHeader(warnings) {
  return encodeURIComponent(JSON.stringify(warnings.slice(0, 10)))
}

async function renderPDF(html) {
  const blockedHosts = new Set()
  const failedHosts = new Set()
  let browser
  let context
  try {
    browser = await chromium.launch({ headless: true, timeout: 30000 })
    activeBrowser = browser
    context = await browser.newContext({ javaScriptEnabled: false, viewport: { width: 1080, height: 1440 } })
    context.setDefaultTimeout(30000)
    const page = await context.newPage()
    await page.emulateMedia({ media: 'screen' })
    await page.route('**/*', async (route) => {
      const request = route.request()
      if (request.resourceType() !== 'image') {
        await route.abort('blockedbyclient')
        return
      }
      if (!isAllowedImageURL(request.url(), allowedSuffixes)) {
        if (shouldWarnForBlockedImageURL(request.url(), silentBlockedSuffixes)) {
          blockedHosts.add(imageHost(request.url()) || 'รูปที่มี URL ไม่ถูกต้อง')
        }
        await route.abort('blockedbyclient')
        return
      }
      await route.continue()
    })
    page.on('requestfailed', (request) => {
      if (request.resourceType() !== 'image') return
      if (!shouldWarnForBlockedImageURL(request.url(), silentBlockedSuffixes)) return
      failedHosts.add(imageHost(request.url()) || 'รูปที่โหลดไม่สำเร็จ')
    })
    await page.setContent(html, { waitUntil: 'domcontentloaded', timeout: 20000 })
    await page.waitForFunction(() => Array.from(document.images).every((image) => image.complete), undefined, { timeout: 10000 }).catch(() => {})
    const broken = await page.evaluate(() => Array.from(document.images)
      .filter((image) => image.src && image.complete && image.naturalWidth === 0)
      .map((image) => image.src))
    for (const source of broken) {
      if (!shouldWarnForBlockedImageURL(source, silentBlockedSuffixes)) continue
      failedHosts.add(imageHost(source) || 'รูปที่โหลดไม่สำเร็จ')
    }
    const pdf = await withTimeout(page.pdf({
      format: 'A4',
      printBackground: true,
      preferCSSPageSize: true,
      margin: { top: '10mm', right: '10mm', bottom: '10mm', left: '10mm' },
    }), 30000, 'สร้าง PDF ใช้เวลานานเกินกำหนด')
    if (pdf.length === 0 || pdf.length > maxPDFBytes) throw new Error('ขนาด PDF เกินขอบเขตที่อนุญาต')
    const warnings = [
      ...Array.from(blockedHosts).sort().map((host) => `ไม่ได้ฝังรูปจาก ${host} เพราะไม่อยู่ในรายชื่อโดเมน marketplace ที่อนุญาต`),
      ...Array.from(failedHosts).sort().map((host) => `โหลดรูปจาก ${host} ไม่สำเร็จ`),
    ]
    return { pdf, warnings }
  } catch (error) {
    if (requiresRendererRestart(error)) requestRestart('Chromium process limit reached')
    throw error
  } finally {
    // Explicitly close the context before Chromium. A failed cleanup is worse
    // than a retry: retain no process that can consume the renderer PID quota.
    let cleanupFailed = false
    if (context) {
      await withTimeout(context.close(), 5000, 'Playwright context cleanup timed out').catch((error) => {
        cleanupFailed = true
        console.error('Failed to close Playwright context:', error)
      })
    }
    if (browser) {
      await withTimeout(browser.close(), 5000, 'Playwright browser cleanup timed out').catch((error) => {
        cleanupFailed = true
        console.error('Failed to close Playwright browser:', error)
      })
    }
    if (activeBrowser === browser) activeBrowser = null
    if (cleanupFailed) requestRestart('Playwright cleanup failed')
  }
}

function requestRestart(reason) {
  if (shuttingDown) return
  shuttingDown = true
  console.error(`Restarting PDF renderer: ${reason}`)
  setTimeout(() => process.exit(1), 100).unref()
}

const server = http.createServer((req, res) => {
  if (req.method === 'GET' && req.url === '/health') {
    if (shuttingDown) return json(res, 503, { status: 'restarting' })
    return json(res, 200, {
      status: 'ok',
      allowed_image_host_suffixes: allowedSuffixes,
      silent_blocked_image_host_suffixes: silentBlockedSuffixes,
      render_queue: renderQueue.status(),
    })
  }
  if (shuttingDown) return json(res, 503, { error: 'PDF renderer is restarting' })
  if (req.method !== 'POST' || req.url !== '/v1/render') return json(res, 404, { error: 'not found' })
  if (!isAuthorized(req)) return json(res, 401, { error: 'unauthorized' })
  let size = 0
  let tooLarge = false
  const chunks = []
  req.on('data', (chunk) => {
    size += chunk.length
    if (size > maxHTMLBytes + 1024 * 1024) {
      tooLarge = true
      return
    }
    chunks.push(chunk)
  })
  req.on('error', () => json(res, 400, { error: 'อ่านข้อมูลไม่สำเร็จ' }))
  req.on('end', async () => {
    try {
      if (tooLarge) return json(res, 413, { error: 'ข้อมูล HTML มีขนาดใหญ่เกินกำหนด' })
      const body = JSON.parse(Buffer.concat(chunks).toString('utf8'))
      if (typeof body.html !== 'string' || body.html.trim() === '') return json(res, 400, { error: 'ไม่พบ HTML สำหรับสร้าง PDF' })
      if (Buffer.byteLength(body.html, 'utf8') > maxHTMLBytes) return json(res, 413, { error: 'ข้อมูล HTML มีขนาดใหญ่เกินกำหนด' })
      const result = await renderQueue.run(() => renderPDF(body.html))
      res.writeHead(200, {
        'Content-Type': 'application/pdf',
        'Content-Length': result.pdf.length,
        'X-BillFlow-Render-Warnings': warningHeader(result.warnings),
      })
      res.end(result.pdf)
    } catch (error) {
      json(res, 422, { error: error instanceof Error ? error.message : 'สร้าง PDF ไม่สำเร็จ' })
    }
  })
})

server.requestTimeout = 70000
server.headersTimeout = 75000
server.listen(port, '0.0.0.0', () => console.log(`BillFlow email renderer listening on ${port}`))

async function shutdown(signal) {
  if (shuttingDown) return
  shuttingDown = true
  console.log(`Stopping PDF renderer after ${signal}`)
  const forceExit = setTimeout(() => process.exit(1), 10000)
  forceExit.unref()
  server.close(() => process.exit(0))
  if (activeBrowser) await activeBrowser.close().catch((error) => console.error('Failed to close browser during shutdown:', error))
}

process.on('SIGTERM', () => { void shutdown('SIGTERM') })
process.on('SIGINT', () => { void shutdown('SIGINT') })
process.on('uncaughtException', (error) => {
  console.error('Uncaught PDF renderer error:', error)
  requestRestart('uncaught exception')
})
process.on('unhandledRejection', (error) => {
  console.error('Unhandled PDF renderer rejection:', error)
  requestRestart('unhandled rejection')
})
