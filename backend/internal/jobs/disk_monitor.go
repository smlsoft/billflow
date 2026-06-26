package jobs

import (
	"fmt"
	"syscall"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

type DiskMonitor struct {
	warnPercent int
	notifier    Notifier
	instanceID  string
	log         *zap.Logger
}

func NewDiskMonitor(warnPercent int, notifier Notifier, instanceID string, log *zap.Logger) *DiskMonitor {
	return &DiskMonitor{warnPercent: warnPercent, notifier: notifier, instanceID: instanceID, log: log}
}

func (j *DiskMonitor) Register(c *cron.Cron) {
	// Every day at 07:00
	c.AddFunc("0 7 * * *", j.Run)
}

func (j *DiskMonitor) Run() {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		j.log.Error("disk monitor: statfs", zap.Error(err))
		return
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free
	usedPct := int(float64(used) / float64(total) * 100)
	usedGB := float64(used) / 1024 / 1024 / 1024
	totalGB := float64(total) / 1024 / 1024 / 1024

	j.log.Info("disk monitor", zap.Int("used_pct", usedPct))

	if usedPct >= j.warnPercent {
		msg := fmt.Sprintf(
			"⚠️ [%s] Disk Usage High\n─────────────────────\nUsed  : %d%% (%.1fGB / %.1fGB)\nLimit : %d%%",
			j.instanceID, usedPct, usedGB, totalGB, j.warnPercent,
		)
		if j.notifier != nil {
			_ = j.notifier.PushAdmin(msg)
		}
	}
}
