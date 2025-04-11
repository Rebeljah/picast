package rtsp

import (
	"crypto/rand"
	"math/big"
	"sync"
	"time"

	"github.com/rebeljah/picast/media"
)

type SessionUID string

func newSessionUID(length int) SessionUID {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length) // Keep len >= 8 to comply with RFC2326
	for i := range b {
		randIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[randIndex.Int64()]
	}
	return SessionUID(b)
}

type Session struct {
	sync.RWMutex // many readers OR one writer
	UID          SessionUID
	CreatedAt    time.Time
	ContentID    media.UID
	TrackStreams map[media.TrackID]*TrackStreamState
}

func NewSession() *Session {
	return &Session{
		UID:          newSessionUID(16),
		CreatedAt:    time.Now().UTC(),
		TrackStreams: make(map[media.TrackID]*TrackStreamState),
	}
}

func (s *Session) State() TrackState {
	s.RLock()
	defer s.RUnlock()

	trackStates := make([]TrackState, 0)
	for _, streamInfo := range s.TrackStreams {
		trackStates = append(trackStates, streamInfo.StateNow)
	}
	return StateFromTrackStreamStates(trackStates)
}

func (s *Session) ActiveStreams() []*TrackStreamState {
	s.RLock()
	defer s.RUnlock()

	states := make([]*TrackStreamState, 0, len(s.TrackStreams))

	for _, st := range s.TrackStreams {
		if st.StateNow != Init && st.StateNow != ErrorState {
			states = append(states, st)
		}
	}

	return states
}

type sessionManager struct {
	sync.RWMutex
	sessions map[SessionUID]*Session
}

func newSessionManager() sessionManager {
	return sessionManager{
		sessions: make(map[SessionUID]*Session),
	}
}

func (s *sessionManager) add(session *Session) {
	s.Lock()
	defer s.Unlock()

	s.sessions[session.UID] = session
}

func (s *sessionManager) get(uid SessionUID) (*Session, bool) {
	s.RLock()
	defer s.RUnlock()

	session, ok := s.sessions[uid]
	return session, ok
}

func (s *sessionManager) delete(uid SessionUID) bool {
	s.Lock()
	defer s.Unlock()

	_, ok := s.sessions[uid]
	delete(s.sessions, uid)
	return ok
}
