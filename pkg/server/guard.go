package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// sessionCookie is the prefix of the dashboard's session cookie, which carries a random session
// id (never the access token) after the first authorised visit.
const sessionCookie = "dendrite_session"

// GenerateToken returns a random 256-bit token for the control server.
func GenerateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// hostName strips the port and brackets from a Host header value.
func hostName(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return strings.ToLower(h)
	}
	return strings.ToLower(strings.Trim(hostport, "[]"))
}

func isLoopbackName(name string) bool {
	if name == "localhost" {
		return true
	}
	ip := net.ParseIP(name)
	return ip != nil && ip.IsLoopback()
}

// hostAllowed reports whether the Host header names this server: a loopback name,
// or the host the server was configured to bind. Anything else is a DNS-rebinding attempt.
func (s *Server) hostAllowed(r *http.Request) bool {
	name := hostName(r.Host)
	return isLoopbackName(name) || name == strings.ToLower(s.cfg.Host)
}

// originAllowed accepts requests with no Origin (non-browser clients) and requests
// whose Origin is the server's own origin.
func originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func tokenEqual(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// authorised checks the Bearer header, the token query parameter and the session cookie.
func (s *Server) authorised(r *http.Request) (ok bool, viaQuery bool) {
	want := s.cfg.AuthToken
	if tokenEqual(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), want) {
		return true, false
	}
	if tokenEqual(r.URL.Query().Get("token"), want) {
		return true, true
	}
	if c, err := r.Cookie(s.cookieName()); err == nil && s.sessions.valid(c.Value) {
		return true, false
	}
	return false, false
}

// jsonBodyOK requires application/json on requests that send a body.
func jsonBodyOK(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return true
	}
	if r.ContentLength == 0 && r.Body == http.NoBody {
		return true
	}
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mt == "application/json"
}

// screen rejects requests that name a foreign host or origin, and answers preflights.
// It reports whether the request was handled.
func (s *Server) screen(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case !s.hostAllowed(r):
		http.Error(w, "forbidden host", http.StatusForbidden)
	case !originAllowed(r):
		http.Error(w, "forbidden origin", http.StatusForbidden)
	case r.Method == http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
	default:
		return false
	}
	return true
}

// authenticate enforces the token, and turns a dashboard visit's token into a session cookie.
// It reports whether the request may continue.
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.AuthToken == "" {
		return true
	}
	ok, viaQuery := s.authorised(r)
	if !ok {
		http.Error(w, ErrUnauthorizedAccess.Error(), http.StatusUnauthorized)
		return false
	}
	if viaQuery && r.URL.Path == "/" {
		id, err := s.sessions.create()
		if err != nil {
			http.Error(w, "cannot start a session", http.StatusInternalServerError)
			return false
		}
		http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: id, Path: "/", MaxAge: int(sessionTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteStrictMode})
	}
	return true
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.screen(w, r) || !s.authenticate(w, r) {
			return
		}
		if !jsonBodyOK(r) {
			http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		next.ServeHTTP(w, r)
	})
}
