package glance

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func visibilityTestApp(t *testing.T, authenticated bool) *application {
	t.Helper()
	secret := bytes.Repeat([]byte{42}, AUTH_SECRET_KEY_LENGTH)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	auth := ""
	if authenticated {
		auth = fmt.Sprintf("auth:\n  secret-key: %s\n  users:\n    admin:\n      password-hash: %s\n", base64.StdEncoding.EncodeToString(secret), passwordHash)
	}
	config, err := newConfigFromYAML([]byte(auth + `pages:
  - name: Public Home
    public: true
    columns:
      - size: full
        widgets:
          - type: html
            source: PUBLIC_HOME_MARKER
  - name: Projects
    public: true
    head-widgets:
      - type: html
        source: SHARED_PUBLIC_HEADER
    columns:
      - size: full
        widgets:
          - type: html
            source: PUBLIC_PROJECT_MARKER
    authenticated-columns:
      - size: full
        widgets:
          - type: html
            source: PRIVATE_PROJECT_MARKER
  - name: Private Vault
    columns:
      - size: full
        widgets:
          - type: html
            source: PRIVATE_VAULT_MARKER
  - name: Legacy Private
    public: false
    columns:
      - size: full
        widgets:
          - type: html
            source: LEGACY_MARKER
`))
	if err != nil {
		t.Fatal(err)
	}
	app, err := newApplication(config)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func visibilityRequest(app *application, path, session string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if session != "" {
		request.AddCookie(&http.Cookie{Name: AUTH_SESSION_COOKIE_NAME, Value: session})
	}
	response := httptest.NewRecorder()
	if strings.HasPrefix(path, "/api/pages/") {
		request.SetPathValue("page", strings.Split(path, "/")[3])
		app.handlePageContentRequest(response, request)
	} else {
		slug := strings.Trim(path, "/")
		request.SetPathValue("page", slug)
		app.handlePageRequest(response, request)
	}
	return response
}

func visibilitySession(t *testing.T, app *application, when time.Time) string {
	t.Helper()
	token, err := generateSessionToken("admin", app.authSecretKey, when)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestPageVisibilityGuestAndAuthenticated(t *testing.T) {
	app := visibilityTestApp(t, true)
	session := visibilitySession(t, app, time.Now())
	for _, test := range []struct {
		name, path, session string
		status              int
		contains, absent    []string
	}{
		{"guest home", "/", "", 200, []string{"Public Home", `/projects`, `/login`}, []string{"Private Vault", "Legacy Private", `/logout`}},
		{"guest public direct", "/projects", "", 200, []string{"Projects", `/login`}, []string{"Private Vault", "Legacy Private", "/logout"}},
		{"guest public content", "/api/pages/projects/content/", "", 200, []string{"PUBLIC_PROJECT_MARKER", "SHARED_PUBLIC_HEADER"}, []string{"PRIVATE_PROJECT_MARKER", "PRIVATE_VAULT_MARKER"}},
		{"guest private direct", "/private-vault", "", 303, []string{"/login"}, []string{"PRIVATE_VAULT_MARKER"}},
		{"guest private content", "/api/pages/private-vault/content/", "", 401, nil, []string{"PRIVATE_VAULT_MARKER"}},
		{"guest legacy direct", "/legacy-private", "", 303, []string{"/login"}, nil},
		{"guest legacy content", "/api/pages/legacy-private/content/", "", 401, nil, []string{"LEGACY_MARKER"}},
		{"member home", "/", session, 200, []string{"Private Vault", "Legacy Private", `/logout`}, []string{`/login`}},
		{"member projects", "/projects", session, 200, []string{"Private Vault", `/logout`}, []string{`/login`}},
		{"member projects content", "/api/pages/projects/content/", session, 200, []string{"PRIVATE_PROJECT_MARKER", "SHARED_PUBLIC_HEADER"}, []string{"PUBLIC_PROJECT_MARKER"}},
		{"member private content", "/api/pages/private-vault/content/", session, 200, []string{"PRIVATE_VAULT_MARKER"}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := visibilityRequest(app, test.path, test.session)
			if response.Code != test.status {
				t.Fatalf("status=%d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
			body := response.Body.String() + response.Header().Get("Location")
			for _, marker := range test.contains {
				if !strings.Contains(body, marker) {
					t.Errorf("missing %q in %s", marker, body)
				}
			}
			for _, marker := range test.absent {
				if strings.Contains(body, marker) {
					t.Errorf("leaked %q", marker)
				}
			}
			if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Errorf("Cache-Control=%q", got)
			}
			if got := response.Header().Get("Vary"); got != "Cookie" {
				t.Errorf("Vary=%q", got)
			}
		})
	}
}

func TestPageVisibilityForgedExpiredAndLogout(t *testing.T) {
	app := visibilityTestApp(t, true)
	for _, session := range []string{"forged", visibilitySession(t, app, time.Now().Add(-AUTH_TOKEN_VALID_PERIOD-time.Hour))} {
		if response := visibilityRequest(app, "/api/pages/private-vault/content/", session); response.Code != http.StatusUnauthorized {
			t.Errorf("invalid session private content: %d", response.Code)
		}
		response := visibilityRequest(app, "/api/pages/projects/content/", session)
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "PRIVATE_PROJECT_MARKER") {
			t.Errorf("invalid session public view: %d %s", response.Code, response.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/logout", nil)
	request.AddCookie(&http.Cookie{Name: AUTH_SESSION_COOKIE_NAME, Value: visibilitySession(t, app, time.Now())})
	response := httptest.NewRecorder()
	app.handleLogoutRequest(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/" {
		t.Errorf("logout: %d %q", response.Code, response.Header().Get("Location"))
	}
	if len(response.Result().Cookies()) == 0 || response.Result().Cookies()[0].Value != "" {
		t.Fatal("logout did not clear session cookie")
	}
	if page := visibilityRequest(app, "/api/pages/projects/content/", ""); strings.Contains(page.Body.String(), "PRIVATE_PROJECT_MARKER") {
		t.Fatal("private data visible after logout")
	}
}

func TestPageVisibilityConfigDefaults(t *testing.T) {
	app := visibilityTestApp(t, true)
	if app.Config.Pages[2].Public || app.Config.Pages[3].Public {
		t.Fatal("unmarked or explicitly private pages must not become public")
	}
	if _, err := newConfigFromYAML([]byte(`pages:
  - name: Public
    public: true
    columns:
      - size: full
    authenticated-columns:
      - size: full
`)); err == nil {
		t.Fatal("authenticated-columns without auth must fail closed")
	}
	secret := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{42}, AUTH_SECRET_KEY_LENGTH))
	if _, err := newConfigFromYAML([]byte(fmt.Sprintf(`auth:
  secret-key: %s
  users:
    admin:
      password: password123
pages:
  - name: Private
    columns:
      - size: full
    authenticated-columns:
      - size: full
`, secret))); err == nil {
		t.Fatal("authenticated-columns on a non-public page must be rejected")
	}
}

func TestPageVisibilityWithoutAuthPreservesExistingBehavior(t *testing.T) {
	config, err := newConfigFromYAML([]byte(`pages:
  - name: Old Home
    columns:
      - size: full
        widgets:
          - type: html
            source: OLD_CONTENT_MARKER
`))
	if err != nil {
		t.Fatal(err)
	}
	app, err := newApplication(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/api/pages/old-home/content/"} {
		response := visibilityRequest(app, path, "")
		if response.Code != http.StatusOK {
			t.Errorf("legacy unauthenticated %s: %d", path, response.Code)
		}
	}
	if response := visibilityRequest(app, "/api/pages/old-home/content/", ""); !strings.Contains(response.Body.String(), "OLD_CONTENT_MARKER") {
		t.Errorf("legacy unauthenticated content missing: %s", response.Body.String())
	}
}

func TestPageVisibilityLoginAndNavigation(t *testing.T) {
	app := visibilityTestApp(t, true)
	guest := visibilityRequest(app, "/projects", "")
	if got := strings.Count(guest.Body.String(), `href="/login"`); got != 2 {
		t.Errorf("guest desktop/mobile login links: %d", got)
	}
	if got := strings.Count(guest.Body.String(), `href="/private-vault"`); got != 0 {
		t.Errorf("guest private navigation links: %d", got)
	}

	loginPage := httptest.NewRecorder()
	app.handleLoginPageRequest(loginPage, httptest.NewRequest(http.MethodGet, "/login", nil))
	if loginPage.Code != http.StatusOK || !strings.Contains(loginPage.Body.String(), `id="login-button"`) {
		t.Fatalf("login page: %d %s", loginPage.Code, loginPage.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, "/api/authenticate", strings.NewReader(`{"username":"admin","password":"password123"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.handleAuthenticationAttempt(response, request)
	if response.Code != http.StatusOK || len(response.Result().Cookies()) != 1 {
		t.Fatalf("login: %d %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("login cache policy: %q", got)
	}
	session := response.Result().Cookies()[0].Value
	member := visibilityRequest(app, "/projects", session)
	if member.Code != http.StatusOK || strings.Count(member.Body.String(), `href="/private-vault"`) != 2 {
		t.Errorf("member private navigation: %d %s", member.Code, member.Body.String())
	}
	if got := strings.Count(member.Body.String(), `href="/logout"`); got != 2 {
		t.Errorf("member desktop/mobile logout links: %d", got)
	}

	forged := httptest.NewRequest(http.MethodGet, "/api/pages/projects/content/", nil)
	forged.SetPathValue("page", "projects")
	forged.Header.Set("X-User", "admin")
	forged.Header.Set("X-Forwarded-User", "admin")
	forged.URL.RawQuery = "audience=private"
	forgedResponse := httptest.NewRecorder()
	app.handlePageContentRequest(forgedResponse, forged)
	if strings.Contains(forgedResponse.Body.String(), "PRIVATE_PROJECT_MARKER") {
		t.Fatal("client-supplied audience or identity exposed private content")
	}
}

func TestPageVisibilityLogoutWhenHomePrivate(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{42}, AUTH_SECRET_KEY_LENGTH))
	config, err := newConfigFromYAML([]byte(fmt.Sprintf(`auth:
  secret-key: %s
  users:
    admin:
      password: password123
pages:
  - name: Private Home
    columns:
      - size: full
  - name: Public Projects
    public: true
    columns:
      - size: full
`, secret)))
	if err != nil {
		t.Fatal(err)
	}
	app, err := newApplication(config)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.handleLogoutRequest(response, httptest.NewRequest(http.MethodGet, "/logout", nil))
	if got := response.Header().Get("Location"); got != "/public-projects" {
		t.Errorf("logout redirect=%q", got)
	}
	if guest := visibilityRequest(app, "/", ""); guest.Code != http.StatusSeeOther {
		t.Errorf("private home guest response=%d", guest.Code)
	}
}
