package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"billflow/internal/models"
)

type MappingRepo struct {
	db *sql.DB
}

func NewMappingRepo(db *sql.DB) *MappingRepo {
	return &MappingRepo{db: db}
}

func (r *MappingRepo) FindByRawName(rawName string) (*models.Mapping, error) {
	m := &models.Mapping{}
	err := r.db.QueryRow(
		`SELECT id, raw_name, item_code, unit_code, confidence, source,
		        usage_count, last_used_at, learned_from_bill_id, created_by, created_at
		 FROM mappings WHERE raw_name = $1`, rawName,
	).Scan(
		&m.ID, &m.RawName, &m.ItemCode, &m.UnitCode, &m.Confidence,
		&m.Source, &m.UsageCount, &m.LastUsedAt, &m.LearnedFromBillID,
		&m.CreatedBy, &m.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("FindByRawName: %w", err)
	}
	return m, nil
}

func (r *MappingRepo) ListAll() ([]models.Mapping, error) {
	rows, err := r.db.Query(
		`SELECT id, raw_name, item_code, unit_code, confidence, source,
		        usage_count, last_used_at, created_at
		 FROM mappings ORDER BY usage_count DESC, raw_name`,
	)
	if err != nil {
		return nil, fmt.Errorf("ListAll: %w", err)
	}
	defer rows.Close()

	var mappings []models.Mapping
	for rows.Next() {
		var m models.Mapping
		if err := rows.Scan(
			&m.ID, &m.RawName, &m.ItemCode, &m.UnitCode, &m.Confidence,
			&m.Source, &m.UsageCount, &m.LastUsedAt, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

// ListPage returns a bounded admin view of mappings. ListAll remains for the
// mapper service, which intentionally loads the complete learned-memory set.
func (r *MappingRepo) ListPage(page, perPage int, query string) ([]models.Mapping, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}

	query = strings.TrimSpace(query)
	whereSQL := ""
	args := []interface{}{}
	if query != "" {
		pattern := "%" + escapeMappingSearch(query) + "%"
		whereSQL = ` WHERE raw_name ILIKE $1 ESCAPE '\' OR item_code ILIKE $1 ESCAPE '\' OR unit_code ILIKE $1 ESCAPE '\'`
		args = append(args, pattern)
	}

	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM mappings`+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListPage count: %w", err)
	}

	offset := (page - 1) * perPage
	limitArg, offsetArg := "$1", "$2"
	if query != "" {
		limitArg, offsetArg = "$2", "$3"
	}
	args = append(args, perPage, offset)
	rows, err := r.db.Query(
		`SELECT id, raw_name, item_code, unit_code, confidence, source,
		        usage_count, last_used_at, created_at
		 FROM mappings`+whereSQL+` ORDER BY usage_count DESC, raw_name LIMIT `+limitArg+` OFFSET `+offsetArg,
		args...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("ListPage: %w", err)
	}
	defer rows.Close()

	mappings := make([]models.Mapping, 0, perPage)
	for rows.Next() {
		var m models.Mapping
		if err := rows.Scan(
			&m.ID, &m.RawName, &m.ItemCode, &m.UnitCode, &m.Confidence,
			&m.Source, &m.UsageCount, &m.LastUsedAt, &m.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		mappings = append(mappings, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return mappings, total, nil
}

func escapeMappingSearch(query string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return replacer.Replace(query)
}

func (r *MappingRepo) Create(rawName, itemCode, unitCode, createdBy string) (*models.Mapping, error) {
	m := &models.Mapping{}
	err := r.db.QueryRow(
		`INSERT INTO mappings (raw_name, item_code, unit_code, source, created_by)
		 VALUES ($1, $2, $3, 'manual', $4)
		 RETURNING id, raw_name, item_code, unit_code, confidence, source, usage_count, created_at`,
		rawName, itemCode, unitCode, createdBy,
	).Scan(&m.ID, &m.RawName, &m.ItemCode, &m.UnitCode, &m.Confidence, &m.Source, &m.UsageCount, &m.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("Create mapping: %w", err)
	}
	return m, nil
}

func (r *MappingRepo) Upsert(rawName, itemCode, unitCode, source string, billID *string) error {
	_, err := r.db.Exec(
		`INSERT INTO mappings (raw_name, item_code, unit_code, source, confidence, learned_from_bill_id)
		 VALUES ($1, $2, $3, $4, 1.0, $5)
		 ON CONFLICT (raw_name) DO UPDATE
		   SET item_code = EXCLUDED.item_code,
		       unit_code = EXCLUDED.unit_code,
		       source = EXCLUDED.source,
		       confidence = 1.0,
		       learned_from_bill_id = EXCLUDED.learned_from_bill_id,
		       usage_count = mappings.usage_count + 1,
		       last_used_at = NOW()`,
		rawName, itemCode, unitCode, source, billID,
	)
	return err
}

func (r *MappingRepo) IncrementUsage(id string) error {
	_, err := r.db.Exec(
		`UPDATE mappings SET usage_count = usage_count + 1, last_used_at = NOW() WHERE id = $1`,
		id,
	)
	return err
}

func (r *MappingRepo) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM mappings WHERE id = $1`, id)
	return err
}

func (r *MappingRepo) Stats() (map[string]interface{}, error) {
	stats := map[string]interface{}{}

	var total, aiLearned, manual int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM mappings`).Scan(&total)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM mappings WHERE source='ai_learned'`).Scan(&aiLearned)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM mappings WHERE source='manual'`).Scan(&manual)

	stats["total"] = total
	stats["ai_learned"] = aiLearned
	stats["manual"] = manual
	// auto_confirmed = ai_learned mappings (system learned from feedback)
	// needs_review = manual mappings (admin had to map manually)
	// Both share the same denominator (total mappings) for consistent %-bars
	stats["auto_confirmed"] = aiLearned
	stats["needs_review"] = manual

	var feedbackCount int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM mapping_feedback`).Scan(&feedbackCount)
	stats["feedback_count"] = feedbackCount

	return stats, nil
}
