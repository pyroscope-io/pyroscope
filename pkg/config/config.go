package config

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type Config struct {
	Version bool

	Agent     Agent     `skip:"true"`
	Server    Server    `skip:"true"`
	Convert   Convert   `skip:"true"`
	Exec      Exec      `skip:"true"`
	DbManager DbManager `skip:"true"`
}

type Agent struct {
	Config string `def:"<defaultAgentConfigPath>" desc:"location of config file"`

	LogFilePath string `def:"<defaultAgentLogFilePath>" desc:"log file path"`
	LogLevel    string `def:"info" desc:"log level: debug|info|warn|error"`
	NoLogging   bool   `def:"false" desc:"disables logging from pyroscope"`

	ServerAddress          string        `def:"http://localhost:4040" desc:"address of the pyroscope server"`
	AuthToken              string        `def:"" desc:"authorization token used to upload profiling data"`
	UpstreamThreads        int           `def:"4" desc:"number of upload threads"`
	UpstreamRequestTimeout time.Duration `def:"10s" desc:"profile upload timeout"`

	Targets []Target `desc:"list of targets to be profiled"`
}

type Target struct {
	ServiceName string `yaml:"service-name" desc:"name of the system service to be profiled"`

	SpyName            string `yaml:"spy-name" def:"" desc:"name of the profiler you want to use. Supported ones are: <supportedProfilers>"`
	ApplicationName    string `yaml:"application-name" def:"" desc:"application name used when uploading profiling data"`
	SampleRate         uint   `yaml:"sample-rate" def:"100" desc:"sample rate for the profiler in Hz. 100 means reading 100 times per second"`
	DetectSubprocesses bool   `yaml:"detect-subprocesses" def:"true" desc:"makes pyroscope keep track of and profile subprocesses of the main process"`

	// Spy-specific settings.

	PyspyBlocking bool `yaml:"pyspy-blocking" def:"false" desc:"enables blocking mode for pyspy"`
	RbspyBlocking bool `yaml:"rbspy-blocking" def:"false" desc:"enables blocking mode for rbspy"`
}

type Server struct {
	AnalyticsOptOut bool `def:"false" desc:"disables analytics"`

	Config         string `def:"<installPrefix>/etc/pyroscope/server.yml" desc:"location of config file"`
	LogLevel       string `def:"info" desc:"log level: debug|info|warn|error"`
	BadgerLogLevel string `def:"error" desc:"log level: debug|info|warn|error"`

	StoragePath string `def:"<installPrefix>/var/lib/pyroscope" desc:"directory where pyroscope stores profiling data"`
	APIBindAddr string `def:":4040" desc:"port for the HTTP server used for data ingestion and web UI"`
	BaseURL     string `def:"" desc:"base URL for when the server is behind a reverse proxy with a different path"`

	CacheEvictThreshold float64 `def:"0.25" desc:"percentage of memory at which cache evictions start"`
	CacheEvictVolume    float64 `def:"0.33" desc:"percentage of cache that is evicted per eviction run"`

	// TODO: I don't think a lot of people will change these values.
	//   I think these should just be constants.
	BadgerNoTruncate     bool `def:"false" desc:"indicates whether value log files should be truncated to delete corrupt data, if any"`
	DisablePprofEndpoint bool `def:"false" desc:"disables /debug/pprof route"`

	MaxNodesSerialization int `def:"2048" desc:"max number of nodes used when saving profiles to disk"`
	MaxNodesRender        int `def:"8192" desc:"max number of nodes used to display data on the frontend"`

	// currently only used in our demo app
	HideApplications []string `def:"" desc:"please don't use, this will soon be deprecated"`

	Retention time.Duration `def:"" desc:"sets the maximum amount of time the profiling data is stored for. Data before this threshold is deleted. Disabled by default"`

	// Deprecated fields. They can be set (for backwards compatibility) but have no effect
	// TODO: we should print some warning messages when people try to use these
	SampleRate          uint              `deprecated:"true"`
	OutOfSpaceThreshold bytesize.ByteSize `deprecated:"true"`
	CacheDimensionSize  int               `deprecated:"true"`
	CacheDictionarySize int               `deprecated:"true"`
	CacheSegmentSize    int               `deprecated:"true"`
	CacheTreeSize       int               `deprecated:"true"`

	GoogleEnabled      bool   `def:"false" desc:"enables Google Oauth"`
	GoogleClientID     string `def:"<yourClientID>" desc:"client ID generated for Google API"`
	GoogleClientSecret string `def:"<yourClientSecret>" desc:"client secret generated for Google API"`
	GoogleRedirectURL  string `def:"" desc:"url that google will redirect to after logging in. Has to be in form <pathToPyroscopeServer/google/callback>"`
	GoogleAuthURL      string `def:"https://accounts.google.com/o/oauth2/auth" desc:"auth url for Google API (usually present in credentials.json file)"`
	GoogleTokenURL     string `def:"https://accounts.google.com/o/oauth2/token" desc:"token url for Google API (usually present in credentials.json file)"`

	GitlabEnabled       bool   `def:"false" desc:"enables Gitlab Oauth"`
	GitlabApplicationID string `def:"<yourApplicationID>" desc:"application ID generated for GitLab API"`
	GitlabClientSecret  string `def:"<yourClientSecret>" desc:"client secret generated for GitLab API"`
	GitlabRedirectURL   string `def:"" desc:"url that gitlab will redirect to after logging in. Has to be in form <pathToPyroscopeServer/gitlab/callback>"`
	GitlabAuthURL       string `def:"https://gitlab.com/oauth/authorize" desc:"auth url for GitLab API (keep default for cloud, usually https://gitlab.mycompany.com/oauth/authorize for on-premise)"`
	GitlabTokenURL      string `def:"https://gitlab.com/oauth/token" desc:"token url for GitLab API (keep default for cloud, usually https://gitlab.mycompany.com/oauth/token for on-premise)"`
	GitlabAPIURL        string `def:"https://gitlab.com/api/v4/user" desc:"URL to gitlab API (keep default for cloud, usually https://gitlab.mycompany.com/api/v4/user for on-premise)"`

	GithubEnabled      bool   `def:"false" desc:"enables Github Oauth"`
	GithubClientID     string `def:"<yourClientID>" desc:"client ID generated for Github API"`
	GithubClientSecret string `def:"<yourClientSecret>" desc:"client secret generated for Github API"`
	GithubRedirectURL  string `def:"" desc:"url that Github will redirect to after logging in. Has to be in form <pathToPyroscopeServer/github/callback>"`
	GithubAuthURL      string `def:"https://github.com/login/oauth/authorize" desc:"auth url for Github API"`
	GithubTokenURL     string `def:"https://github.com/login/oauth/access_token" desc:"token url for Github API"`

	JWTSecret                string `def:"" desc:"secret used to secure your JWT tokens"`
	LoginMaximumLifetimeDays int    `def:"0" desc:"amount of days after which user will be logged out. 0 means non-expiring."`
}

type Convert struct {
	Format string `def:"tree"`
}

type DbManager struct {
	LogLevel        string `def:"error" desc:"log level: debug|info|warn|error"`
	StoragePath     string `def:"<installPrefix>/var/lib/pyroscope" desc:"directory where pyroscope stores profiling data"`
	DstStartTime    time.Time
	DstEndTime      time.Time
	SrcStartTime    time.Time
	ApplicationName string

	EnableProfiling bool `def:"false" desc:"enables profiling of dbmanager"`
}

type Exec struct {
	SpyName                string        `def:"auto" desc:"name of the profiler you want to use. Supported ones are: <supportedProfilers>"`
	ApplicationName        string        `def:"" desc:"application name used when uploading profiling data"`
	SampleRate             uint          `def:"100" desc:"sample rate for the profiler in Hz. 100 means reading 100 times per second"`
	DetectSubprocesses     bool          `def:"true" desc:"makes pyroscope keep track of and profile subprocesses of the main process"`
	LogLevel               string        `def:"info" desc:"log level: debug|info|warn|error"`
	ServerAddress          string        `def:"http://localhost:4040" desc:"address of the pyroscope server"`
	AuthToken              string        `def:"" desc:"authorization token used to upload profiling data"`
	UpstreamThreads        int           `def:"4" desc:"number of upload threads"`
	UpstreamRequestTimeout time.Duration `def:"10s" desc:"profile upload timeout"`
	NoLogging              bool          `def:"false" desc:"disables logging from pyroscope"`
	NoRootDrop             bool          `def:"false" desc:"disables permissions drop when ran under root. use this one if you want to run your command as root"`
	Pid                    int           `def:"0" desc:"PID of the process you want to profile. Pass -1 to profile the whole system (only supported by ebpfspy)"`
	UserName               string        `def:"" desc:"starts process under specified user name"`
	GroupName              string        `def:"" desc:"starts process under specified group name"`
	PyspyBlocking          bool          `def:"false" desc:"enables blocking mode for pyspy"`
	RbspyBlocking          bool          `def:"false" desc:"enables blocking mode for rbspy"`
}
