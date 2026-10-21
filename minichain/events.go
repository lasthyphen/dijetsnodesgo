package minichain

import (
	"fmt"
	"time"

	"github.com/lasthyphen/dijetsnodesgo/ids"
)

// EventType identifies the kind of a mini-chain event.
type EventType string

const (
	// EventRegisteredObserved: this node observed a registered mini-chain it
	// tracks for the first time since it started.
	EventRegisteredObserved         EventType = "registered_observed"
	EventInitialisationStarted      EventType = "initialisation_started"
	EventActivated                  EventType = "activated"
	EventHealthChanged              EventType = "health_changed"
	EventStopped                    EventType = "stopped"
	EventFailed                     EventType = "failed"
	EventLifecycleChanged           EventType = "lifecycle_changed"
	EventValidatorMembershipChanged EventType = "validator_membership_changed"
)

// SourceLocalObservation marks data observed by the answering node, as
// opposed to authoritative M-chain state.
const SourceLocalObservation = "local-observation"

// Event is a locally observed mini-chain event. Events are derived from
// observations; they are not authoritative state transitions. Authoritative
// registration and membership changes are M-chain transactions and can be
// consumed through the M-chain index (/ext/index/M/block).
type Event struct {
	SchemaVersion uint32 `json:"schemaVersion"`
	// ID is globally unique: "<bootID>-<seq>".
	ID string `json:"id"`
	// Seq is strictly increasing within a BootID.
	Seq           uint64     `json:"seq"`
	BootID        string     `json:"bootID"`
	Type          EventType  `json:"type"`
	ChainID       ids.ID     `json:"chainID"`
	SubnetID      ids.ID     `json:"subnetID"`
	From          Lifecycle  `json:"from"`
	To            Lifecycle  `json:"to"`
	Reasons       []string   `json:"reasons"`
	ObservedAt    time.Time  `json:"observedAt"`
	ObserverNode  ids.NodeID `json:"observerNode"`
	Source        string     `json:"source"`
	Authoritative bool       `json:"authoritative"`
}

// EventPage is a page of events.
type EventPage struct {
	BootID string  `json:"bootID"`
	Events []Event `json:"events"`
	// NextCursor is the sequence number to pass as "after" next time.
	NextCursor uint64 `json:"nextCursor"`
	// OldestSeq is the oldest sequence number still retained (0 if empty).
	OldestSeq uint64 `json:"oldestSeq"`
	// Truncated is true if events after the requested cursor were evicted;
	// the consumer must resynchronise from minichain.list.
	Truncated bool `json:"truncated"`
}

func eventTypeFor(from, to Lifecycle) EventType {
	switch {
	case to == Initialising:
		return EventInitialisationStarted
	case to == Active && from == Degraded:
		return EventHealthChanged
	case to == Active:
		return EventActivated
	case to == Degraded && from == Active:
		return EventHealthChanged
	case to == Stopped:
		return EventStopped
	case to == Failed:
		return EventFailed
	default:
		return EventLifecycleChanged
	}
}

type lastState struct {
	lifecycle Lifecycle
	committee string
}

// eventLog is a bounded ring buffer of events. Not thread safe.
type eventLog struct {
	capacity int
	buf      []Event
	start    int
	nextSeq  uint64
	state    map[ids.ID]lastState
}

func newEventLog(capacity int) *eventLog {
	return &eventLog{
		capacity: capacity,
		nextSeq:  1,
		state:    make(map[ids.ID]lastState),
	}
}

func (l *eventLog) last(chainID ids.ID) (Lifecycle, string, bool) {
	s, ok := l.state[chainID]
	return s.lifecycle, s.committee, ok
}

func (l *eventLog) remember(chainID ids.ID, lifecycle Lifecycle, committee string) {
	l.state[chainID] = lastState{lifecycle: lifecycle, committee: committee}
}

func (l *eventLog) append(e Event) {
	e.Seq = l.nextSeq
	e.ID = fmt.Sprintf("%s-%d", e.BootID, e.Seq)
	l.nextSeq++
	if len(l.buf) < l.capacity {
		l.buf = append(l.buf, e)
		return
	}
	l.buf[l.start] = e
	l.start = (l.start + 1) % l.capacity
}

func (l *eventLog) at(i int) Event {
	return l.buf[(l.start+i)%len(l.buf)]
}

func (l *eventLog) page(bootID string, after uint64, limit int) EventPage {
	p := EventPage{BootID: bootID, Events: []Event{}, NextCursor: after}
	if len(l.buf) == 0 {
		return p
	}
	oldest := l.at(0).Seq
	p.OldestSeq = oldest
	p.Truncated = after+1 < oldest
	for i := 0; i < len(l.buf) && len(p.Events) < limit; i++ {
		e := l.at(i)
		if e.Seq <= after {
			continue
		}
		p.Events = append(p.Events, e)
		p.NextCursor = e.Seq
	}
	return p
}
