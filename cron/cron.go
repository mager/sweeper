package cron

import (
	"time"

	"github.com/go-co-op/gocron"
)

func ProvideCron() *gocron.Scheduler {
	return gocron.NewScheduler(time.UTC)
}

var Options = ProvideCron
