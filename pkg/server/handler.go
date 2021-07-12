package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/markbates/pkger"
	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/sirupsen/logrus"
)

func (ctrl *Controller) loginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmplt := template.New("login.html")
		tmplt, _ = tmplt.ParseFiles("./webapp/templates/login.html")
		params := map[string]bool{
			"GoogleEnabled": ctrl.config.GoogleEnabled,
			"GithubEnabled": ctrl.config.GithubEnabled,
			"GitlabEnabled": ctrl.config.GitlabEnabled,
		}

		tmplt.Execute(w, params)
	}
}

func createCookie(w http.ResponseWriter, name, value string) {
	cookie := &http.Cookie{
		Name:     name,
		Path:     "/",
		Value:    value,
		HttpOnly: true,
		MaxAge:   0,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)
}

func invalidateCookie(w http.ResponseWriter, name string) {
	cookie := &http.Cookie{
		Name:     name,
		Path:     "/",
		Value:    "",
		HttpOnly: true,
		// MaxAge -1 request cookie be deleted immediately
		MaxAge:   -1,
		SameSite: http.SameSiteStrictMode,
	}

	http.SetCookie(w, cookie)
}

func (ctrl *Controller) logoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		invalidateCookie(w, jwtCookieName)
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
	}
}

// can be replaced with a faster solution if cryptographic randomness isn't a priority
func generateStateToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func getCallbackURL(host, configCallbackURL string, oauthType int, hasTLS bool) (string, error) {
	if configCallbackURL != "" {
		return configCallbackURL, nil
	}

	if host == "" {
		return "", errors.New("host is empty")
	}

	schema := "http"
	if hasTLS {
		schema = "https"
	}

	switch oauthType {
	case oauthGoogle:
		return fmt.Sprintf("%v://%v/google/callback", schema, host), nil
	case oauthGithub:
		return fmt.Sprintf("%v://%v/github/callback", schema, host), nil
	case oauthGitlab:
		return fmt.Sprintf("%v://%v/gitlab/callback", schema, host), nil
	}

	return "", errors.New("invalid oauth type provided")
}

func (ctrl *Controller) oauthLoginHandler(info *oauthInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callbackURL, err := getCallbackURL(r.Host, info.Config.RedirectURL, info.Type, r.URL.Query().Get("tls") == "true")
		if err != nil {
			ctrl.log.WithError(err).Error("callbackURL parsing failed")
			return
		}

		authURL := *info.AuthURL
		parameters := url.Values{}
		parameters.Add("client_id", info.Config.ClientID)
		parameters.Add("scope", strings.Join(info.Config.Scopes, " "))
		parameters.Add("redirect_uri", callbackURL)
		parameters.Add("response_type", "code")

		// generate state token for CSRF protection
		state, err := generateStateToken(16)
		if err != nil {
			ctrl.log.WithError(err).Error("problem generating state token")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		createCookie(w, stateCookieName, state)
		parameters.Add("state", state)
		authURL.RawQuery = parameters.Encode()

		http.Redirect(w, r, authURL.String(), http.StatusTemporaryRedirect)
	}
}

// Instead of this handler that just redirects, Javascript code can be added to load the state and send it to backend
// this is done so that the state cookie would be send back from browser
func (ctrl *Controller) callbackHandler(redirectURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		redirectURL += "?" + r.URL.RawQuery
		tmplt := template.New("redirect.html")
		tmplt, _ = tmplt.ParseFiles("./webapp/templates/redirect.html")
		params := map[string]string{"RedirectURL": redirectURL}

		tmplt.Execute(w, params)
	}
}

func (ctrl *Controller) forbiddenHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./webapp/templates/forbidden.html")
	}
}

func (ctrl *Controller) decodeGoogleCallbackResponse(resp *http.Response) (string, error) {
	type callbackResponse struct {
		ID            string
		Email         string
		VerifiedEmail bool
		Picture       string
	}

	var userProfile callbackResponse
	err := json.NewDecoder(resp.Body).Decode(&userProfile)
	if err != nil {
		return "", err
	}

	return userProfile.Email, nil
}

func (ctrl *Controller) decodeGithubCallbackResponse(resp *http.Response) (string, error) {
	type callbackResponse struct {
		ID        int64
		Email     string
		Login     string
		AvatarURL string
	}

	var userProfile callbackResponse
	err := json.NewDecoder(resp.Body).Decode(&userProfile)
	if err != nil {
		return "", err
	}

	return userProfile.Login, nil
}

func (ctrl *Controller) decodeGitLabCallbackResponse(resp *http.Response) (string, error) {
	type callbackResponse struct {
		ID        int64
		Email     string
		Username  string
		AvatarURL string
	}

	var userProfile callbackResponse
	err := json.NewDecoder(resp.Body).Decode(&userProfile)
	if err != nil {
		return "", nil
	}

	return userProfile.Username, nil
}

func (ctrl *Controller) newJWTToken(name string) (string, error) {
	claims := jwt.MapClaims{
		"iat":  time.Now().Unix(),
		"name": name,
	}

	if ctrl.config.LoginMaximumLifetimeDays > 0 {
		claims["exp"] = time.Now().Add(time.Hour * 24 * time.Duration(ctrl.config.LoginMaximumLifetimeDays)).Unix()
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tk, err := jwtToken.SignedString([]byte(ctrl.config.JWTSecret))
	if err != nil {
		return "", err
	}

	return tk, nil
}

func (ctrl *Controller) logErrorAndRedirect(w http.ResponseWriter, r *http.Request, logString string, err error) {
	if err != nil {
		ctrl.log.WithError(err).Error(logString)
	} else {
		ctrl.log.Error(logString)
	}

	invalidateCookie(w, stateCookieName)

	http.Redirect(w, r, "/forbidden", http.StatusTemporaryRedirect)
	return
}

func (ctrl *Controller) callbackRedirectHandler(getAccountInfoURL string, info *oauthInfo, decodeResponse decodeResponseFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callbackURL, err := getCallbackURL(r.Host, info.Config.RedirectURL, info.Type, r.URL.Query().Get("tls") == "true")
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "callbackURL parsing failed", nil)
			ctrl.log.WithError(err).Error("")
			return
		}

		oauthConf := *info.Config
		oauthConf.RedirectURL = callbackURL

		cookie, err := r.Cookie(stateCookieName)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "missing state cookie", err)
			return
		}

		cookieState := cookie.Value
		requestState := r.FormValue("state")

		if requestState != cookieState {
			ctrl.logErrorAndRedirect(w, r, "invalid oauth state", nil)
			return
		}

		code := r.FormValue("code")
		if code == "" {
			ctrl.logErrorAndRedirect(w, r, "code not found", nil)
			return
		}

		token, err := oauthConf.Exchange(r.Context(), code)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "exchanging auth code for token failed", err)
			return
		}

		client := oauthConf.Client(r.Context(), token)
		resp, err := client.Get(getAccountInfoURL)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "failed to get oauth user info", err)
			return
		}
		defer resp.Body.Close()

		name, err := decodeResponse(resp)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "decoding response body failed", err)
			return
		}

		tk, err := ctrl.newJWTToken(name)
		if err != nil {
			ctrl.logErrorAndRedirect(w, r, "signing jwt failed", err)
			return
		}

		// delete state cookie and add jwt cookie
		invalidateCookie(w, stateCookieName)
		createCookie(w, jwtCookieName, tk)

		tmplt := template.New("welcome.html")
		tmplt, _ = tmplt.ParseFiles("./webapp/templates/welcome.html")
		params := map[string]string{"Name": name}

		tmplt.Execute(w, params)
		return
	}
}

func (ctrl *Controller) indexHandler() http.HandlerFunc {
	var dir http.FileSystem
	if build.UseEmbeddedAssets {
		// for this to work you need to run `pkger` first. See Makefile for more information
		dir = pkger.Dir("/webapp/public")
	} else {
		dir = http.Dir("./webapp/public")
	}
	fs := http.FileServer(dir)
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			ctrl.statsInc("index")
			ctrl.renderIndexPage(dir, rw, r)
		} else if r.URL.Path == "/comparison" {
			ctrl.statsInc("index")
			ctrl.renderIndexPage(dir, rw, r)
		} else {
			fs.ServeHTTP(rw, r)
		}
	}
}

func renderServerError(rw http.ResponseWriter, text string) {
	rw.WriteHeader(500)
	rw.Write([]byte(text))
	rw.Write([]byte("\n"))
}

type indexPageJSON struct {
	AppNames []string `json:"appNames"`
}
type indexPage struct {
	InitialState  string
	BuildInfo     string
	ExtraMetadata string
	BaseURL       string
}

func (ctrl *Controller) renderIndexPage(dir http.FileSystem, rw http.ResponseWriter, _ *http.Request) {
	f, err := dir.Open("/index.html")
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not find file index.html: %q", err))
		return
	}

	b, err := ioutil.ReadAll(f)
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not read file index.html: %q", err))
		return
	}

	tmpl, err := template.New("index.html").Parse(string(b))
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not parse index.html template: %q", err))
		return
	}

	initialStateObj := indexPageJSON{}
	ctrl.storage.GetValues("__name__", func(v string) bool {
		initialStateObj.AppNames = append(initialStateObj.AppNames, v)
		return true
	})
	b, err = json.Marshal(initialStateObj)
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not marshal initialStateObj json: %q", err))
		return
	}
	initialStateStr := string(b)

	var extraMetadataStr string
	extraMetadataPath := os.Getenv("PYROSCOPE_EXTRA_METADATA")
	if extraMetadataPath != "" {
		b, err = ioutil.ReadFile(extraMetadataPath)
		if err != nil {
			logrus.Errorf("failed to read file at %s", extraMetadataPath)
		}
		extraMetadataStr = string(b)
	}

	rw.Header().Add("Content-Type", "text/html")
	err = tmpl.Execute(rw, indexPage{
		InitialState:  initialStateStr,
		BuildInfo:     build.JSON(),
		ExtraMetadata: extraMetadataStr,
		BaseURL:       ctrl.config.BaseURL,
	})
	if err != nil {
		renderServerError(rw, fmt.Sprintf("could not marshal json: %q", err))
		return
	}
}
