package exec

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mitchellh/go-ps"
	"github.com/pyroscope-io/pyroscope/pkg/agent"
	"github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/atexit"
	"github.com/pyroscope-io/pyroscope/pkg/util/names"
	"github.com/sirupsen/logrus"
)

// used in tests
var disableMacOSChecks bool
var disableLinuxChecks bool

// Cli is command line interface for both exec and connect commands
func Cli(cfg *config.Exec, args []string) error {
	// isExec = true means we need to start the process first (pyroscope exec)
	// isExec = false means the process is already there (pyroscope connect)
	isExec := cfg.Pid == 0

	if isExec && len(args) == 0 {
		return errors.New("no arguments passed")
	}

	// TODO: this is somewhat hacky, we need to find a better way to configure agents
	pyspy.Blocking = cfg.PyspyBlocking

	spyName := cfg.SpyName
	if spyName == "auto" {
		if isExec {
			baseName := path.Base(args[0])
			spyName = spy.ResolveAutoName(baseName)
			if spyName == "" {
				supportedSpies := spy.SupportedExecSpies()
				suggestedCommand := fmt.Sprintf("pyroscope exec -spy-name %s %s", supportedSpies[0], strings.Join(args, " "))
				return fmt.Errorf(
					"could not automatically find a spy for program \"%s\". Pass spy name via %s argument, for example: \n  %s\n\nAvailable spies are: %s\nIf you believe this is a mistake, please submit an issue at %s",
					baseName,
					color.YellowString("-spy-name"),
					color.YellowString(suggestedCommand),
					strings.Join(supportedSpies, ","),
					color.GreenString("https://github.com/pyroscope-io/pyroscope/issues"),
				)
			}
		} else {
			supportedSpies := spy.SupportedExecSpies()
			suggestedCommand := fmt.Sprintf("pyroscope connect -spy-name %s %s", supportedSpies[0], strings.Join(args, " "))
			return fmt.Errorf(
				"pass spy name via %s argument, for example: \n  %s\n\nAvailable spies are: %s\nIf you believe this is a mistake, please submit an issue at %s",
				color.YellowString("-spy-name"),
				color.YellowString(suggestedCommand),
				strings.Join(supportedSpies, ","),
				color.GreenString("https://github.com/pyroscope-io/pyroscope/issues"),
			)
		}
	}

	logrus.Info("to disable logging from pyroscope, pass " + color.YellowString("-no-logging") + " argument to pyroscope exec")

	if err := performChecks(spyName); err != nil {
		return err
	}

	if cfg.ApplicationName == "" {
		logrus.Infof("we recommend specifying application name via %s flag or env variable %s", color.YellowString("-application-name"), color.YellowString("PYROSCOPE_APPLICATION_NAME"))
		cfg.ApplicationName = spyName + "." + names.GetRandomName(generateSeed(args))
		logrus.Infof("for now we chose the name for you and it's \"%s\"", color.GreenString(cfg.ApplicationName))
	}

	logrus.WithFields(logrus.Fields{
		"args": fmt.Sprintf("%q", args),
	}).Debug("starting command")

	pid := cfg.Pid
	var cmd *exec.Cmd
	if isExec {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin
		if err := adjustCmd(cmd, *cfg); err != nil {
			logrus.Error(err)
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		pid = cmd.Process.Pid
	}

	rc := remote.RemoteConfig{
		AuthToken:              cfg.AuthToken,
		UpstreamAddress:        cfg.ServerAddress,
		UpstreamThreads:        cfg.UpstreamThreads,
		UpstreamRequestTimeout: cfg.UpstreamRequestTimeout,
	}
	u, err := remote.New(rc, logrus.StandardLogger())
	if err != nil {
		return fmt.Errorf("new remote upstream: %v", err)
	}
	defer u.Stop()

	logrus.WithFields(logrus.Fields{
		"app-name":            cfg.ApplicationName,
		"spy-name":            spyName,
		"pid":                 pid,
		"detect-subprocesses": cfg.DetectSubprocesses,
	}).Debug("starting agent session")

	// if the sample rate is zero, use the default value
	if cfg.SampleRate == 0 {
		cfg.SampleRate = types.DefaultSampleRate
	}

	sc := agent.SessionConfig{
		Upstream:         u,
		AppName:          cfg.ApplicationName,
		ProfilingTypes:   []spy.ProfileType{spy.ProfileCPU},
		SpyName:          spyName,
		SampleRate:       uint32(cfg.SampleRate),
		UploadRate:       10 * time.Second,
		Pid:              pid,
		WithSubprocesses: cfg.DetectSubprocesses,
	}
	session := agent.NewSession(&sc, logrus.StandardLogger())
	if err := session.Start(); err != nil {
		return fmt.Errorf("start session: %v", err)
	}
	defer session.Stop()

	if isExec {
		waitForSpawnedProcessToExit(cmd)
	} else {
		waitForProcessToExit(pid)
	}

	return nil
}

// TODO: very hacky, at some point we'll need to make `cmd.Wait()` work
//   Currently the issue is that on Linux it often thinks the process exited when it did not.
func waitForSpawnedProcessToExit(cmd *exec.Cmd) {
	sigc := make(chan struct{})

	go func() {
		cmd.Wait()
	}()

	atexit.Register(func() {
		sigc <- struct{}{}
	})

	t := time.NewTicker(time.Second)
	for {
		select {
		case <-sigc:
			logrus.Debug("received a signal, killing subprocess")
			cmd.Process.Kill()
			return
		case <-t.C:
			p, err := ps.FindProcess(cmd.Process.Pid)
			if p == nil || err != nil {
				logrus.WithField("err", err).Debug("could not find subprocess, it might be dead")
				return
			}
		}
	}
}

func waitForProcessToExit(pid int) {
	// pid == -1 means we're profiling whole system
	if pid == -1 {
		select {}
	}

	t := time.NewTicker(time.Second)
	for range t.C {
		p, err := ps.FindProcess(pid)
		if p == nil || err != nil {
			logrus.WithField("err", err).Debug("could not find subprocess, it might be dead")
			return
		}
	}
}

func performChecks(spyName string) error {
	if spyName == types.GoSpy {
		return fmt.Errorf("gospy can not profile other processes. See our documentation on using gospy: %s", color.GreenString("https://pyroscope.io/docs/"))
	}

	err := performOSChecks(spyName)
	if err != nil {
		return err
	}

	if !stringsContains(spy.SupportedSpies, spyName) {
		supportedSpies := spy.SupportedExecSpies()
		return fmt.Errorf(
			"spy \"%s\" is not supported. Available spies are: %s",
			color.GreenString(spyName),
			strings.Join(supportedSpies, ","),
		)
	}

	return nil
}

func stringsContains(arr []string, element string) bool {
	for _, v := range arr {
		if v == element {
			return true
		}
	}
	return false
}

func generateSeed(args []string) string {
	path, err := os.Getwd()
	if err != nil {
		path = "<unknown>"
	}
	return path + "|" + strings.Join(args, "&")
}
