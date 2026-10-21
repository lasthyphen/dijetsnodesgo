package minichain

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/ids"
)

func TestEventLogPaging(t *testing.T) {
	require := require.New(t)
	l := newEventLog(3)
	chainID := ids.GenerateTestID()

	page := l.page("boot", 0, 10)
	require.Empty(page.Events)
	require.False(page.Truncated)

	for i := 0; i < 5; i++ {
		l.append(Event{BootID: "boot", ChainID: chainID})
	}
	// Only the last 3 events (seq 3, 4, 5) are retained.
	page = l.page("boot", 0, 10)
	require.True(page.Truncated)
	require.Equal(uint64(3), page.OldestSeq)
	require.Len(page.Events, 3)
	require.Equal(uint64(3), page.Events[0].Seq)
	require.Equal("boot-3", page.Events[0].ID)
	require.Equal(uint64(5), page.NextCursor)

	page = l.page("boot", 2, 10)
	require.False(page.Truncated)
	require.Len(page.Events, 3)

	page = l.page("boot", 3, 1)
	require.Len(page.Events, 1)
	require.Equal(uint64(4), page.Events[0].Seq)
	require.Equal(uint64(4), page.NextCursor)

	page = l.page("boot", 5, 10)
	require.Empty(page.Events)
	require.Equal(uint64(5), page.NextCursor)
}

func TestEventTypeFor(t *testing.T) {
	require := require.New(t)
	require.Equal(EventInitialisationStarted, eventTypeFor(Pending, Initialising))
	require.Equal(EventActivated, eventTypeFor(Initialising, Active))
	require.Equal(EventHealthChanged, eventTypeFor(Active, Degraded))
	require.Equal(EventHealthChanged, eventTypeFor(Degraded, Active))
	require.Equal(EventFailed, eventTypeFor(Initialising, Failed))
	require.Equal(EventStopped, eventTypeFor(Stopping, Stopped))
	require.Equal(EventLifecycleChanged, eventTypeFor(Active, Stopping))
}
