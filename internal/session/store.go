// Package session holds the user-plane session state.
//
// Directionality is explicit: local and remote SEIDs are never conflated,
// and uplink and downlink TEIDs are stored under separate names. The store
// is safe for concurrent use by the PFCP and GTP-U goroutines.
package session

import (
	"fmt"
	"net"
	"sync"
)

// Session describes one user-plane session.
type Session struct {
	// LocalSEID is the SEID this UPF allocates and advertises to the peer.
	LocalSEID uint32
	// RemoteSEID is the SEID the peer allocated. It is not interchangeable
	// with LocalSEID.
	RemoteSEID uint32
	// UplinkTEID identifies the uplink tunnel (RAN -> UPF).
	UplinkTEID uint32
	// DownlinkTEID identifies the downlink tunnel (UPF -> RAN). It is not
	// interchangeable with UplinkTEID.
	DownlinkTEID uint32
	// UEIP is the UE address assigned by the peer.
	UEIP net.IP
	// Peer is the PFCP endpoint owning the session.
	Peer *net.UDPAddr
}

// String renders the session for logs. It keeps the directions visible.
func (s *Session) String() string {
	return fmt.Sprintf("session{localSEID=%d remoteSEID=%d uplinkTEID=%d downlinkTEID=%d ueIP=%s}",
		s.LocalSEID, s.RemoteSEID, s.UplinkTEID, s.DownlinkTEID, s.UEIP)
}

// Store is a concurrency-safe session table.
//
// A Store must be created with NewStore and must not be copied.
type Store struct {
	mu sync.RWMutex

	byLocalSEID  map[uint32]*Session
	byRemoteSEID map[uint32]*Session
	byUplinkTEID map[uint32]*Session
	byUEIP       map[string]*Session
}

// NewStore returns an empty session store.
func NewStore() *Store {
	return &Store{
		byLocalSEID:  make(map[uint32]*Session),
		byRemoteSEID: make(map[uint32]*Session),
		byUplinkTEID: make(map[uint32]*Session),
		byUEIP:       make(map[string]*Session),
	}
}

// Put inserts or replaces a session.
//
// A session with a zero LocalSEID is rejected: the local SEID is the primary
// key and PFCP messages without one cannot be correlated.
func (s *Store) Put(sess *Session) error {
	if sess == nil {
		return fmt.Errorf("session: cannot store a nil session")
	}
	if sess.LocalSEID == 0 {
		return fmt.Errorf("session: LocalSEID must not be zero")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Drop the indexes of the session being replaced, otherwise the old
	// TEID or UE IP would keep pointing at this record.
	if previous, ok := s.byLocalSEID[sess.LocalSEID]; ok {
		s.removeIndexesLocked(previous)
	}

	s.byLocalSEID[sess.LocalSEID] = sess
	if sess.RemoteSEID != 0 {
		s.byRemoteSEID[sess.RemoteSEID] = sess
	}
	if sess.UplinkTEID != 0 {
		s.byUplinkTEID[sess.UplinkTEID] = sess
	}
	if sess.UEIP != nil {
		s.byUEIP[sess.UEIP.String()] = sess
	}
	return nil
}

// GetByLocalSEID looks a session up by the SEID this UPF allocated.
func (s *Store) GetByLocalSEID(seid uint32) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.byLocalSEID[seid]
	return sess, ok
}

// GetByRemoteSEID looks a session up by the SEID the peer allocated.
func (s *Store) GetByRemoteSEID(seid uint32) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.byRemoteSEID[seid]
	return sess, ok
}

// GetByUplinkTEID looks a session up by its uplink tunnel identifier.
func (s *Store) GetByUplinkTEID(teid uint32) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.byUplinkTEID[teid]
	return sess, ok
}

// GetByUEIP looks a session up by the UE address.
func (s *Store) GetByUEIP(ip net.IP) (*Session, bool) {
	if ip == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.byUEIP[ip.String()]
	return sess, ok
}

// Delete removes the session with the given local SEID and reports whether
// one was present.
func (s *Store) Delete(localSEID uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.byLocalSEID[localSEID]
	if !ok {
		return false
	}
	s.removeIndexesLocked(sess)
	delete(s.byLocalSEID, localSEID)
	return true
}

// Len returns the number of sessions in the store.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byLocalSEID)
}

// removeIndexesLocked drops every index entry pointing at sess.
// The caller must hold the write lock.
func (s *Store) removeIndexesLocked(sess *Session) {
	delete(s.byLocalSEID, sess.LocalSEID)
	if sess.RemoteSEID != 0 {
		delete(s.byRemoteSEID, sess.RemoteSEID)
	}
	if sess.UplinkTEID != 0 {
		delete(s.byUplinkTEID, sess.UplinkTEID)
	}
	if sess.UEIP != nil {
		delete(s.byUEIP, sess.UEIP.String())
	}
}
