package server

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"strconv"
	"sync"
	"time"
)

// sessionTTL is how long a dashboard session stays valid; sessions never outlive the process.
const sessionTTL = 12 * time.Hour

// maxSessions bounds the store so a loop of token visits cannot grow it without limit.
const maxSessions = 256

// sessionStore holds the random ids the dashboard cookie carries. The cookie never holds the
// access token, so another service on localhost that reads it learns nothing reusable
// elsewhere and an id can be revoked by expiry.
type sessionStore struct {
	mu  sync.Mutex
	ids map[string]time.Time
	now func() time.Time
}

func newSessionStore() *sessionStore {
	return &sessionStore{ids: map[string]time.Time{}, now: time.Now}
}

// create returns a new random session id.
func (st *sessionStore) create() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)
	st.mu.Lock()
	defer st.mu.Unlock()
	now := st.now()
	for k, exp := range st.ids {
		if !now.Before(exp) {
			delete(st.ids, k)
		}
	}
	if len(st.ids) >= maxSessions {
		var oldest string
		for k, exp := range st.ids {
			if oldest == "" || exp.Before(st.ids[oldest]) {
				oldest = k
			}
		}
		delete(st.ids, oldest)
	}
	st.ids[id] = now.Add(sessionTTL)
	return id, nil
}

// valid reports whether id is a live session.
func (st *sessionStore) valid(id string) bool {
	if id == "" {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	exp, ok := st.ids[id]
	if ok && !st.now().Before(exp) {
		delete(st.ids, id)
		return false
	}
	return ok
}

// cookieName is per port: browsers share cookies between ports of one host, so a fixed name
// would let two control servers on one machine overwrite each other's session.
func (s *Server) cookieName() string {
	port := s.cfg.Port
	if s.listener != nil {
		if _, p, err := net.SplitHostPort(s.listener.Addr().String()); err == nil {
			if n, err := strconv.Atoi(p); err == nil {
				port = n
			}
		}
	}
	return sessionCookie + "_" + strconv.Itoa(port)
}
