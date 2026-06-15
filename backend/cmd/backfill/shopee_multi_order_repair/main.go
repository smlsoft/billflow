// shopee_multi_order_repair creates missing shopee_shipped purchase bills from
// an existing multi-order payment email artifact. It is intentionally targeted:
// use dry-run first, then --apply after count/total reconciliation passes.
//
// Usage:
//
//	shopee_multi_order_repair --bill-id <bill_id> --dry-run --expected-order-count=9 --expected-total=4901
//	shopee_multi_order_repair --message-id <message_id> --apply --expected-order-count=9 --expected-total=4901
//
// Required env var: DATABASE_URL
// Required for --apply: OPENROUTER_API_KEY
// Optional env var: ARTIFACTS_DIR (default /app/artifacts)
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"billflow/internal/config"
	"billflow/internal/database"
	"billflow/internal/handlers"
	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/ai"
	"billflow/internal/services/artifact"
	"billflow/internal/services/catalog"
	emailservice "billflow/internal/services/email"

	"github.com/lib/pq"
	"go.uber.org/zap"
)

type targetBill struct {
	ID        string
	Subject   string
	FromAddr  string
	MessageID string
	Raw       map[string]interface{}
}

type repairReport struct {
	OrderIDs []string
	Existing map[string]string
	Missing  []string
	Total    float64
}

func main() {
	billID := flag.String("bill-id", "", "source bill id that owns the Shopee email artifact")
	messageID := flag.String("message-id", "", "Shopee email Message-ID")
	dryRun := flag.Bool("dry-run", false, "print what would happen without creating bills")
	apply := flag.Bool("apply", false, "create missing bills")
	expectedOrderCount := flag.Int("expected-order-count", 0, "optional exact order count guard")
	expectedTotal := flag.Float64("expected-total", 0, "optional exact email total guard")
	flag.Parse()

	if strings.TrimSpace(*billID) == "" && strings.TrimSpace(*messageID) == "" {
		log.Fatal("--bill-id or --message-id is required")
	}
	if *apply && *dryRun {
		log.Fatal("choose only one of --dry-run or --apply")
	}
	if !*apply {
		*dryRun = true
	}

	cfg := config.Load()
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer db.Close()
	if err := repository.NewAppSettingsRepo(db).ApplyToConfig(cfg); err != nil {
		log.Fatalf("apply app settings: %v", err)
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	artifactRepo := repository.NewBillArtifactRepo(db)
	artifactSvc := artifact.New(cfg.ArtifactsDir, cfg.ArtifactsMaxBytes, artifactRepo, logger)

	target, err := loadTargetBill(db, strings.TrimSpace(*billID), strings.TrimSpace(*messageID))
	if err != nil {
		log.Fatalf("load target: %v", err)
	}
	bodyText, bodyHTML, artifactID, err := loadShopeeEmailBody(artifactSvc, artifactRepo, target)
	if err != nil {
		log.Fatalf("load artifact/body: %v", err)
	}
	report, err := inspectRepairTarget(db, bodyText, bodyHTML)
	if err != nil {
		log.Fatalf("inspect target: %v", err)
	}
	if *expectedOrderCount > 0 && len(report.OrderIDs) != *expectedOrderCount {
		log.Fatalf("expected %d orders, detected %d (%s)", *expectedOrderCount, len(report.OrderIDs), strings.Join(report.OrderIDs, ","))
	}
	if *expectedTotal > 0 && math.Abs(report.Total-*expectedTotal) > 0.01 {
		log.Fatalf("expected total %.2f, detected %.2f", *expectedTotal, report.Total)
	}

	fmt.Printf("target bill: %s\n", target.ID)
	fmt.Printf("message_id: %s\n", target.MessageID)
	fmt.Printf("artifact_id: %s\n", artifactID)
	fmt.Printf("detected=%d existing=%d missing=%d sum=%.2f\n", len(report.OrderIDs), len(report.Existing), len(report.Missing), report.Total)
	fmt.Printf("orders: %s\n", strings.Join(report.OrderIDs, ","))
	if len(report.Missing) > 0 {
		fmt.Printf("missing: %s\n", strings.Join(report.Missing, ","))
	}
	if *dryRun {
		fmt.Println("dry-run: no bills created")
		return
	}
	if len(report.Missing) == 0 {
		fmt.Println("apply: nothing to create")
		return
	}
	if cfg.OpenRouterAPIKey == "" {
		log.Fatal("OPENROUTER_API_KEY is required for --apply")
	}

	billRepo := repository.NewBillRepo(db)
	auditRepo := repository.NewAuditLogRepo(db)
	catalogRepo := repository.NewSMLCatalogRepo(db)
	channelDefaultRepo := repository.NewChannelDefaultRepo(db)
	catalogSvc := catalog.NewSMLCatalogService(catalogRepo, cfg.ShopeeSMLURL, smlHeaders(cfg), logger)
	aiClient := ai.NewClient(
		cfg.OpenRouterAPIKey,
		cfg.OpenRouterModel,
		cfg.OpenRouterFallback,
		cfg.OpenRouterAudioModel,
	).WithAppAttribution(cfg.OpenRouterAppTitle, cfg.OpenRouterAppReferer)

	h := handlers.NewEmailHandler(aiClient, nil, nil, nil, billRepo, auditRepo, nil, cfg.AutoConfirmThreshold, logger)
	h.SetCatalogServices(catalogSvc, nil, nil, catalogRepo)
	h.SetChannelDefaults(channelDefaultRepo)
	h.SetArtifactService(artifactSvc)

	start := time.Now()
	outcome, err := h.ProcessShopeeShippedEmailBody(target.Subject, target.FromAddr, bodyText, bodyHTML, target.MessageID, mailSourceFromRaw(target.Raw))
	if err != nil {
		if skip, ok := err.(*emailservice.MessageSkipError); !ok || skip.Code != "duplicate_or_empty" {
			log.Fatalf("apply repair: %v", err)
		}
	}
	after, err := inspectRepairTarget(db, bodyText, bodyHTML)
	if err != nil {
		log.Fatalf("verify after apply: %v", err)
	}
	if len(after.Missing) > 0 {
		log.Fatalf("repair incomplete after apply; missing: %s", strings.Join(after.Missing, ","))
	}

	targetID := target.ID
	durMs := int(time.Since(start).Milliseconds())
	_ = auditRepo.Log(models.AuditEntry{
		Action:     "shopee_multi_order_repair",
		TargetID:   &targetID,
		Source:     "shopee_shipped",
		Level:      "info",
		DurationMs: &durMs,
		Detail: map[string]interface{}{
			"message_id":         target.MessageID,
			"detected_order_ids": report.OrderIDs,
			"existing_before":    len(report.Existing),
			"missing_before":     report.Missing,
			"existing_after":     len(after.Existing),
			"sum":                after.Total,
			"outcome_kind":       outcome.Kind,
			"outcome_code":       outcome.Code,
		},
	})
	fmt.Printf("apply: repair complete; existing_after=%d missing_after=0 sum=%.2f\n", len(after.Existing), after.Total)
}

func loadTargetBill(db *sql.DB, billID, messageID string) (targetBill, error) {
	where := "id = $1"
	arg := billID
	if billID == "" {
		where = "raw_data->>'email_message_id' = $1"
		arg = messageID
	}
	row := db.QueryRow(
		`SELECT id::text,
		        COALESCE(raw_data->>'subject',''),
		        COALESCE(raw_data->>'from',''),
		        COALESCE(raw_data->>'email_message_id',''),
		        raw_data
		   FROM bills
		  WHERE source = 'shopee_shipped'
		    AND bill_type = 'purchase'
		    AND archived_at IS NULL
		    AND `+where+`
		  ORDER BY created_at ASC
		  LIMIT 1`,
		arg,
	)
	var t targetBill
	var rawBytes []byte
	if err := row.Scan(&t.ID, &t.Subject, &t.FromAddr, &t.MessageID, &rawBytes); err != nil {
		return t, err
	}
	_ = json.Unmarshal(rawBytes, &t.Raw)
	if t.MessageID == "" {
		t.MessageID = messageID
	}
	return t, nil
}

func loadShopeeEmailBody(svc *artifact.Service, repo *repository.BillArtifactRepo, target targetBill) (bodyText, bodyHTML, artifactID string, err error) {
	artifacts, err := repo.ListByBill(target.ID)
	if err != nil {
		return "", "", "", err
	}
	for _, preferKind := range []string{"email_html", "email_text"} {
		for _, a := range artifacts {
			if a.Kind != preferKind {
				continue
			}
			if target.MessageID != "" && artifactMessageID(a) != "" && artifactMessageID(a) != target.MessageID {
				continue
			}
			data, _, err := svc.Read(a.ID)
			if err != nil || len(data) == 0 {
				continue
			}
			if a.Kind == "email_html" {
				return "", string(data), a.ID, nil
			}
			return string(data), "", a.ID, nil
		}
	}
	bodyText = stringField(target.Raw, "body_text")
	bodyHTML = stringField(target.Raw, "body_html")
	if strings.TrimSpace(bodyText) == "" && strings.TrimSpace(bodyHTML) == "" {
		return "", "", "", fmt.Errorf("no email_html/email_text artifact or raw body found")
	}
	return bodyText, bodyHTML, "raw_data", nil
}

func inspectRepairTarget(db *sql.DB, bodyText, bodyHTML string) (repairReport, error) {
	orderIDs := handlers.DetectShopeeBodyOrderIDs(bodyText, bodyHTML)
	if len(orderIDs) == 0 {
		return repairReport{}, fmt.Errorf("no Shopee order ids detected")
	}
	existing, err := existingShopeeBills(db, orderIDs)
	if err != nil {
		return repairReport{}, err
	}
	missing := []string{}
	total := 0.0
	for _, orderID := range orderIDs {
		if existing[orderID] == "" {
			missing = append(missing, orderID)
		}
		amount, ok := repository.ExtractShopeeMoneyLabel(bodyText, bodyHTML, orderID, "ยอดที่ต้องชำระทั้งหมด")
		if !ok {
			return repairReport{}, fmt.Errorf("missing paid total for order %s", orderID)
		}
		total += amount
	}
	return repairReport{OrderIDs: orderIDs, Existing: existing, Missing: missing, Total: math.Round(total*100) / 100}, nil
}

func existingShopeeBills(db *sql.DB, orderIDs []string) (map[string]string, error) {
	out := map[string]string{}
	rows, err := db.Query(
		`SELECT id::text,
		        UPPER(TRIM(LEADING '#' FROM COALESCE(NULLIF(raw_data->>'order_id',''), sml_order_id, ''))) AS order_id
		   FROM bills
		  WHERE source = 'shopee_shipped'
		    AND bill_type = 'purchase'
		    AND archived_at IS NULL
		    AND UPPER(TRIM(LEADING '#' FROM COALESCE(NULLIF(raw_data->>'order_id',''), sml_order_id, ''))) = ANY($1)`,
		pq.Array(orderIDs),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var billID, orderID string
		if err := rows.Scan(&billID, &orderID); err != nil {
			return nil, err
		}
		out[orderID] = billID
	}
	return out, rows.Err()
}

func artifactMessageID(a models.BillArtifact) string {
	if len(a.SourceMeta) == 0 {
		return ""
	}
	var meta struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(a.SourceMeta, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.MessageID)
}

func mailSourceFromRaw(raw map[string]interface{}) emailservice.MailSource {
	return emailservice.MailSource{
		AccountID:   stringField(raw, "imap_account_id"),
		AccountName: stringField(raw, "imap_account_name"),
		Username:    stringField(raw, "imap_username"),
		Mailbox:     stringField(raw, "imap_mailbox"),
		EmailDate:   stringField(raw, "email_date"),
	}
}

func stringField(raw map[string]interface{}, key string) string {
	if raw == nil {
		return ""
	}
	switch v := raw[key].(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func smlHeaders(cfg *config.Config) map[string]string {
	return map[string]string{
		"guid":           cfg.ShopeeSMLGUID,
		"provider":       cfg.ShopeeSMLProvider,
		"configFileName": cfg.ShopeeSMLConfigFile,
		"databaseName":   cfg.ShopeeSMLDatabase,
	}
}
