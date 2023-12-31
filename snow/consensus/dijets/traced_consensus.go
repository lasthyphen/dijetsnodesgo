// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package dijets

import (
	"context"

	"go.opentelemetry.io/otel/attribute"

	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/trace"
)

var _ Consensus = (*tracedConsensus)(nil)

type tracedConsensus struct {
	Consensus
	tracer trace.Tracer
}

func Trace(consensus Consensus, tracer trace.Tracer) Consensus {
	return &tracedConsensus{
		Consensus: consensus,
		tracer:    tracer,
	}
}

func (c *tracedConsensus) Add(ctx context.Context, vtx Vertex) error {
	ctx, span := c.tracer.Start(ctx, "tracedConsensus.Add", oteltrace.WithAttributes(
		attribute.Stringer("vtxID", vtx.ID()),
	))
	defer span.End()

	return c.Consensus.Add(ctx, vtx)
}

func (c *tracedConsensus) RecordPoll(ctx context.Context, votes ids.UniqueBag) error {
	var allVotes ids.BitSet64
	for _, vote := range votes {
		allVotes.Union(vote)
	}

	ctx, span := c.tracer.Start(ctx, "tracedConsensus.RecordPoll", oteltrace.WithAttributes(
		attribute.Int("numVotes", allVotes.Len()),
		attribute.Int("numVtxIDs", len(votes)),
	))
	defer span.End()

	return c.Consensus.RecordPoll(ctx, votes)
}
