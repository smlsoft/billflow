package repository

import (
	"database/sql"
	"fmt"

	"billflow/internal/models"
)

type ChannelDefaultRepo struct {
	db *sql.DB
}

func NewChannelDefaultRepo(db *sql.DB) *ChannelDefaultRepo {
	return &ChannelDefaultRepo{db: db}
}

const channelDefaultCols = `
  channel, bill_type, party_code, party_name, party_phone,
  party_address, party_tax_id, doc_format_code, endpoint,
  doc_prefix, doc_running_format,
  wh_code, shelf_code, vat_type, vat_rate,
  updated_by, updated_at
`

func scanChannelDefault(s interface{ Scan(...any) error }) (*models.ChannelDefault, error) {
	d := &models.ChannelDefault{}
	var updatedBy sql.NullString
	err := s.Scan(
		&d.Channel, &d.BillType, &d.PartyCode, &d.PartyName, &d.PartyPhone,
		&d.PartyAddress, &d.PartyTaxID, &d.DocFormatCode, &d.Endpoint,
		&d.DocPrefix, &d.DocRunningFormat,
		&d.WHCode, &d.ShelfCode, &d.VATType, &d.VATRate,
		&updatedBy, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if updatedBy.Valid {
		s := updatedBy.String
		d.UpdatedBy = &s
	}
	return d, nil
}

func (r *ChannelDefaultRepo) ListAll() ([]*models.ChannelDefault, error) {
	rows, err := r.db.Query(
		`SELECT ` + channelDefaultCols + ` FROM channel_defaults
		 ORDER BY channel, bill_type`)
	if err != nil {
		return nil, fmt.Errorf("ListAll channel_defaults: %w", err)
	}
	defer rows.Close()

	var out []*models.ChannelDefault
	for rows.Next() {
		d, err := scanChannelDefault(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *ChannelDefaultRepo) Get(channel, billType string) (*models.ChannelDefault, error) {
	row := r.db.QueryRow(
		`SELECT `+channelDefaultCols+` FROM channel_defaults
		 WHERE channel=$1 AND bill_type=$2`,
		channel, billType,
	)
	d, err := scanChannelDefault(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("Get channel_default: %w", err)
	}
	return d, nil
}

// Upsert inserts or updates by (channel, bill_type).
// updatedBy may be empty when the call comes from a system seed.
func (r *ChannelDefaultRepo) Upsert(d *models.ChannelDefault, updatedBy string) error {
	var ub sql.NullString
	if updatedBy != "" {
		ub = sql.NullString{String: updatedBy, Valid: true}
	}
	_, err := r.db.Exec(
		`INSERT INTO channel_defaults (
		   channel, bill_type, party_code, party_name, party_phone,
		   party_address, party_tax_id, doc_format_code, endpoint,
		   doc_prefix, doc_running_format,
		   wh_code, shelf_code, vat_type, vat_rate,
		   updated_by, updated_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16, NOW())
		 ON CONFLICT (channel, bill_type) DO UPDATE SET
		   party_code = EXCLUDED.party_code,
		   party_name = EXCLUDED.party_name,
		   party_phone = EXCLUDED.party_phone,
		   party_address = EXCLUDED.party_address,
		   party_tax_id = EXCLUDED.party_tax_id,
		   doc_format_code = EXCLUDED.doc_format_code,
		   endpoint = EXCLUDED.endpoint,
		   doc_prefix = EXCLUDED.doc_prefix,
		   doc_running_format = EXCLUDED.doc_running_format,
		   wh_code = EXCLUDED.wh_code,
		   shelf_code = EXCLUDED.shelf_code,
		   vat_type = EXCLUDED.vat_type,
		   vat_rate = EXCLUDED.vat_rate,
		   updated_by = EXCLUDED.updated_by,
		   updated_at = NOW()`,
		d.Channel, d.BillType, d.PartyCode, d.PartyName, d.PartyPhone,
		d.PartyAddress, d.PartyTaxID, d.DocFormatCode, d.Endpoint,
		d.DocPrefix, d.DocRunningFormat,
		d.WHCode, d.ShelfCode, d.VATType, d.VATRate,
		ub,
	)
	if err != nil {
		return fmt.Errorf("Upsert channel_default: %w", err)
	}
	return nil
}

func (r *ChannelDefaultRepo) Delete(channel, billType string) error {
	_, err := r.db.Exec(
		`DELETE FROM channel_defaults WHERE channel=$1 AND bill_type=$2`,
		channel, billType,
	)
	return err
}

// IsEmpty reports whether the table has zero rows. Used by main.go to decide
// whether to run seedChannelDefaultsFromEnv on first boot.
func (r *ChannelDefaultRepo) IsEmpty() (bool, error) {
	var n int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM channel_defaults`).Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}
