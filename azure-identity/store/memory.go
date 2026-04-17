package store

import (
	"sync"
	"time"
)

// ── Data structs ──────────────────────────────────────────────────────────────

type Session struct {
	InternalID      string
	AzureSessionID  string
	UserID          string
	Status          string // pending | liveness_passed | liveness_failed | completed
	DecisionStatus  string // pending | verified | failed
	CreatedAt       time.Time
}

type LivenessResult struct {
	SessionID         string
	Verdict           string // succeeded | failed
	SessionImageID    string // Azure sessionImageId — used to download the face capture
	SessionImageBytes []byte // downloaded face capture — used for face comparison against doc
}

type DocScanResult struct {
	SessionID      string
	FirstName      string
	LastName       string
	DOB            string
	IDNumber       string
	Expiry         string
	IssuingCountry string
	Address        string
	DocumentType   string
}

type FaceMatchResult struct {
	SessionID  string
	Similarity float64
	Passed     bool
}

type IdentityHash struct {
	UserID    string
	FieldName string // doc_number | first_name_dob | face_id
	HashValue string
	HashAlgo  string
}

type ConsentRecord struct {
	UserID    string
	SessionID string
	FieldName string
	Consented bool
}

type VerifiedData struct {
	UserID         string
	SessionID      string
	FieldName      string
	EncryptedValue string
	EncryptionIV   string
}

// ── Store ─────────────────────────────────────────────────────────────────────

type Store struct {
	mu sync.RWMutex

	Sessions         map[string]*Session        // key: internalID
	LivenessResults  map[string]*LivenessResult  // key: sessionID
	DocScanResults   map[string]*DocScanResult   // key: sessionID
	FaceMatchResults map[string]*FaceMatchResult // key: sessionID

	// key: fieldName+":"+hashValue → for cross-user dedup lookups
	IdentityHashIndex map[string]*IdentityHash

	// key: userID
	IdentityHashes map[string][]IdentityHash
	ConsentRecords map[string][]ConsentRecord
	VerifiedData   map[string][]VerifiedData
}

func New() *Store {
	return &Store{
		Sessions:          make(map[string]*Session),
		LivenessResults:   make(map[string]*LivenessResult),
		DocScanResults:    make(map[string]*DocScanResult),
		FaceMatchResults:  make(map[string]*FaceMatchResult),
		IdentityHashIndex: make(map[string]*IdentityHash),
		IdentityHashes:    make(map[string][]IdentityHash),
		ConsentRecords:    make(map[string][]ConsentRecord),
		VerifiedData:      make(map[string][]VerifiedData),
	}
}

// ── Session helpers ───────────────────────────────────────────────────────────

func (s *Store) SaveSession(sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Sessions[sess.InternalID] = sess
}

func (s *Store) GetSession(internalID string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.Sessions[internalID]
	return sess, ok
}

func (s *Store) UpdateSession(internalID, status, decisionStatus string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.Sessions[internalID]; ok {
		if status != "" {
			sess.Status = status
		}
		if decisionStatus != "" {
			sess.DecisionStatus = decisionStatus
		}
	}
}

// ── Liveness helpers ──────────────────────────────────────────────────────────

func (s *Store) SaveLivenessResult(r *LivenessResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LivenessResults[r.SessionID] = r
}

func (s *Store) GetLivenessResult(sessionID string) (*LivenessResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.LivenessResults[sessionID]
	return r, ok
}

// ── Doc scan helpers ──────────────────────────────────────────────────────────

func (s *Store) SaveDocScan(r *DocScanResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.DocScanResults[r.SessionID] = r
}

func (s *Store) GetDocScan(sessionID string) (*DocScanResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.DocScanResults[sessionID]
	return r, ok
}

// ── Face match helpers ────────────────────────────────────────────────────────

func (s *Store) SaveFaceMatch(r *FaceMatchResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FaceMatchResults[r.SessionID] = r
}

// ── Identity hash helpers ─────────────────────────────────────────────────────

// FindHash returns the existing hash for a given field+value, or nil.
func (s *Store) FindHash(fieldName, hashValue string) *IdentityHash {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := fieldName + ":" + hashValue
	h, ok := s.IdentityHashIndex[key]
	if !ok {
		return nil
	}
	return h
}

// UpsertHash writes a hash, ensuring only one row per userID+fieldName exists.
func (s *Store) UpsertHash(h IdentityHash) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove old entry from index for this user+field if it exists.
	existing := s.IdentityHashes[h.UserID]
	newList := existing[:0]
	for _, old := range existing {
		if old.FieldName == h.FieldName {
			delete(s.IdentityHashIndex, old.FieldName+":"+old.HashValue)
		} else {
			newList = append(newList, old)
		}
	}
	newList = append(newList, h)
	s.IdentityHashes[h.UserID] = newList
	s.IdentityHashIndex[h.FieldName+":"+h.HashValue] = &h
}

// ClearUserData removes all stored data for a user (re-verification).
func (s *Store) ClearUserData(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, h := range s.IdentityHashes[userID] {
		delete(s.IdentityHashIndex, h.FieldName+":"+h.HashValue)
	}
	delete(s.IdentityHashes, userID)
	delete(s.ConsentRecords, userID)
	delete(s.VerifiedData, userID)
}

// ── Consent / verified data helpers ──────────────────────────────────────────

func (s *Store) AppendConsent(c ConsentRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ConsentRecords[c.UserID] = append(s.ConsentRecords[c.UserID], c)
}

func (s *Store) AppendVerifiedData(v VerifiedData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.VerifiedData[v.UserID] = append(s.VerifiedData[v.UserID], v)
}
