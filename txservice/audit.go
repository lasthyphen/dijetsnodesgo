package txservice

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

var ErrAuditChainBroken = errors.New("audit log hash chain is broken")

// AuditEntry is one record of the audit log. Each entry commits to its
// predecessor through PrevHash, so removing, reordering or editing entries
// is detectable.
type AuditEntry struct {
	Seq        uint64            `json:"seq"`
	Time       time.Time         `json:"time"`
	Event      string            `json:"event"`
	Client     string            `json:"client,omitempty"`
	Delegation string            `json:"delegation,omitempty"`
	PaymentID  string            `json:"paymentID,omitempty"`
	Fields     map[string]string `json:"fields,omitempty"`
	PrevHash   string            `json:"prevHash"`
	Hash       string            `json:"hash"`
}

func (e *AuditEntry) computeHash() (string, error) {
	unhashed := *e
	unhashed.Hash = ""
	b, err := json.Marshal(&unhashed)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// AuditLog is an append-only, hash-chained JSON-lines file.
type AuditLog struct {
	lock     sync.Mutex
	file     *os.File
	seq      uint64
	lastHash string
	now      func() time.Time
}

// OpenAuditLog opens the log at [path], verifying the existing chain.
func OpenAuditLog(path string, now func() time.Time) (*AuditLog, error) {
	entries, err := ReadAuditLog(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	log := &AuditLog{now: now}
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		log.seq = last.Seq
		log.lastHash = last.Hash
	}
	log.file, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return log, nil
}

// ReadAuditLog reads and verifies every entry of the log at [path].
func ReadAuditLog(path string) ([]*AuditEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var (
		entries  []*AuditEntry
		prevHash string
		scanner  = bufio.NewScanner(f)
	)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		entry := &AuditEntry{}
		if err := json.Unmarshal(scanner.Bytes(), entry); err != nil {
			return nil, fmt.Errorf("%w: entry %d is malformed: %s", ErrAuditChainBroken, len(entries)+1, err)
		}
		hash, err := entry.computeHash()
		if err != nil {
			return nil, err
		}
		if entry.Seq != uint64(len(entries)+1) || entry.PrevHash != prevHash || entry.Hash != hash {
			return nil, fmt.Errorf("%w at entry %d", ErrAuditChainBroken, len(entries)+1)
		}
		prevHash = entry.Hash
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

// Append writes an entry and syncs it to disk before returning.
func (l *AuditLog) Append(entry AuditEntry) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	entry.Seq = l.seq + 1
	entry.Time = l.now().UTC()
	entry.PrevHash = l.lastHash
	hash, err := entry.computeHash()
	if err != nil {
		return err
	}
	entry.Hash = hash
	b, err := json.Marshal(&entry)
	if err != nil {
		return err
	}
	if _, err := l.file.Write(append(b, '\n')); err != nil {
		return err
	}
	if err := l.file.Sync(); err != nil {
		return err
	}
	l.seq = entry.Seq
	l.lastHash = entry.Hash
	return nil
}

// Head returns the sequence number and hash of the latest entry.
func (l *AuditLog) Head() (uint64, string) {
	l.lock.Lock()
	defer l.lock.Unlock()
	return l.seq, l.lastHash
}

func (l *AuditLog) Close() error {
	return l.file.Close()
}
