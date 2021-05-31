// +build !windows

package cli

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/direct"
	"github.com/pyroscope-io/pyroscope/pkg/analytics"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/atexit"
)

func startServer(cfg *config.Server) error {
	// new a storage with configuration
	s, err := storage.New(cfg)
	if err != nil {
		return fmt.Errorf("new storage: %v", err)
	}
	atexit.Register(func() { s.Close() })

	// new a direct upstream
	u := direct.New(s)

	// uploading the server profile self
	if err := agent.SelfProfile(uint32(cfg.SampleRate), u, "pyroscope.server", logrus.StandardLogger()); err != nil {
		return fmt.Errorf("start self profile: %v", err)
	}

	// debuging the RAM and disk usages
	go printRAMUsage()
	go printDiskUsage(cfg)

	// new server
	c, err := server.New(cfg, s)
	if err != nil {
		return fmt.Errorf("new server: %v", err)
	}
	atexit.Register(func() { c.Stop() })

	// start the analytics
	if !cfg.AnalyticsOptOut {
		analyticsService := analytics.NewService(cfg, s, c)
		go analyticsService.Start()
		atexit.Register(func() { analyticsService.Stop() })
	}
	// if you ever change this line, make sure to update this homebrew test:
	//   https://github.com/pyroscope-io/homebrew-brew/blob/main/Formula/pyroscope.rb#L94
	logrus.Info("starting HTTP server")

	// start the server
	return c.Start()
}
