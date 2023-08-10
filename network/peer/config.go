// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package peer

import (
	"time"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/message"
	"github.com/lasthyphen/dijetsnodesgo/network/throttling"
	"github.com/lasthyphen/dijetsnodesgo/snow/networking/router"
	"github.com/lasthyphen/dijetsnodesgo/snow/networking/tracker"
	"github.com/lasthyphen/dijetsnodesgo/snow/uptime"
	"github.com/lasthyphen/dijetsnodesgo/snow/validators"
	"github.com/lasthyphen/dijetsnodesgo/utils/logging"
	"github.com/lasthyphen/dijetsnodesgo/utils/set"
	"github.com/lasthyphen/dijetsnodesgo/utils/timer/mockable"
	"github.com/lasthyphen/dijetsnodesgo/version"
)

type Config struct {
	// Size, in bytes, of the buffer this peer reads messages into
	ReadBufferSize int
	// Size, in bytes, of the buffer this peer writes messages into
	WriteBufferSize int
	Clock           mockable.Clock
	Metrics         *Metrics
	MessageCreator  message.Creator

	Log                  logging.Logger
	InboundMsgThrottler  throttling.InboundMsgThrottler
	Network              Network
	Router               router.InboundHandler
	VersionCompatibility version.Compatibility
	MySubnets            set.Set[ids.ID]
	Beacons              validators.Set
	NetworkID            uint32
	PingFrequency        time.Duration
	PongTimeout          time.Duration
	MaxClockDifference   time.Duration

	// Unix time of the last message sent and received respectively
	// Must only be accessed atomically
	LastSent, LastReceived int64

	// Tracks CPU/disk usage caused by each peer.
	ResourceTracker tracker.ResourceTracker

	// Tracks which peer knows about which peers
	GossipTracker GossipTracker

	// Calculates uptime of peers
	UptimeCalculator uptime.Calculator

	// Signs my IP so I can send my signed IP address in the Version message
	IPSigner *IPSigner
}