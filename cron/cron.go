package cron

import (
	"context"
	"time"

	"github.com/go-co-op/gocron"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func ProvideCron(lc fx.Lifecycle, logger *zap.SugaredLogger) *gocron.Scheduler {
	s := gocron.NewScheduler(time.UTC)

	lc.Append(
		fx.Hook{
			OnStart: func(context.Context) error {
				s.Every(24).Hours().Do(func() {
					logger.Info("Ping")
				})
				s.StartAsync()
				return nil
			},
			OnStop: func(context.Context) error {
				defer s.Clear()
				return nil
			},
		},
	)

	return s
}

var Options = ProvideCron
