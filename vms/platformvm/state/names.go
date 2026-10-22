package state

import (
	"encoding/binary"
	"fmt"

	"github.com/lasthyphen/dijetsnodesgo/database"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/names"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
)

var (
	// name --> current record
	nameRecordPrefix = []byte("nameRecord")
	// name | 0x00 | generation | version --> record
	nameHistoryPrefix = []byte("nameHistory")
	// target --> name
	nameTargetPrefix = []byte("nameTarget")
)

// nameChanges holds registry modifications that have not been written to
// the parent state yet. It is used by both the base state and diffs.
type nameChanges struct {
	// name --> latest record
	records map[string]*names.Record
	// every record version written, in order
	versions []*names.Record
	// target --> name. An empty name marks a deleted entry.
	targets map[ids.ShortID]string
}

func (c *nameChanges) putName(record *names.Record) {
	if c.records == nil {
		c.records = make(map[string]*names.Record)
	}
	c.records[record.Name] = record
	c.versions = append(c.versions, record)
}

func (c *nameChanges) putTarget(target ids.ShortID, name string) {
	if c.targets == nil {
		c.targets = make(map[ids.ShortID]string)
	}
	c.targets[target] = name
}

func (c *nameChanges) getName(name string) (*names.Record, bool) {
	record, ok := c.records[name]
	return record, ok
}

func (c *nameChanges) getTarget(target ids.ShortID) (string, bool, error) {
	name, ok := c.targets[target]
	if ok && name == "" {
		return "", true, database.ErrNotFound
	}
	return name, ok, nil
}

func (c *nameChanges) apply(baseState Chain) {
	for _, record := range c.versions {
		baseState.PutName(record)
	}
	for target, name := range c.targets {
		if name == "" {
			baseState.DeleteNameTarget(target)
		} else {
			baseState.PutNameTarget(target, name)
		}
	}
}

func nameHistoryKey(name string, generation, version uint64) []byte {
	key := make([]byte, len(name)+1+2*8)
	copy(key, name)
	binary.BigEndian.PutUint64(key[len(name)+1:], generation)
	binary.BigEndian.PutUint64(key[len(name)+9:], version)
	return key
}

func parseNameRecord(bytes []byte) (*names.Record, error) {
	record := &names.Record{}
	if _, err := txs.Codec.Unmarshal(bytes, record); err != nil {
		return nil, fmt.Errorf("failed to parse name record: %w", err)
	}
	return record, nil
}

func (s *state) GetName(name string) (*names.Record, error) {
	if record, ok := s.nameChanges.getName(name); ok {
		return record, nil
	}
	bytes, err := s.nameRecordDB.Get([]byte(name))
	if err != nil {
		return nil, err
	}
	return parseNameRecord(bytes)
}

func (s *state) PutName(record *names.Record) {
	s.nameChanges.putName(record)
}

func (s *state) GetNameTarget(target ids.ShortID) (string, error) {
	if name, ok, err := s.nameChanges.getTarget(target); ok {
		return name, err
	}
	bytes, err := s.nameTargetDB.Get(target[:])
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (s *state) PutNameTarget(target ids.ShortID, name string) {
	s.nameChanges.putTarget(target, name)
}

func (s *state) DeleteNameTarget(target ids.ShortID) {
	s.nameChanges.putTarget(target, "")
}

// GetNameHistory returns every stored version of [name], oldest first. Only
// written versions are returned.
func (s *state) GetNameHistory(name string) ([]*names.Record, error) {
	prefix := make([]byte, len(name)+1)
	copy(prefix, name)
	it := s.nameHistoryDB.NewIteratorWithPrefix(prefix)
	defer it.Release()

	var history []*names.Record
	for it.Next() {
		record, err := parseNameRecord(it.Value())
		if err != nil {
			return nil, err
		}
		history = append(history, record)
	}
	return history, it.Error()
}

func (s *state) writeNames() error {
	for _, record := range s.nameChanges.versions {
		bytes, err := txs.Codec.Marshal(txs.Version, record)
		if err != nil {
			return fmt.Errorf("failed to serialize name record: %w", err)
		}
		key := nameHistoryKey(record.Name, record.Generation, record.Version)
		if err := s.nameHistoryDB.Put(key, bytes); err != nil {
			return fmt.Errorf("failed to write name history: %w", err)
		}
	}
	for name, record := range s.nameChanges.records {
		bytes, err := txs.Codec.Marshal(txs.Version, record)
		if err != nil {
			return fmt.Errorf("failed to serialize name record: %w", err)
		}
		if err := s.nameRecordDB.Put([]byte(name), bytes); err != nil {
			return fmt.Errorf("failed to write name record: %w", err)
		}
	}
	for target, name := range s.nameChanges.targets {
		var err error
		if name == "" {
			err = s.nameTargetDB.Delete(target[:])
		} else {
			err = s.nameTargetDB.Put(target[:], []byte(name))
		}
		if err != nil {
			return fmt.Errorf("failed to write name target: %w", err)
		}
	}
	s.nameChanges = nameChanges{}
	return nil
}
