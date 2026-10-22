package txservice

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/set"
)

// store persists payments and delegation revocations. Every change is
// written to disk, through a temporary file and a rename, before it is
// acknowledged, so idempotency, reservations and daily limits survive a
// restart.
type store struct {
	lock sync.Mutex
	path string
	data storeData
}

type storeData struct {
	Payments map[string]*Payment `json:"payments"`
	// client/delegation/idempotency key --> payment ID
	Idempotency map[string]string    `json:"idempotency"`
	Revoked     map[string]time.Time `json:"revoked"`
}

func openStore(path string) (*store, error) {
	s := &store{
		path: path,
		data: storeData{
			Payments:    make(map[string]*Payment),
			Idempotency: make(map[string]string),
			Revoked:     make(map[string]time.Time),
		},
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	return s, json.Unmarshal(b, &s.data)
}

func (s *store) flush() error {
	b, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".store-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

func idempotencyKey(client, delegation, key string) string {
	return client + "\x00" + delegation + "\x00" + key
}

// get returns a copy of a payment.
func (s *store) get(id string) (*Payment, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	p, ok := s.data.Payments[id]
	if !ok {
		return nil, false
	}
	clone := *p
	return &clone, true
}

func (s *store) byIdempotencyKey(client, delegation, key string) (*Payment, bool) {
	s.lock.Lock()
	id, ok := s.data.Idempotency[idempotencyKey(client, delegation, key)]
	s.lock.Unlock()
	if !ok {
		return nil, false
	}
	return s.get(id)
}

// put stores a copy of [p].
func (s *store) put(p *Payment) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	clone := *p
	s.data.Payments[p.ID] = &clone
	if p.IdempotencyKey != "" {
		s.data.Idempotency[idempotencyKey(p.PreparedBy, p.Delegation, p.IdempotencyKey)] = p.ID
	}
	return s.flush()
}

// list returns copies of a delegation's payments, newest first.
func (s *store) list(delegation string) []*Payment {
	s.lock.Lock()
	defer s.lock.Unlock()
	var payments []*Payment
	for _, p := range s.data.Payments {
		if p.Delegation == delegation {
			clone := *p
			payments = append(payments, &clone)
		}
	}
	sort.Slice(payments, func(i, j int) bool {
		return payments[i].CreatedAt.After(payments[j].CreatedAt)
	})
	return payments
}

// reserved returns the inputs consumed by a delegation's submitted
// payments whose transactions are not decided yet.
func (s *store) reserved(delegation string) set.Set[ids.ID] {
	s.lock.Lock()
	defer s.lock.Unlock()
	reserved := set.NewSet[ids.ID](0)
	for _, p := range s.data.Payments {
		if p.Delegation == delegation && p.Status == StatusSubmitted {
			reserved.Add(p.Inputs...)
		}
	}
	return reserved
}

// spentSince sums a delegation's submitted and accepted payments, including
// fees, since [since].
func (s *store) spentSince(delegation string, since time.Time) uint64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	var spent uint64
	for _, p := range s.data.Payments {
		if p.Delegation != delegation || p.SubmittedAt == nil || p.SubmittedAt.Before(since) {
			continue
		}
		if p.Status == StatusSubmitted || p.Status == StatusAccepted {
			spent += uint64(p.Amount.Units) + uint64(p.Fee.Units)
		}
	}
	return spent
}

func (s *store) revoke(delegation string, at time.Time) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	if _, ok := s.data.Revoked[delegation]; ok {
		return nil
	}
	s.data.Revoked[delegation] = at
	return s.flush()
}

func (s *store) isRevoked(delegation string) bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	_, ok := s.data.Revoked[delegation]
	return ok
}
