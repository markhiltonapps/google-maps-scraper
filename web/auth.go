package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookieName = "gms_session"
	sessionTTL        = 30 * 24 * time.Hour
	// loginFailureDelay slows down password guessing.
	loginFailureDelay = time.Second
)

// ServerOption configures the web server.
type ServerOption func(*Server)

// WithPassword protects the app with one shared team password. Results share
// pages (/share/...), their downloads and static files stay public.
func WithPassword(password string) ServerOption {
	return func(s *Server) {
		s.password = password
	}
}

func (s *Server) authEnabled() bool {
	return s.password != ""
}

// isPublicPath reports whether a path is reachable without signing in.
func isPublicPath(path string) bool {
	switch {
	case path == "/login", path == "/logout", path == "/download":
		// /download is linked from public share pages; job IDs are random UUIDs.
		return true
	case strings.HasPrefix(path, "/static/"), strings.HasPrefix(path, "/share/"):
		return true
	default:
		return false
	}
}

// requireAuth redirects visitors without a valid session to the login page.
// API clients may instead send the password with HTTP basic auth.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authEnabled() || isPublicPath(r.URL.Path) || s.validSession(r) {
			next.ServeHTTP(w, r)

			return
		}

		switch {
		case strings.HasPrefix(r.URL.Path, "/api/"):
			if _, pass, ok := r.BasicAuth(); ok && s.passwordMatches(pass) {
				next.ServeHTTP(w, r)

				return
			}

			w.Header().Set("WWW-Authenticate", `Basic realm="Google Maps Scraper"`)
			renderJSON(w, http.StatusUnauthorized, apiError{Code: http.StatusUnauthorized, Message: "unauthorized"})
		case r.Header.Get("HX-Request") == "true":
			// htmx follows HX-Redirect with a full page load.
			w.Header().Set("HX-Redirect", "/login")
			w.WriteHeader(http.StatusUnauthorized)
		default:
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
	})
}

func (s *Server) passwordMatches(candidate string) bool {
	want := sha256.Sum256([]byte(s.password))
	got := sha256.Sum256([]byte(candidate))

	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}

// sessionKey is derived from the password, so changing it signs everyone out.
func (s *Server) sessionKey() []byte {
	key := sha256.Sum256([]byte("gms-session:" + s.password))

	return key[:]
}

func (s *Server) sessionToken(expires time.Time) string {
	exp := strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, s.sessionKey())
	mac.Write([]byte(exp))

	return exp + "." + hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) validSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}

	exp, sig, ok := strings.Cut(cookie.Value, ".")
	if !ok {
		return false
	}

	unix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() > unix {
		return false
	}

	want := s.sessionToken(time.Unix(unix, 0))
	_, wantSig, _ := strings.Cut(want, ".")

	return hmac.Equal([]byte(sig), []byte(wantSig))
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

type loginData struct {
	Error string
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.authEnabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)

		return
	}

	switch r.Method {
	case http.MethodGet:
		s.renderLogin(w, http.StatusOK, loginData{})
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			s.renderLogin(w, http.StatusBadRequest, loginData{Error: "Something went wrong. Please try again."})

			return
		}

		if !s.passwordMatches(r.PostForm.Get("password")) {
			time.Sleep(s.loginDelay)
			s.renderLogin(w, http.StatusUnauthorized, loginData{Error: "That password isn't right."})

			return
		}

		expires := time.Now().Add(sessionTTL)
		//nolint:gosec // Secure is set whenever the request came over HTTPS (always behind Railway's proxy); forcing it would break sign-in on plain-HTTP local runs.
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    s.sessionToken(expires),
			Path:     "/",
			Expires:  expires,
			MaxAge:   int(sessionTTL.Seconds()),
			HttpOnly: true,
			Secure:   isHTTPS(r),
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	default:
		http.Error(w, methodNotAllowedMessage, http.StatusMethodNotAllowed)
	}
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	//nolint:gosec // see login: Secure follows the request scheme.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) renderLogin(w http.ResponseWriter, status int, data loginData) {
	tmpl, ok := s.tmpl["static/templates/login.html"]
	if !ok {
		http.Error(w, "missing tpl", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = tmpl.Execute(w, data)
}
