package chains

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/api/health"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/logging"
)

func TestLocalStatus(t *testing.T) {
	require := require.New(t)

	m := New(&ManagerConfig{Log: logging.NoLog{}}).(*manager)
	chainID := ids.GenerateTestID()
	params := ChainParameters{
		ID:       chainID,
		SubnetID: ids.GenerateTestID(),
		VMID:     ids.GenerateTestID(),
	}

	_, ok := m.LocalStatus(chainID)
	require.False(ok, "unknown chains have no local status")

	m.QueueChainCreation(params)
	status, ok := m.LocalStatus(chainID)
	require.True(ok)
	require.Equal(params.SubnetID, status.SubnetID)
	require.Equal(params.VMID, status.VMID)
	require.False(status.Created)
	require.NoError(status.CreationErr)

	// Queueing the same chain twice is idempotent.
	m.QueueChainCreation(params)
	require.Equal(1, m.chainsQueue.Len())

	creationErr := errors.New("bad genesis")
	m.chainsLock.Lock()
	m.failed[chainID] = creationErr
	m.chainsLock.Unlock()
	status, ok = m.LocalStatus(chainID)
	require.True(ok)
	require.ErrorIs(status.CreationErr, creationErr)
	require.False(status.Created)
}

// A chain that fails to build must not hold the other chains of its subnet in
// bootstrapping forever (they would wait for it after every node restart).
func TestFailedChainDoesNotBlockSubnetBootstrapping(t *testing.T) {
	require := require.New(t)

	healthChecker, err := health.New(logging.NoLog{}, prometheus.NewRegistry())
	require.NoError(err)
	m := New(&ManagerConfig{
		Log:          logging.NoLog{},
		Health:       healthChecker,
		ChainDataDir: t.TempDir(),
	}).(*manager)

	subnetID := ids.GenerateTestID()
	healthy := ChainParameters{ID: ids.GenerateTestID(), SubnetID: subnetID, VMID: ids.GenerateTestID()}
	broken := ChainParameters{ID: ids.GenerateTestID(), SubnetID: subnetID, VMID: constants.PlatformVMID}
	m.QueueChainCreation(healthy)
	m.QueueChainCreation(broken)
	sb := m.subnets[subnetID]

	m.createChain(broken)
	status, ok := m.LocalStatus(broken.ID)
	require.True(ok)
	require.ErrorIs(status.CreationErr, errCreatePlatformVM)
	require.False(sb.IsBootstrapped(), "the healthy chain is still bootstrapping")

	sb.Bootstrapped(healthy.ID)
	require.True(sb.IsBootstrapped())
}
