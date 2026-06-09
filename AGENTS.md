# AGENTS.md — BillFlow

Use this file as the short Codex index for `/Users/nontawatwongnuk/dev_bos/billflow`. Keep it small; load detailed docs only when needed.

## Product Shape

BillFlow reduces manual bill entry for Thai stores by ingesting LINE, email, marketplace exports, and marketplace purchase flows, then creating reviewed documents in SML ERP.

Primary stack:

- Backend: Go 1.24, Gin, PostgreSQL 16
- Frontend: React + Vite + TypeScript
- Deploy: Docker Compose + Cloudflare Tunnel
- Key integrations: SML ERP, LINE OA, IMAP email, Shopee/Lazada/TikTok imports

## High-Value Docs

- Current handoff and latest release state: `docs/current-state.md`
- Architecture and data flow: `docs/billflow-main-sml-api-architecture.md`
- Overall product flow: `docs/overview.md`
- Deploy/runtime policy: `docs/deploy-instances.md`
- Email and Lazada purchase: `docs/email.md`, `docs/lazada-email-purchase.md`
- Marketplace purchase print/payment: `docs/marketplace-purchase-print-and-payment.md`
- Shopee import/API: `docs/shopee-import.md`, `docs/shopee-open-api-live-cutover.md`
- Bulk SML jobs: `docs/sml-bulk-send-jobs.md`
- Phase 1 user guide and QA: `docs/phase1-guide.md`, `docs/phase1-test-checklist.md`

Read only the docs needed for the current task.

## Critical Runtime Facts

- Main deploy folder on server: `/home/bosscatdog/billflow`; deployed copy is not a git checkout.
- Ports: backend `8090`, frontend `3010`, postgres `5438`.
- SML gateway and customer instance details live in `docs/deploy-instances.md`.
- Runtime secrets, passwords, API keys, LINE tokens, and partner credentials must come from local/deploy secret sources, not tracked docs.

## Critical Product Rules

- Do not create duplicate bills for the same external marketplace order.
- Preserve traceability: source order/email/artifact -> BillFlow bill -> SML document -> audit/log timeline.
- SML document numbers must avoid patterns known to disappear in SML UI; use the existing counter/doc-no services.
- SML Thai payloads require the existing ASCII JSON escaping path; do not replace it with a charset-only fix.
- `channel_defaults` is the source of truth for channel party, endpoint, doc format, WH/shelf, VAT, and print/payment behavior.
- Marketplace purchase flows are review-first before SML send.
- IMAP uses durable dedup via `processed_email_keys`; do not rely only on read/unread state.
- Admin UX must show clear empty, disabled, partial, error, and recovery states.

## Graphify Auto-Lite

Use Graphify as a context map for cross-subsystem work, not as source of truth.

Use Graphify before broad raw searches when work spans backend, frontend, SML routing, marketplace purchase, email, print/payment, and deployment docs.

Skip Graphify for small single-file edits, exact symbol lookups, logs, or test failure triage where `rg` and source reads are faster.

Commands:

```bash
bash scripts/graphify-update.sh
bash scripts/graphify-query.sh "bill print payment method"
bash scripts/graphify-preflight.sh
```

Rules:

- Always open source files before editing.
- If Graphify disagrees with code or docs, code/docs win.
- `graphify-out/` is local-only and must remain untracked.
- Update Graphify manually after flow or architecture changes.
- Do not install Graphify hooks until the manual workflow has proven stable.

## Validation

Run targeted checks based on the touched area:

```bash
cd backend && go test ./...
cd frontend && npm run build
```

`npm run lint` currently exists but no ESLint config is present; treat that as a tooling gap, not a required gate, until config is added.

## Default Working Style

- Use `production-engineering` for code review, refactor, migration, release, and safety work.
- Use `shopee-open-api` with `production-engineering` for Shopee API/OAuth/order/logistics/settlement work.
- Use `impeccable` for frontend UX/UI polish.
- Keep changes narrow and preserve current production behavior unless the task explicitly changes behavior.
