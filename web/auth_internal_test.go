package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const testPassword = "correct horse battery staple"

func newAuthTestServer(t *testing.T, opts ...ServerOption) *Server {
	t.Helper()

	srv, err := New(NewService(&mockJobRepo{}, t.TempDir()), ":0", opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	srv.loginDelay = 0

	return srv
}

func serve(srv *Server, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(rec, req)

	return rec
}

func TestAuthDisabledByDefault(t *testing.T) {
	srv := newAuthTestServer(t)

	rec := serve(srv, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 without a password configured", rec.Code)
	}

	if strings.Contains(rec.Body.String(), `href="/logout"`) {
		t.Fatal("sign-out link must be hidden when auth is off")
	}
}

func TestAuthRedirectsAndProtects(t *testing.T) {
	srv := newAuthTestServer(t, WithPassword(testPassword))

	rec := serve(srv, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("page: got %d %q, want redirect to /login", rec.Code, rec.Header().Get("Location"))
	}

	htmx := httptest.NewRequest(http.MethodGet, "/jobs", http.NoBody)
	htmx.Header.Set("HX-Request", "true")

	rec = serve(srv, htmx)
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("HX-Redirect") != "/login" {
		t.Fatalf("htmx: got %d %q", rec.Code, rec.Header().Get("HX-Redirect"))
	}

	rec = serve(srv, httptest.NewRequest(http.MethodGet, "/api/v1/jobs", http.NoBody))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("api: got %d, want 401", rec.Code)
	}

	api := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", http.NoBody)
	api.SetBasicAuth("team", testPassword)

	if rec = serve(srv, api); rec.Code != http.StatusOK {
		t.Fatalf("api with basic auth: got %d, want 200", rec.Code)
	}

	// Share pages, their downloads and static assets stay public.
	for _, path := range []string{"/share/11111111-1111-1111-1111-111111111111", "/download?id=11111111-1111-1111-1111-111111111111", "/static/css/main.css", "/login"} {
		if rec = serve(srv, httptest.NewRequest(http.MethodGet, path, http.NoBody)); rec.Code == http.StatusSeeOther || rec.Code == http.StatusUnauthorized {
			t.Fatalf("%s must be public, got %d", path, rec.Code)
		}
	}
}

func TestLoginFlow(t *testing.T) {
	srv := newAuthTestServer(t, WithPassword(testPassword))

	post := func(password string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{"password": {password}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		return serve(srv, req)
	}

	if rec := post("wrong"); rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "isn&#39;t right") {
		t.Fatalf("wrong password: got %d", rec.Code)
	}

	rec := post(testPassword)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("login: got %d %q", rec.Code, rec.Header().Get("Location"))
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || !cookies[0].HttpOnly {
		t.Fatalf("unexpected cookies: %+v", cookies)
	}

	page := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	page.AddCookie(cookies[0])

	if rec = serve(srv, page); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `href="/logout"`) {
		t.Fatalf("signed in: got %d", rec.Code)
	}

	tampered := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	tampered.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookies[0].Value + "0"}) //nolint:gosec // test-only request cookie

	if rec = serve(srv, tampered); rec.Code != http.StatusSeeOther {
		t.Fatalf("tampered cookie: got %d, want redirect", rec.Code)
	}

	// Changing the password signs everyone out.
	other := newAuthTestServer(t, WithPassword("a different password"))
	stale := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	stale.AddCookie(cookies[0])

	if rec = serve(other, stale); rec.Code != http.StatusSeeOther {
		t.Fatalf("cookie from old password: got %d, want redirect", rec.Code)
	}
}

func TestCleanEmailListForOlderResults(t *testing.T) {
	got := cleanEmailList("Plumbing-Callout-1-150×150@2x.webp, logo-300×149@2x.png, joe@acme.com, Joe@acme.com")
	if got != "joe@acme.com" {
		t.Fatalf("got %q", got)
	}
}
