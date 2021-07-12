package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct"
	"github.com/pyroscope-io/pyroscope/pkg/analytics"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/debug"
)

type serverService struct {
	config           *config.Server
	logger           *logrus.Logger
	controller       *server.Controller
	storage          *storage.Storage
	directUpstream   *direct.Direct
	analyticsService *analytics.Service
	selfProfiling    *agent.ProfileSession
	debugReporter    *debug.Reporter

	stopped chan struct{}
	done    chan struct{}
	group   *errgroup.Group
}

func newServerService(logger *logrus.Logger, c *config.Server) (*serverService, error) {
	svc := serverService{
		config:  c,
		logger:  logger,
		stopped: make(chan struct{}),
		done:    make(chan struct{}),
	}

	var err error
	svc.storage, err = storage.New(svc.config)
	if err != nil {
		return nil, fmt.Errorf("new storage: %v", err)
	}
	svc.controller, err = server.New(svc.config, svc.storage, svc.logger)
	if err != nil {
		return nil, fmt.Errorf("new server: %v", err)
	}

	svc.debugReporter = debug.NewReporter(svc.logger, svc.storage, svc.config)
	svc.directUpstream = direct.New(svc.storage)
	selfProfilingConfig := &agent.SessionConfig{
		Upstream:       svc.directUpstream,
		AppName:        "pyroscope.server",
		ProfilingTypes: types.DefaultProfileTypes,
		SpyName:        types.GoSpy,
		SampleRate:     100,
		UploadRate:     10 * time.Second,
	}
	svc.selfProfiling = agent.NewSession(selfProfilingConfig, svc.logger)
	if !c.AnalyticsOptOut {
		svc.analyticsService = analytics.NewService(c, svc.storage, svc.controller)
	}

	return &svc, nil
}

func (svc *serverService) Start() error {
	g, ctx := errgroup.WithContext(context.Background())
	svc.group = g
	g.Go(func() error {
		// if you ever change this line, make sure to update this homebrew test:
		// https://github.com/pyroscope-io/homebrew-brew/blob/main/Formula/pyroscope.rb#L94
		svc.logger.Info("starting HTTP server")
		return svc.controller.Start()
	})

	go svc.debugReporter.Start()
	if svc.analyticsService != nil {
		go svc.analyticsService.Start()
	}

	svc.directUpstream.Start()
	if err := svc.selfProfiling.Start(); err != nil {
		svc.logger.WithError(err).Error("failed to start self-profiling")
	}

	svc.logger.Debug("collecting local profiles")
	if err := svc.storage.CollectLocalProfiles(); err != nil {
		svc.logger.WithError(err).Error("failed to collect local profiles")
	}

	defer close(svc.done)
	select {
	case <-svc.stopped:
	case <-ctx.Done():
		// The context is canceled the first time a function passed to Go
		// returns a non-nil error.
	}
	// N.B. internal components are de-initialized/disposed (if applicable)
	// regardless of the exit reason. Once server is stopped, wait for all
	// Go goroutines to finish.
	svc.stop()
	return svc.group.Wait()
}

func (svc *serverService) Stop() {
	close(svc.stopped)
	<-svc.done
}

func (svc *serverService) stop() {
	svc.controller.Drain()
	svc.debugReporter.Stop()
	if svc.analyticsService != nil {
		svc.analyticsService.Stop()
	}
	svc.selfProfiling.Stop()
	svc.directUpstream.Stop()
	svc.logger.Debug("stopping storage")
	if err := svc.storage.Close(); err != nil {
		svc.logger.WithError(err).Error("storage close")
	}
	svc.logger.Debug("stopping http server")
	if err := svc.controller.Stop(); err != nil {
		svc.logger.WithError(err).Error("controller stop")
	}
}
