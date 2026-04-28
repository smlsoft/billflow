package repository

import (
	"database/sql"
	"fmt"

	"billflow/internal/models"
)

type ImapAccountRepo struct {
	db *sql.DB
}

func NewImapAccountRepo(db *sql.DB) *ImapAccountRepo {
	return &ImapAccountRepo{db: db}
}

const imapSelectCols = `
  id, name, host, port, username, password, mailbox,
  filter_from, filter_subjects, channel, shopee_domains,
  lookback_days, poll_interval_seconds, enabled,
  last_polled_at, last_poll_status, last_poll_error, last_poll_messages,
  consecutive_failures, last_admin_alert_at, created_at, updated_at
`

func scanImapAccount(s interface{ Scan(...any) error }) (*models.IMAPAccount, error) {
	a := &models.IMAPAccount{}
	var status, errMsg sql.NullString
	var msgCount sql.NullInt32
	err := s.Scan(
		&a.ID, &a.Name, &a.Host, &a.Port, &a.Username, &a.Password, &a.Mailbox,
		&a.FilterFrom, &a.FilterSubjects, &a.Channel, &a.ShopeeDomains,
		&a.LookbackDays, &a.PollIntervalSeconds, &a.Enabled,
		&a.LastPolledAt, &status, &errMsg, &msgCount,
		&a.ConsecutiveFailures, &a.LastAdminAlertAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if status.Valid {
		s := status.String
		a.LastPollStatus = &s
	}
	if errMsg.Valid {
		s := errMsg.String
		a.LastPollError = &s
	}
	if msgCount.Valid {
		n := int(msgCount.Int32)
		a.LastPollMessages = &n
	}
	return a, nil
}

func (r *ImapAccountRepo) ListAll() ([]*models.IMAPAccount, error) {
	rows, err := r.db.Query(`SELECT ` + imapSelectCols + ` FROM imap_accounts ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("ListAll imap_accounts: %w", err)
	}
	defer rows.Close()

	var out []*models.IMAPAccount
	for rows.Next() {
		a, err := scanImapAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *ImapAccountRepo) ListEnabled() ([]*models.IMAPAccount, error) {
	rows, err := r.db.Query(
		`SELECT ` + imapSelectCols + ` FROM imap_accounts WHERE enabled = TRUE ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("ListEnabled imap_accounts: %w", err)
	}
	defer rows.Close()

	var out []*models.IMAPAccount
	for rows.Next() {
		a, err := scanImapAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *ImapAccountRepo) GetByID(id string) (*models.IMAPAccount, error) {
	row := r.db.QueryRow(`SELECT `+imapSelectCols+` FROM imap_accounts WHERE id = $1`, id)
	a, err := scanImapAccount(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetByID imap_account: %w", err)
	}
	return a, nil
}

// Create inserts a new account. The runtime status fields stay at defaults.
func (r *ImapAccountRepo) Create(a *models.IMAPAccount) error {
	return r.db.QueryRow(
		`INSERT INTO imap_accounts (
		   name, host, port, username, password, mailbox,
		   filter_from, filter_subjects, channel, shopee_domains,
		   lookback_days, poll_interval_seconds, enabled
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 RETURNING id, created_at, updated_at`,
		a.Name, a.Host, a.Port, a.Username, a.Password, a.Mailbox,
		a.FilterFrom, a.FilterSubjects, a.Channel, a.ShopeeDomains,
		a.LookbackDays, a.PollIntervalSeconds, a.Enabled,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

// Update replaces the user-editable fields. If the supplied password is
// empty the existing one is preserved (so the UI can omit it on edit).
func (r *ImapAccountRepo) Update(id string, a *models.IMAPAccount) error {
	if a.Password == "" {
		_, err := r.db.Exec(
			`UPDATE imap_accounts SET
			   name=$2, host=$3, port=$4, username=$5, mailbox=$6,
			   filter_from=$7, filter_subjects=$8, channel=$9, shopee_domains=$10,
			   lookback_days=$11, poll_interval_seconds=$12, enabled=$13,
			   updated_at=NOW()
			 WHERE id=$1`,
			id, a.Name, a.Host, a.Port, a.Username, a.Mailbox,
			a.FilterFrom, a.FilterSubjects, a.Channel, a.ShopeeDomains,
			a.LookbackDays, a.PollIntervalSeconds, a.Enabled,
		)
		return err
	}
	_, err := r.db.Exec(
		`UPDATE imap_accounts SET
		   name=$2, host=$3, port=$4, username=$5, password=$6, mailbox=$7,
		   filter_from=$8, filter_subjects=$9, channel=$10, shopee_domains=$11,
		   lookback_days=$12, poll_interval_seconds=$13, enabled=$14,
		   updated_at=NOW()
		 WHERE id=$1`,
		id, a.Name, a.Host, a.Port, a.Username, a.Password, a.Mailbox,
		a.FilterFrom, a.FilterSubjects, a.Channel, a.ShopeeDomains,
		a.LookbackDays, a.PollIntervalSeconds, a.Enabled,
	)
	return err
}

func (r *ImapAccountRepo) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM imap_accounts WHERE id = $1`, id)
	return err
}

// UpdatePollStatus is called by the coordinator after each poll cycle.
// status="ok" resets consecutive_failures to 0; anything else increments.
func (r *ImapAccountRepo) UpdatePollStatus(id, status, errMsg string, messageCount int) error {
	var em sql.NullString
	if errMsg != "" {
		em = sql.NullString{String: errMsg, Valid: true}
	}
	if status == "ok" {
		_, err := r.db.Exec(
			`UPDATE imap_accounts SET
			   last_polled_at=NOW(), last_poll_status='ok', last_poll_error=NULL,
			   last_poll_messages=$2, consecutive_failures=0
			 WHERE id=$1`,
			id, messageCount,
		)
		return err
	}
	_, err := r.db.Exec(
		`UPDATE imap_accounts SET
		   last_polled_at=NOW(), last_poll_status=$2, last_poll_error=$3,
		   last_poll_messages=$4,
		   consecutive_failures = consecutive_failures + 1
		 WHERE id=$1`,
		id, status, em, messageCount,
	)
	return err
}

// MarkAlertSent stamps last_admin_alert_at so the LINE notify throttler
// can skip resending until > 1 h has elapsed.
func (r *ImapAccountRepo) MarkAlertSent(id string) error {
	_, err := r.db.Exec(
		`UPDATE imap_accounts SET last_admin_alert_at=NOW() WHERE id=$1`, id)
	return err
}
