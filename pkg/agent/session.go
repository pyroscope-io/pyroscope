package agent

import (
	"sync"
	"time"

	// revive:disable:blank-imports Depending on configuration these packages may or may not be used.
	//   That's why we do a blank import here and then packages themselves register with the rest of the code.

	_ "github.com/pyroscope-io/pyroscope/pkg/agent/debugspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/dotnetspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/gospy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/phpspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/rbspy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"

	// revive:enable:blank-imports

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

// Each Session can deal with:
// * multiple processes (one main process and zero or more subprocesses)
// * multiple profile types (cpu, mem, etc)
// * multiple names (app.cpu{} or app.cpu{controller=foo}) (one at a a time)

/*
                PROCESSES
            ┌─────┬─────┬─────┐
            │pid 1│pid 2│pid 3│
            └──┬──┴──┬──┴──┬──┘
               │     │     │          NAMES/TAGS
               │     │     │            ┌─app.cpu{}
             0 ▼   1 ▼   2 ▼            │     ┌─app.cpu{controller=bar}
            ┌─────┬─────┬─────┐      ┌─────┬─────┐     ┌──────┐
     0 cpu  │     │     │     │ ───► │     │     │ ──► │      │
            └─────┴─────┴─────┘      └─────┴─────┘     │      │
PROFILE TYPES      SPIES                 TRIES     ──► │server│
            ┌─────┬─────┬─────┐      ┌─────┬─────┐     │      │
     1 mem  │     │     │     │ ───► │     │     │ ──► │      │
            └─────┴─────┴─────┘      └─────┴─────┘     └──────┘
*/
type ProfileSession struct {
	upstream   upstream.Upstream
	appName    string
	spyName    string
	sampleRate uint32
	uploadRate time.Duration
	pids       []int
	spies      [][]spy.Spy
	stopCh     chan struct{}
	trieMutex  sync.Mutex

	// see comment about multiple dimensions above
	previousTries map[string][]*transporttrie.Trie
	tries         map[string][]*transporttrie.Trie

	profileTypes     []spy.ProfileType
	disableGCRuns    bool
	withSubprocesses bool

	startTime time.Time

	logger Logger
}

type SessionConfig struct {
	Upstream         upstream.Upstream
	AppName          string
	ProfilingTypes   []spy.ProfileType
	DisableGCRuns    bool
	SpyName          string
	SampleRate       uint32
	UploadRate       time.Duration
	Pid              int
	WithSubprocesses bool
}

func NewSession(c *SessionConfig, logger Logger) *ProfileSession {
	ps := &ProfileSession{
		upstream:         c.Upstream,
		appName:          c.AppName,
		spyName:          c.SpyName,
		profileTypes:     c.ProfilingTypes,
		disableGCRuns:    c.DisableGCRuns,
		sampleRate:       c.SampleRate,
		uploadRate:       c.UploadRate,
		pids:             []int{c.Pid},
		stopCh:           make(chan struct{}),
		withSubprocesses: c.WithSubprocesses,
		logger:           logger,

		// string is appName, int is index in pids
		previousTries: make(map[string][]*transporttrie.Trie),
		tries:         make(map[string][]*transporttrie.Trie),
	}

	ps.previousTries[ps.appName] = []*transporttrie.Trie{nil}
	ps.tries[ps.appName] = []*transporttrie.Trie{transporttrie.New()}

	return ps
}

func (ps *ProfileSession) takeSnapshots() {
	ticker := time.NewTicker(time.Second / time.Duration(ps.sampleRate))
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			isdueToReset := ps.isDueForReset()
			// reset the profiler for spies every upload rate(10s), and before uploading, it needs to read profile data every sample rate
			if isdueToReset {
				for _, sarr := range ps.spies {
					for _, s := range sarr {
						if sr, ok := s.(spy.Resettable); ok {
							sr.Reset()
						}
					}
				}
			}

			for _, sarr := range ps.spies {
				for i, s := range sarr {
					s.Snapshot(func(stack []byte, v uint64, err error) {
						if err != nil {
							// TODO: figure out what to do with these messages. A couple of considerations:
							// * We probably shouldn't just suppress these messages as they might be useful for users
							// * We probably want to throttle the messages because this is code that runs 100 times per second.
							//   If we don't throttle we risk upsetting users with a flood of messages
							// * In gospy case we need to add ability for users to bring their own logger, we can't just use logrus here
							return
						}
						if len(stack) > 0 {
							ps.trieMutex.Lock()
							defer ps.trieMutex.Unlock()

							ps.tries[ps.appName][i].Insert(stack, v, true)
						}
					})
				}
			}

			// upload the read data to server and reset the start time
			if isdueToReset {
				ps.reset()
			}

		case <-ps.stopCh:
			// stop the spies
			for _, sarr := range ps.spies {
				for _, s := range sarr {
					s.Stop()
				}
			}
			return
		}
	}
}

func (ps *ProfileSession) initializeSpies(pid int) ([]spy.Spy, error) {
	res := []spy.Spy{}

	sf, err := spy.StartFunc(ps.spyName)
	if err != nil {
		return res, err
	}

	for _, pt := range ps.profileTypes {
		s, err := sf(pid, pt, ps.sampleRate, ps.disableGCRuns)

		if err != nil {
			return res, err
		}
		res = append(res, s)
	}
	return res, nil
}

func (ps *ProfileSession) ChangeName(newName string) error {
	ps.trieMutex.Lock()
	defer ps.trieMutex.Unlock()

	if _, ok := ps.previousTries[newName]; !ok {
		// TODO Only set the trie if it's not already set
		ps.previousTries[newName] = []*transporttrie.Trie{nil}
		ps.tries[newName] = []*transporttrie.Trie{}
		for i := 0; i < len(ps.pids); i++ {
			ps.previousTries[newName] = append(ps.previousTries[newName], nil)
			ps.tries[newName] = append(ps.tries[newName], transporttrie.New())
		}
	}

	ps.appName = newName

	return nil
}

func (ps *ProfileSession) Start() error {
	ps.reset()

	pid := -1
	if len(ps.pids) > 0 {
		pid = ps.pids[0]
	}
	spies, err := ps.initializeSpies(pid)
	if err != nil {
		return err
	}

	ps.spies = append(ps.spies, spies)

	go ps.takeSnapshots()
	return nil
}

func (ps *ProfileSession) isDueForReset() bool {
	// TODO: duration should be either taken from config or ideally passed from server
	now := time.Now().Truncate(ps.uploadRate)
	start := ps.startTime.Truncate(ps.uploadRate)

	return !start.Equal(now)
}

// the difference between stop and reset is that reset stops current session
// and then instantly starts a new one
func (ps *ProfileSession) reset() {
	ps.trieMutex.Lock()
	defer ps.trieMutex.Unlock()

	now := time.Now()
	// upload the read data to server
	ps.uploadTries(now)

	// reset the start time
	ps.startTime = now

	if ps.withSubprocesses {
		ps.addSubprocesses()
	}
}

func (ps *ProfileSession) Stop() {
	ps.trieMutex.Lock()
	defer ps.trieMutex.Unlock()

	close(ps.stopCh)
	// TODO: wait for stopCh consumer to finish!

	// before stopping, upload the tries
	ps.uploadTries(time.Now())
}

func (ps *ProfileSession) uploadTries(now time.Time) {
	for name, tarr := range ps.tries {
		for i, trie := range tarr {
			profileType := ps.profileTypes[i]
			skipUpload := false

			if trie != nil {
				endTime := now.Truncate(ps.uploadRate)

				uploadTrie := trie
				if profileType.IsCumulative() {
					previousTrie := ps.previousTries[name][i]
					if previousTrie == nil {
						skipUpload = true
					} else {
						// TODO: Diff doesn't remove empty branches. We need to add that at some point
						uploadTrie = trie.Diff(previousTrie)
					}
				}

				if !skipUpload {
					name := ps.appName + "." + string(profileType)
					ps.upstream.Upload(&upstream.UploadJob{
						Name:            name,
						StartTime:       ps.startTime,
						EndTime:         endTime,
						SpyName:         ps.spyName,
						SampleRate:      ps.sampleRate,
						Units:           profileType.Units(),
						AggregationType: profileType.AggregationType(),
						Trie:            uploadTrie,
					})
				}
				if profileType.IsCumulative() {
					ps.previousTries[name][i] = trie
				}
			}
			ps.tries[name][i] = transporttrie.New()
		}
	}
}

func (ps *ProfileSession) addSubprocesses() {
	newPids := findAllSubprocesses(ps.pids[0])
	for _, newPid := range newPids {
		if !slices.IntContains(ps.pids, newPid) {
			ps.pids = append(ps.pids, newPid)
			newSpies, err := ps.initializeSpies(newPid)
			if err != nil {
				if ps.logger != nil {
					ps.logger.Errorf("failed to initialize a spy %d [%s]", newPid, ps.spyName)
				}
			} else {
				if ps.logger != nil {
					ps.logger.Debugf("started spy for subprocess %d [%s]", newPid, ps.spyName)
				}
				ps.spies = append(ps.spies, newSpies)
			}
		}
	}
}
