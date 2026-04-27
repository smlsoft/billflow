package jobs

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

type BackupCron struct {
	dbUser    string
	dbName    string
	backupDir string
	log       *zap.Logger
}

func NewBackupCron(dbUser, dbName, backupDir string, log *zap.Logger) *BackupCron {
	return &BackupCron{
		dbUser:    dbUser,
		dbName:    dbName,
		backupDir: backupDir,
		log:       log,
	}
}

func (j *BackupCron) Register(c *cron.Cron, hour int) {
	schedule := fmt.Sprintf("0 %d * * *", hour)
	c.AddFunc(schedule, j.Run)
}

func (j *BackupCron) Run() {
	j.log.Info("backup cron: starting")
	date := time.Now().Format("20060102")
	outFile := filepath.Join(j.backupDir, date+".sql.gz")

	cmd := exec.Command("sh", "-c",
		fmt.Sprintf(
			"docker exec billflow-postgres pg_dump -U %s %s | gzip > %s",
			j.dbUser, j.dbName, outFile,
		),
	)
	if err := cmd.Run(); err != nil {
		j.log.Error("backup cron: pg_dump failed", zap.Error(err))
		return
	}

	// Prune backups older than 30 days
	prune := exec.Command("sh", "-c",
		fmt.Sprintf("find %s -name '*.sql.gz' -mtime +30 -delete", j.backupDir),
	)
	_ = prune.Run()

	j.log.Info("backup cron: done", zap.String("file", outFile))
}
