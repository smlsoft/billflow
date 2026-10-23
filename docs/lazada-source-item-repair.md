# Lazada source item rows — 2026-10-23

Status: deployed to Thaisunsport production and scoped repair applied successfully on 2026-10-23 through user-run AnyDesk commands. Awaiting user visual verification/mapping and feedback. No direct customer database access was used from the development machine.

## Confirmed customer result

- Release checkout `fd80db9`, fix `300ac80`, repair/deployment tooling `08cb2d5`.
- Docker build and real-artifact preflight succeeded. Backend startup initially returned connection resets, then the script confirmed `SUCCESS: backend healthy`.
- Real source rows: SKU `21099144072` and `21099144071`, each quantity 2, unit price 678, allocated discount 144. Goods 2712, coupon 288, shipping 134, paid total 2558.
- Reviewed plan hash: `a557ba82ae5239d86196447f1eb6479cbb2766fd5fbac5752031a48679c4e7bc`.
- Apply output: `applied: source items restored, fee preserved, status=needs_review; map each variant before SML`.
- Backup directory: `/home/thaisunspot/billflow-thaisunsport/backups/lazada-source-items-20261023-104141` (source copy, `database.sql.gz`, and `dry-run.txt`); rollback image: `billflow-lazada-rollback:20261023-104141`.
- Successful apply confirms the repair transaction committed. A subsequent SQL query, refreshed UI screenshot, correct SML color mapping, and SML send have not been supplied; await customer feedback. Do not rerun repair simply because feedback is still pending.

## Report and cause

Order `1124675871588966`, bill `e25e856f-8cee-4765-99cb-5b597c1dc4d3`, has two product cards with the same name and different images: each card shows quantity 2 at THB 678 per unit. The supplied bill JSON instead has one goods row with quantity 2 and unit price 1,356, plus a shipping row at 134.

Lazada extraction sends stripped text to AI, which omits image differences and product-link SKU identity. The saved extraction contains only one goods row; the original AI response was not supplied, so its exact output before post-processing is not confirmed. The deterministic bug is `applyLazadaEmailSummaryPrices`: when only one item remained, it divided the order goods subtotal by that item's quantity (`2712 / 2 = 1356`), hiding the missing row behind a zero money delta.

## Corrected behavior

- Read each HTML product card, preserving occurrences in source order even when titles or SKUs repeat. Retain its SKU, image, line number, quantity, and original unit price; do not infer a color name from the photo.
- Verify that all source quantity blocks were parsed and that `sum(qty * unit_price)` equals the email goods subtotal within 0.01. Recommendations without quantity labels do not become order items.
- If a quantity block is incomplete or the subtotal differs, stop ingestion/rebuild with an error. Never change unit prices to absorb missing items.
- Unsupported templates without quantity blocks keep AI rows for review, mark `source_items_status=unverified` and money reconciliation `missing`, and block SML send. They require template support or source repair before sending; merely mapping a product does not clear this guard.
- Use the same parser for intake, existing email-repair rebuild, repair preview, and the scoped command below. No schema migration or frontend rebuild is needed.
- Existing bills are not automatically reprocessed. Legacy rows without a new source verification marker retain existing send behavior; inspect affected historical bills separately. The scoped repair refuses sent, archived, in-progress, wrong-order, or manually mapped goods.

Expected repair: 2 goods rows, each `qty=2`, `price=678`, `discount=144`, plus the existing shipping row `134`; net remains `2558`.

Local validation: backend `go test ./...`, `go vet ./...`, Linux/CGO-disabled `go build ./...`, deployment script `bash -n`, and `git diff --check`. Regression cases cover duplicate titles/SKUs, missing or collapsed AI rows, nested HTML/split currency cells, recommendations, incomplete quantity blocks, incorrect subtotals, blocked SML send, duplicate follow-up notifications, read-only dry-run, changed-plan refusal, and rollback on audit failure. Docker was unavailable on the development machine; image build/execution was subsequently verified on the customer machine. Graphify was rebuilt locally; its 38 MB graph exceeds the existing 20 MB preflight budget and remains untracked.

## Customer backend deployment and source preflight

Target: Thaisunsport production only, `/home/thaisunspot/billflow-thaisunsport`, backend `8100`. Skip frontend, BillFlow main/demo, SML gateway, and all Nexflow instances.

Run as root on the customer machine from a fresh checkout of the release:

```bash
set -euo pipefail
SOURCE=$(mktemp -d /tmp/billflow-lazada-lines-XXXXXX)
git clone --depth 1 --branch main https://github.com/smlsoft/billflow.git "$SOURCE"
SOURCE_DIR="$SOURCE" \
DEPLOY_DIR=/home/thaisunspot/billflow-thaisunsport \
bash "$SOURCE/scripts/update-customer-lazada-source-items.sh"
```

The script saves the running image under a rollback tag, copies old backend source, dumps PostgreSQL with gzip verification, builds the new backend, and runs a read-only dry-run against this exact bill's matching source HTML before recreating the backend. If source parsing fails, the running container is not replaced; preserve the output and inspect that template. The old source and database dump are under the printed `backups/lazada-source-items-<stamp>/` directory. Successful health verification does not mean the historical bill has been repaired.

The HTML artifact was not supplied locally: tests use representative Lazada HTML and reported quantities/prices. Customer preflight subsequently verified the real artifact and both SKUs listed above. For future repairs, review the real preflight result before applying; if source rows cannot be verified, inspect the original HTML and diagnostics rather than manually inflating prices or guessing SKUs.

## Scoped repair

Dry-run is a read-only SQL transaction and does not run migrations:

```bash
docker exec billflow-thaisunsport-backend /app/lazada_source_items \
  --bill-id e25e856f-8cee-4765-99cb-5b597c1dc4d3 \
  --order-id 1124675871588966 --dry-run
```

After reviewing both source rows, use the hash printed as `plan=`:

```bash
docker exec billflow-thaisunsport-backend /app/lazada_source_items \
  --bill-id e25e856f-8cee-4765-99cb-5b597c1dc4d3 \
  --order-id 1124675871588966 \
  --apply --expected-plan '<hash from dry-run>'
```

Apply locks the bill/items, checks source email identity and artifact checksum, revalidates source money totals and the existing fee, and compares the plan hash. Changed bill data, source, or plan causes a refusal. It replaces only unmapped goods, preserves shipping, allocates coupons using the existing service, keeps `needs_review`, and atomically records the previous raw data and full item snapshot in `lazada_source_items_repaired`. Audit failure rolls back the changes. Existing mapping feedback foreign keys may also prevent deletion; stop and inspect rather than bypassing them. Re-running an already completed repair reports no changes. No SML documents are sent by this command.

Refresh the bill and verify two separate product rows and thumbnails, each quantity 2 at 678, discounts 144 each, existing shipping 134, net 2558. Map each SKU/color to its correct SML product and unit. The repair leaves product mappings blank for explicit review.

## Rollback

For runtime regression before historical repair, retag the script's printed `billflow-lazada-rollback:<stamp>` as the backend image tag shown by `docker inspect --format '{{.Config.Image}}' billflow-thaisunsport-backend`, then recreate backend only and check port 8100 health. Keep the saved source copy with that image.

After a data repair, image rollback alone does not undo rows. Use the atomic audit snapshot or the saved PostgreSQL dump in a separately reviewed, scoped recovery; do not restore the whole database over newer user work.
