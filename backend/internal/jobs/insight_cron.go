package jobs

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"billflow/internal/repository"
	"billflow/internal/services/insight"
)

type InsightCron struct {
	insightSvc  *insight.Service
	billRepo    *repository.BillRepo
	insightRepo *repository.InsightRepo
	notifier    Notifier
	notify      bool
	log         *zap.Logger
}

func NewInsightCron(
	insightSvc *insight.Service,
	billRepo *repository.BillRepo,
	insightRepo *repository.InsightRepo,
	notifier Notifier,
	notify bool,
	log *zap.Logger,
) *InsightCron {
	return &InsightCron{
		insightSvc:  insightSvc,
		billRepo:    billRepo,
		insightRepo: insightRepo,
		notifier:    notifier,
		notify:      notify,
		log:         log,
	}
}

func (j *InsightCron) Register(c *cron.Cron, hour int) {
	schedule := fmt.Sprintf("0 %d * * *", hour)
	c.AddFunc(schedule, j.Run)
}

func (j *InsightCron) Run() {
	j.log.Info("insight cron: starting")
	start := time.Now()

	stats, err := j.billRepo.DashboardStats()
	if err != nil {
		j.log.Error("insight cron: get stats", zap.Error(err))
		return
	}

	statsBytes, _ := json.Marshal(stats)
	text, err := j.insightSvc.Generate(string(statsBytes))
	if err != nil {
		j.log.Error("insight cron: generate", zap.Error(err))
		return
	}

	if j.insightRepo != nil {
		if err := j.insightRepo.Save(string(statsBytes), text); err != nil {
			j.log.Error("insight cron: save to DB", zap.Error(err))
		}
	}

	j.log.Info("insight cron: done", zap.Duration("elapsed", time.Since(start)))

	if j.notify && j.notifier != nil {
		if err := j.notifier.PushAdmin("📊 Daily Insight\n" + text); err != nil {
			j.log.Error("insight cron: notify", zap.Error(err))
		}
	}
}
