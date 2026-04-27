package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"billflow/internal/models"
)

// SMLCatalogRepo handles DB operations for sml_catalog
type SMLCatalogRepo struct {
	db *sql.DB
}

func NewSMLCatalogRepo(db *sql.DB) *SMLCatalogRepo {
	return &SMLCatalogRepo{db: db}
}

// DB returns the underlying database connection (for cross-table ops in handlers)
func (r *SMLCatalogRepo) DB() *sql.DB {
	return r.db
}

// UpdateItemMapping sets item_code + unit_code + mapped=true for a bill_item
func (r *SMLCatalogRepo) UpdateItemMapping(billItemID, billID, itemCode, unitCode string) error {
	_, err := r.db.Exec(`
		UPDATE bill_items
		SET item_code = $1, unit_code = $2, mapped = TRUE
		WHERE id = $3 AND bill_id = $4
	`, itemCode, unitCode, billItemID, billID)
	return err
}

// CountUnmappedItems returns number of bill_items for a bill with mapped=false
func (r *SMLCatalogRepo) CountUnmappedItems(billID string) (int, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM bill_items WHERE bill_id = $1 AND mapped = FALSE`, billID,
	).Scan(&n)
	return n, err
}

// Upsert inserts or updates a catalog item (no embedding)
func (r *SMLCatalogRepo) Upsert(item models.CatalogItem) error {
	_, err := r.db.Exec(`
		INSERT INTO sml_catalog
		  (item_code, item_name, item_name2, unit_code, wh_code, shelf_code,
		   price, group_code, balance_qty, synced_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())
		ON CONFLICT (item_code) DO UPDATE SET
		  item_name   = EXCLUDED.item_name,
		  item_name2  = EXCLUDED.item_name2,
		  unit_code   = EXCLUDED.unit_code,
		  wh_code     = EXCLUDED.wh_code,
		  shelf_code  = EXCLUDED.shelf_code,
		  price       = EXCLUDED.price,
		  group_code  = EXCLUDED.group_code,
		  balance_qty = EXCLUDED.balance_qty,
		  synced_at   = NOW(),
		  -- Reset embedding if name changed
		  embedding_status = CASE
		    WHEN sml_catalog.item_name != EXCLUDED.item_name
		      OR sml_catalog.item_name2 != EXCLUDED.item_name2
		    THEN 'pending'
		    ELSE sml_catalog.embedding_status
		  END,
		  embedded_at = CASE
		    WHEN sml_catalog.item_name != EXCLUDED.item_name
		      OR sml_catalog.item_name2 != EXCLUDED.item_name2
		    THEN NULL
		    ELSE sml_catalog.embedded_at
		  END,
		  embedding = CASE
		    WHEN sml_catalog.item_name != EXCLUDED.item_name
		      OR sml_catalog.item_name2 != EXCLUDED.item_name2
		    THEN NULL
		    ELSE sml_catalog.embedding
		  END
	`,
		item.ItemCode, item.ItemName, item.ItemName2,
		item.UnitCode, item.WHCode, item.ShelfCode,
		item.Price, item.GroupCode, item.BalanceQty,
	)
	return err
}

// SetEmbedding saves a computed embedding for one item
func (r *SMLCatalogRepo) SetEmbedding(itemCode string, embedding []float64, model string) error {
	embJSON, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}
	now := time.Now()
	_, err = r.db.Exec(`
		UPDATE sml_catalog
		SET embedding = $1, embedding_status = 'done', embedded_at = $2, embedding_model = $3
		WHERE item_code = $4
	`, embJSON, now, model, itemCode)
	return err
}

// SetEmbeddingError marks an item as embedding error
func (r *SMLCatalogRepo) SetEmbeddingError(itemCode string) error {
	_, err := r.db.Exec(`
		UPDATE sml_catalog SET embedding_status = 'error' WHERE item_code = $1
	`, itemCode)
	return err
}

// GetEmbedding retrieves the stored embedding for one item
func (r *SMLCatalogRepo) GetEmbedding(itemCode string) ([]float64, error) {
	var embJSON []byte
	err := r.db.QueryRow(`SELECT embedding FROM sml_catalog WHERE item_code = $1`, itemCode).Scan(&embJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if embJSON == nil {
		return nil, nil
	}
	var emb []float64
	if err := json.Unmarshal(embJSON, &emb); err != nil {
		return nil, fmt.Errorf("unmarshal embedding: %w", err)
	}
	return emb, nil
}

// EmbeddedItem is used for in-memory catalog search index building
type EmbeddedItem struct {
	ItemCode  string
	ItemName  string
	ItemName2 string
	UnitCode  string
	WHCode    string
	ShelfCode string
	Price     *float64
	Embedding []float64
}

// LoadAllEmbeddings returns all items with embedding_status='done'
// Used to build the in-memory search index
func (r *SMLCatalogRepo) LoadAllEmbeddings() ([]EmbeddedItem, error) {
	rows, err := r.db.Query(`
		SELECT item_code, item_name, item_name2, unit_code, wh_code, shelf_code,
		       COALESCE(price, 0), embedding
		FROM sml_catalog
		WHERE embedding_status = 'done' AND embedding IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []EmbeddedItem
	for rows.Next() {
		var it EmbeddedItem
		var embJSON []byte
		if err := rows.Scan(
			&it.ItemCode, &it.ItemName, &it.ItemName2,
			&it.UnitCode, &it.WHCode, &it.ShelfCode,
			&it.Price, &embJSON,
		); err != nil {
			continue
		}
		if embJSON != nil {
			_ = json.Unmarshal(embJSON, &it.Embedding)
		}
		if len(it.Embedding) > 0 {
			items = append(items, it)
		}
	}
	return items, rows.Err()
}

// List returns paginated catalog items (no embedding data).
// q filters by item_code or item_name (case-insensitive prefix/substring match).
func (r *SMLCatalogRepo) List(page, perPage int, statusFilter, q string) ([]models.CatalogItem, int, error) {
	offset := (page - 1) * perPage

	// Build WHERE clauses
	conditions := []string{}
	countArgs := []interface{}{}
	if statusFilter != "" {
		conditions = append(conditions, fmt.Sprintf("embedding_status = $%d", len(countArgs)+1))
		countArgs = append(countArgs, statusFilter)
	}
	if q != "" {
		like := "%" + q + "%"
		conditions = append(conditions, fmt.Sprintf("(item_code ILIKE $%d OR item_name ILIKE $%d OR item_name2 ILIKE $%d)", len(countArgs)+1, len(countArgs)+2, len(countArgs)+3))
		countArgs = append(countArgs, like, like, like)
	}
	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + joinAnd(conditions)
	}

	var total int
	countQ := "SELECT COUNT(*) FROM sml_catalog " + where
	if err := r.db.QueryRow(countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// For the main query, append LIMIT/OFFSET args
	listArgs := append(countArgs, perPage, offset)
	n := len(listArgs)
	query := fmt.Sprintf(`
		SELECT item_code, item_name, item_name2, unit_code, wh_code, shelf_code,
		       price, group_code, balance_qty, embedding_status, embedded_at, synced_at, created_at
		FROM sml_catalog
		%s
		ORDER BY item_code
		LIMIT $%d OFFSET $%d
	`, where, n-1, n)

	rows, err := r.db.Query(query, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []models.CatalogItem
	for rows.Next() {
		var it models.CatalogItem
		if err := rows.Scan(
			&it.ItemCode, &it.ItemName, &it.ItemName2,
			&it.UnitCode, &it.WHCode, &it.ShelfCode,
			&it.Price, &it.GroupCode, &it.BalanceQty,
			&it.EmbeddingStatus, &it.EmbeddedAt,
			&it.SyncedAt, &it.CreatedAt,
		); err != nil {
			continue
		}
		items = append(items, it)
	}
	return items, total, rows.Err()
}

func joinAnd(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " AND "
		}
		result += p
	}
	return result
}

// Stats returns count by embedding_status
func (r *SMLCatalogRepo) Stats() (total, done, pending, errCount int, err error) {
	err = r.db.QueryRow(`
		SELECT
		  COUNT(*),
		  COUNT(*) FILTER (WHERE embedding_status = 'done'),
		  COUNT(*) FILTER (WHERE embedding_status = 'pending'),
		  COUNT(*) FILTER (WHERE embedding_status = 'error')
		FROM sml_catalog
	`).Scan(&total, &done, &pending, &errCount)
	return
}

// GetOne returns a single catalog item
func (r *SMLCatalogRepo) GetOne(itemCode string) (*models.CatalogItem, error) {
	var it models.CatalogItem
	err := r.db.QueryRow(`
		SELECT item_code, item_name, item_name2, unit_code, wh_code, shelf_code,
		       price, group_code, balance_qty, embedding_status, embedded_at, synced_at, created_at
		FROM sml_catalog WHERE item_code = $1
	`, itemCode).Scan(
		&it.ItemCode, &it.ItemName, &it.ItemName2,
		&it.UnitCode, &it.WHCode, &it.ShelfCode,
		&it.Price, &it.GroupCode, &it.BalanceQty,
		&it.EmbeddingStatus, &it.EmbeddedAt,
		&it.SyncedAt, &it.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &it, err
}

// CountPending returns number of items pending embedding
func (r *SMLCatalogRepo) CountPending() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM sml_catalog WHERE embedding_status = 'pending'`).Scan(&n)
	return n, err
}

// GetPendingBatch returns a batch of pending items for embedding
func (r *SMLCatalogRepo) GetPendingBatch(limit int) ([]models.CatalogItem, error) {
	rows, err := r.db.Query(`
		SELECT item_code, item_name, item_name2
		FROM sml_catalog
		WHERE embedding_status = 'pending'
		ORDER BY item_code
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.CatalogItem
	for rows.Next() {
		var it models.CatalogItem
		_ = rows.Scan(&it.ItemCode, &it.ItemName, &it.ItemName2)
		items = append(items, it)
	}
	return items, rows.Err()
}

// ListAllNames returns all item codes + names (for Levenshtein fallback)
func (r *SMLCatalogRepo) ListAllNames() ([]models.CatalogItem, error) {
	rows, err := r.db.Query(`
		SELECT item_code, item_name, item_name2, unit_code, wh_code, shelf_code, COALESCE(price, 0)
		FROM sml_catalog
		ORDER BY item_code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.CatalogItem
	for rows.Next() {
		var it models.CatalogItem
		var price float64
		_ = rows.Scan(&it.ItemCode, &it.ItemName, &it.ItemName2,
			&it.UnitCode, &it.WHCode, &it.ShelfCode, &price)
		it.Price = &price
		items = append(items, it)
	}
	return items, rows.Err()
}
