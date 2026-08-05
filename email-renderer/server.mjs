import http from 'node:http'
import { chromium } from 'playwright'

const port = Number.parseInt(process.env.PORT ?? '8080', 10)
const token = String(process.env.EMAIL_PDF_RENDERER_TOKEN ?? '').trim()
const maxHTMLBytes = 10 * 1024 * 1024
const maxPDFBytes = 20 * 1024 * 1024
const allowedSuffixes = String(process.env.EMAIL_PDF_ALLOWED_IMAGE_HOST_SUFFIXES ?? 'shopee.co.th,shopee.sg,susercontent.com,lazada.co.th,alicdn.com,slatic.net,lazcdn.com')
  .split(',')
  .map((value) => value.trim().toLowerCase())
  .filter(Boolean)

function json(res, status, body) {
  const data = Buffer.from(JSON.stringify(body))
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8', 'Content-Length': data.length })
  res.end(data)
}

function isAuthorized(req) {
  return token !== '' && req.headers['x-billflow-renderer-token'] === token
}

function isAllowedImageURL(value) {
  if (value.startsWith('data:image/')) return true
  try {
    const url = new URL(value)
    if (url.protocol !== 'https:') return false
    const host = url.hostname.toLowerCase()
    return allowedSuffixes.some((suffix) => host === suffix || host.endsWith(`.${suffix}`))
  } catch {
    return false
  }
}

function warningHeader(warnings) {
  return encodeURIComponent(JSON.stringify(warnings.slice(0, 10)))
}

async function renderPDF(html) {
  const blockedHosts = new Set()
  const failedHosts = new Set()
  const browser = await chromium.launch({ headless: true })
  try {
    const context = await browser.newContext({ javaScriptEnabled: false, viewport: { width: 1080, height: 1440 } })
    const page = await context.newPage()
    await page.emulateMedia({ media: 'screen' })
    await page.route('**/*', async (route) => {
      const request = route.request()
      if (request.resourceType() !== 'image') {
        await route.abort('blockedbyclient')
        return
      }
      if (!isAllowedImageURL(request.url())) {
        try { blockedHosts.add(new URL(request.url()).hostname) } catch { blockedHosts.add('รูปที่มี URL ไม่ถูกต้อง') }
        await route.abort('blockedbyclient')
        return
      }
      await route.continue()
    })
    page.on('requestfailed', (request) => {
      if (request.resourceType() !== 'image') return
      try { failedHosts.add(new URL(request.url()).hostname) } catch { failedHosts.add('รูปที่โหลดไม่สำเร็จ') }
    })
    await page.setContent(html, { waitUntil: 'domcontentloaded', timeout: 20000 })
    await page.waitForFunction(() => Array.from(document.images).every((image) => image.complete), undefined, { timeout: 10000 }).catch(() => {})
    const broken = await page.evaluate(() => Array.from(document.images)
      .filter((image) => image.src && image.complete && image.naturalWidth === 0)
      .map((image) => image.src))
    for (const source of broken) {
      try { failedHosts.add(new URL(source).hostname) } catch { failedHosts.add('รูปที่โหลดไม่สำเร็จ') }
    }
    const pdf = await page.pdf({
      format: 'A4',
      printBackground: true,
      preferCSSPageSize: true,
      margin: { top: '10mm', right: '10mm', bottom: '10mm', left: '10mm' },
    })
    if (pdf.length === 0 || pdf.length > maxPDFBytes) throw new Error('ขนาด PDF เกินขอบเขตที่อนุญาต')
    const warnings = [
      ...Array.from(blockedHosts).sort().map((host) => `ไม่ได้ฝังรูปจาก ${host} เพราะไม่อยู่ในรายชื่อโดเมน marketplace ที่อนุญาต`),
      ...Array.from(failedHosts).sort().map((host) => `โหลดรูปจาก ${host} ไม่สำเร็จ`),
    ]
    return { pdf, warnings }
  } finally {
    await browser.close()
  }
}

const server = http.createServer((req, res) => {
  if (req.method === 'GET' && req.url === '/health') {
    return json(res, 200, { status: 'ok', allowed_image_host_suffixes: allowedSuffixes })
  }
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
      const result = await renderPDF(body.html)
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
