// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package tracker

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cockroachdb/cockroach/pkg/raft/quorum"
	pb "github.com/cockroachdb/cockroach/pkg/raft/raftpb"
	"github.com/cockroachdb/cockroach/pkg/raft/raftstoreliveness"
	"github.com/cockroachdb/cockroach/pkg/util/hlc"
)

// FortificationTracker is used to track fortification from peers. This can
// then be used to compute until when a leader's support expires.
type FortificationTracker struct {
	config        *quorum.Config
	storeLiveness raftstoreliveness.StoreLiveness

	// fortification contains a map of nodes which have fortified the leader
	// through fortification handshakes, and the corresponding Store Liveness
	// epochs that they have supported the leader in.
	fortification map[pb.PeerID]pb.Epoch
}

// MakeFortificationTracker initializes a FortificationTracker.
func MakeFortificationTracker(
	config *quorum.Config, storeLiveness raftstoreliveness.StoreLiveness,
) FortificationTracker {
	st := FortificationTracker{
		config:        config,
		storeLiveness: storeLiveness,
		fortification: map[pb.PeerID]pb.Epoch{},
	}
	return st
}

// FortificationEnabled returns whether the raft fortification protocol is
// enabled or not.
func (ft *FortificationTracker) FortificationEnabled() bool {
	return ft.storeLiveness.SupportFromEnabled()
}

// RecordFortification records fortification of the given peer for the supplied
// epoch.
func (ft *FortificationTracker) RecordFortification(id pb.PeerID, epoch pb.Epoch) {
	// The supported epoch should never regress. Guard against out of order
	// delivery of fortify responses by using max.
	ft.fortification[id] = max(ft.fortification[id], epoch)
}

// Reset clears out any previously tracked fortification.
func (ft *FortificationTracker) Reset() {
	clear(ft.fortification)
	// TODO(arul): when we introduce ft.LeadSupportUntil we need to make sure it
	// isn't reset here, because we don't want it to regress when a leader steps
	// down.
}

// IsFortifiedBy returns whether the follower fortifies the leader or not.
// If the follower's store doesn't support the leader's store in the store
// liveness fabric, then both isSupported and isFortified will be false.
// If isFortified is true, it implies that isSupported is also true.
func (ft *FortificationTracker) IsFortifiedBy(id pb.PeerID) (isFortified bool, isSupported bool) {
	supportEpoch, curExp := ft.storeLiveness.SupportFrom(id)
	if ft.storeLiveness.SupportExpired(curExp) {
		return false, false
	}

	// At this point we know that the follower's store is providing support
	// at the store liveness fabric.
	fortificationEpoch, exist := ft.fortification[id]
	if !exist {
		// We don't know that the follower is fortified.
		return false, true
	}

	// NB: We can't assert that supportEpoch <= fortificationEpoch because there
	// may be a race between a successful MsgFortifyLeaderResp and the store
	// liveness heartbeat response that lets the leader know the follower's store
	// is supporting the leader's store at the epoch in the MsgFortifyLeaderResp
	// message.
	return fortificationEpoch == supportEpoch, true
}

// LeadSupportUntil returns the timestamp until which the leader is guaranteed
// fortification until based on the fortification being tracked for it by its
// peers.
func (ft *FortificationTracker) LeadSupportUntil() hlc.Timestamp {
	// TODO(arul): avoid this map allocation as we're calling LeadSupportUntil
	// from hot paths.
	supportExpMap := make(map[pb.PeerID]hlc.Timestamp)
	for id, supportEpoch := range ft.fortification {
		curEpoch, curExp := ft.storeLiveness.SupportFrom(id)
		// NB: We can't assert that supportEpoch <= curEpoch because there may be a
		// race between a successful MsgFortifyLeaderResp and the store liveness
		// heartbeat response that lets the leader know the follower's store is
		// supporting the leader's store at the epoch in the MsgFortifyLeaderResp
		// message.
		if curEpoch == supportEpoch {
			supportExpMap[id] = curExp
		}
	}
	return ft.config.Voters.LeadSupportExpiration(supportExpMap)
}

// QuorumActive returns whether the leader is currently supported by a quorum or
// not.
func (ft *FortificationTracker) QuorumActive() bool {
	return !ft.storeLiveness.SupportExpired(ft.LeadSupportUntil())
}

func (ft *FortificationTracker) Empty() bool {
	return len(ft.fortification) == 0
}

func (ft *FortificationTracker) String() string {
	if ft.Empty() {
		return "empty"
	}
	// Print the map in sorted order as we assert on its output in tests.
	ids := make([]pb.PeerID, 0, len(ft.fortification))
	for id := range ft.fortification {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var buf strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&buf, "%d : %d\n", id, ft.fortification[id])
	}
	return buf.String()
}
