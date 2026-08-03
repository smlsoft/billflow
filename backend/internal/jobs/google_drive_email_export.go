package jobs

import (
	"context"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"billflow/internal/services/googledrive"
)

type GoogleDriveEmailExportCron struct {
	service *googledrive.Service
	logger  *zap.Logger
}

func NewGoogleDriveEmailExportCron(service *googledrive.Service, logger *zap.Logger) *GoogleDriveEmailExportCron {
	return &GoogleDriveEmailExportCron{service: service, logger: logger}
}

func (j *GoogleDriveEmailExportCron) Register(c *cron.Cron) {
	if j == nil || j.service == nil {
		return
	}
	_, _ = c.AddFunc("@every 1m", func() { j.service.RunDue(context.Background()) })
	_, _ = c.AddFunc("@every 10m", j.service.ReconcileRecentSent)
	if j.logger != nil {
		j.logger.Info("google drive email export cron registered")
	}
}
