package server

import (
	"context"
	"fmt"
	golog "log"
	"net/http"
	"net/http/pprof"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/hyperloglog"
)

const (
	jwtCookieName   = "pyroscopeJWT"
	stateCookieName = "pyroscopeState"
	oauthGoogle     = iota
	oauthGithub
	oauthGitlab
)

type decodeResponseFunc func(*http.Response) (string, error)

type Controller struct {
	drained uint32

	config     *config.Server
	storage    *storage.Storage
	log        *logrus.Logger
	httpServer *http.Server

	statsMutex sync.Mutex
	stats      map[string]int

	appStats *hyperloglog.HyperLogLogPlus
}

func New(c *config.Server, s *storage.Storage, l *logrus.Logger) (*Controller, error) {
	appStats, err := hyperloglog.NewPlus(uint8(18))
	if err != nil {
		return nil, err
	}

	ctrl := Controller{
		config:   c,
		log:      l,
		storage:  s,
		stats:    make(map[string]int),
		appStats: appStats,
	}

	return &ctrl, nil
}

func (ctrl *Controller) mux() http.Handler {
	mux := http.NewServeMux()
	addRoutes(mux, []route{
		{"/healthz", ctrl.healthz},
		{"/metrics", promhttp.Handler().ServeHTTP},
		{"/config", ctrl.configHandler},
		{"/build", ctrl.buildHandler},
	})

	// auth routes
	addRoutes(mux, ctrl.getAuthRoutes(), ctrl.drainMiddleware)

	// drainable routes:
	routes := []route{
		{"/", ctrl.indexHandler()},
		{"/render", ctrl.renderHandler},
		{"/labels", ctrl.labelsHandler},
		{"/label-values", ctrl.labelValuesHandler},
	}

	addRoutes(mux, routes, ctrl.drainMiddleware, ctrl.authMiddleware)

	if !ctrl.config.DisablePprofEndpoint {
		addRoutes(mux, []route{
			{"/debug/pprof/", pprof.Index},
			{"/debug/pprof/cmdline", pprof.Cmdline},
			{"/debug/pprof/profile", pprof.Profile},
			{"/debug/pprof/symbol", pprof.Symbol},
			{"/debug/pprof/trace", pprof.Trace},
		})
	}

	nonAuthRoutes := []route{
		{"/ingest", ctrl.ingestHandler},
		{"/forbidden", ctrl.forbiddenHandler()},
	}

	addRoutes(mux, nonAuthRoutes, ctrl.drainMiddleware)
	return mux
}

type oauthInfo struct {
	Config  *oauth2.Config
	AuthURL *url.URL
	Type    int
}

func (ctrl *Controller) generateOauthInfo(oauthType int) *oauthInfo {
	switch oauthType {
	case oauthGoogle:
		googleOauthInfo := &oauthInfo{
			Config: &oauth2.Config{
				ClientID:     ctrl.config.GoogleClientID,
				ClientSecret: ctrl.config.GoogleClientSecret,
				Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email"},
				Endpoint:     oauth2.Endpoint{AuthURL: ctrl.config.GoogleAuthURL, TokenURL: ctrl.config.GoogleTokenURL},
			},
			Type: oauthGoogle,
		}
		if ctrl.config.GoogleRedirectURL != "" {
			googleOauthInfo.Config.RedirectURL = ctrl.config.GoogleRedirectURL
		}

		return googleOauthInfo
	case oauthGithub:
		githubOauthInfo := &oauthInfo{
			Config: &oauth2.Config{
				ClientID:     ctrl.config.GithubClientID,
				ClientSecret: ctrl.config.GithubClientSecret,
				Scopes:       []string{"read:user", "user:email"},
				Endpoint:     oauth2.Endpoint{AuthURL: ctrl.config.GithubAuthURL, TokenURL: ctrl.config.GithubTokenURL},
			},
			Type: oauthGithub,
		}

		if ctrl.config.GithubRedirectURL != "" {
			githubOauthInfo.Config.RedirectURL = ctrl.config.GithubRedirectURL
		}

		return githubOauthInfo
	case oauthGitlab:
		gitlabOauthInfo := &oauthInfo{
			Config: &oauth2.Config{
				ClientID:     ctrl.config.GitlabApplicationID,
				ClientSecret: ctrl.config.GitlabClientSecret,
				Scopes:       []string{"read_user"},
				Endpoint:     oauth2.Endpoint{AuthURL: ctrl.config.GitlabAuthURL, TokenURL: ctrl.config.GitlabTokenURL},
			},
			Type: oauthGitlab,
		}

		if ctrl.config.GitlabRedirectURL != "" {
			gitlabOauthInfo.Config.RedirectURL = ctrl.config.GitlabRedirectURL
		}

		return gitlabOauthInfo
	}

	return nil
}

func (ctrl *Controller) getAuthRoutes() []route {
	authRoutes := []route{
		{"/login", ctrl.loginHandler()},
		{"/logout", ctrl.logoutHandler()},
	}

	if ctrl.config.GoogleEnabled {
		authURL, err := url.Parse(ctrl.config.GoogleAuthURL)
		if err != nil {
			ctrl.log.WithError(err).Error("Problem parsing google auth url")
		}

		googleOauthInfo := ctrl.generateOauthInfo(oauthGoogle)
		if err == nil && googleOauthInfo != nil {
			googleOauthInfo.AuthURL = authURL
			authRoutes = append(authRoutes, []route{
				{"/google/login", ctrl.oauthLoginHandler(googleOauthInfo)},
				{"/google/callback", ctrl.callbackHandler("/google/redirect")},
				{"/google/redirect", ctrl.callbackRedirectHandler(
					"https://www.googleapis.com/oauth2/v2/userinfo", googleOauthInfo, ctrl.decodeGoogleCallbackResponse)},
			}...)
		}
	}

	if ctrl.config.GithubEnabled {
		authURL, err := url.Parse(ctrl.config.GithubAuthURL)
		if err != nil {
			ctrl.log.WithError(err).Error("Problem parsing github auth url")
			return nil
		}

		githubOauthInfo := ctrl.generateOauthInfo(oauthGithub)
		if err == nil && githubOauthInfo != nil {
			githubOauthInfo.AuthURL = authURL
			authRoutes = append(authRoutes, []route{
				{"/github/login", ctrl.oauthLoginHandler(githubOauthInfo)},
				{"/github/callback", ctrl.callbackHandler("/github/redirect")},
				{"/github/redirect", ctrl.callbackRedirectHandler("https://api.github.com/user", githubOauthInfo, ctrl.decodeGithubCallbackResponse)},
			}...)
		}
	}

	if ctrl.config.GitlabEnabled {
		authURL, err := url.Parse(ctrl.config.GitlabAuthURL)
		if err != nil {
			ctrl.log.WithError(err).Error("Problem parsing gitlab auth url")
			return nil
		}

		gitlabOauthInfo := ctrl.generateOauthInfo(oauthGitlab)
		if err == nil && gitlabOauthInfo != nil {
			gitlabOauthInfo.AuthURL = authURL
			authRoutes = append(authRoutes, []route{
				{"/gitlab/login", ctrl.oauthLoginHandler(gitlabOauthInfo)},
				{"/gitlab/callback", ctrl.callbackHandler("/gitlab/redirect")},
				{"/gitlab/redirect", ctrl.callbackRedirectHandler(ctrl.config.GitlabAPIURL, gitlabOauthInfo, ctrl.decodeGitLabCallbackResponse)},
			}...)
		}
	}

	return authRoutes
}

func (ctrl *Controller) Start() error {
	logger := logrus.New()
	w := logger.Writer()
	defer w.Close()

	ctrl.httpServer = &http.Server{
		Addr:           ctrl.config.APIBindAddr,
		Handler:        ctrl.mux(),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    30 * time.Second,
		MaxHeaderBytes: 1 << 20,
		ErrorLog:       golog.New(w, "", 0),
	}

	// ListenAndServe always returns a non-nil error. After Shutdown or Close,
	// the returned error is ErrServerClosed.
	err := ctrl.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return fmt.Errorf("listen and serve: %v", err)
}

func (ctrl *Controller) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	return ctrl.httpServer.Shutdown(ctx)
}

func (ctrl *Controller) Drain() {
	atomic.StoreUint32(&ctrl.drained, 1)
}

func (ctrl *Controller) drainMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadUint32(&ctrl.drained) > 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (ctrl *Controller) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jwtCookie, err := r.Cookie(jwtCookieName)
		if err != nil {
			ctrl.log.Error("missing jwt cookie")
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		_, err = jwt.Parse(jwtCookie.Value, func(token *jwt.Token) (interface{}, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(ctrl.config.JWTSecret), nil
		})

		if err != nil {
			ctrl.log.WithError(err).Error("parsing jwt token")
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		next.ServeHTTP(w, r)
	}
}
