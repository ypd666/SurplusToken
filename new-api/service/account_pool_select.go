// Copyright (C) 2026 SurplusToken Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
// This file is part of SurplusToken, a derivative work of New API
// (Copyright (C) 2023-2026 QuantumNous, AGPLv3).

package service

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// PoolGroup is the channel group that holds the contributed-account channels.
const PoolGroup = "pool"

// poolGroups is the set of USER groups whose requests are routed through the
// contributed-account reservation pool (owner routing + share caps + reward).
// Tiers like pro/prolite/plus consume the pool at their own group ratio; the
// contributor group lets owners use their own accounts. Override via the
// POOL_GROUPS env var (comma-separated).
var poolGroups = func() map[string]bool {
	raw := common.GetEnvOrDefaultString("POOL_GROUPS", "pool,contributor,pro,prolite,plus")
	m := map[string]bool{}
	for _, g := range strings.Split(raw, ",") {
		if g = strings.TrimSpace(g); g != "" {
			m[g] = true
		}
	}
	return m
}()

// IsPoolGroup reports whether requests in this user group go through the pool.
func IsPoolGroup(group string) bool { return poolGroups[group] }

const (
	fiveHourSeconds = 5 * 3600
	weekSeconds     = 7 * 24 * 3600
)

// ContainsInt reports whether v is in s.
func ContainsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// ResolvePoolChannel chooses the upstream channel for a pool-group request.
//
//   - If the requester is in an account's active owner set, route to that account
//     (owners are never capped on accounts they co-own).
//   - Otherwise pick a contributed account whose consumption by NON-owners is
//     still under its 5h and weekly share caps.
//
// Returns an error (surfaced as 503) when no account has surplus capacity.
func ResolvePoolChannel(c *gin.Context, modelName string) (*model.Channel, error) {
	requesterId := c.GetInt("id")
	entries, _ := model.ListEnabledPoolEntries()
	now := time.Now().Unix()
	since5h := now - fiveHourSeconds
	since7d := now - weekSeconds

	// owner path — route to an account the requester co-owns (uncapped)
	for _, e := range entries {
		if !model.IsChannelEnabledForGroupModel(PoolGroup, modelName, e.ChannelId) {
			continue
		}
		if !model.IsActiveOwner(e.AuthFile, requesterId) {
			continue
		}
		if ch, err := model.GetChannelById(e.ChannelId, true); err == nil && ch.Status == common.ChannelStatusEnabled {
			return ch, nil
		}
	}

	// non-owner path — first account with remaining shared capacity
	for _, e := range entries {
		if !model.IsChannelEnabledForGroupModel(PoolGroup, modelName, e.ChannelId) {
			continue
		}
		ownerIds := model.GetActiveOwnerIds(e.AuthFile)
		if ContainsInt(ownerIds, requesterId) {
			continue
		}
		usage5h, err := model.SumOthersQuota(e.ChannelId, ownerIds, since5h)
		if err != nil || usage5h >= int64(e.ShareCap5h) {
			continue
		}
		usageWeek, err := model.SumOthersQuota(e.ChannelId, ownerIds, since7d)
		if err != nil || usageWeek >= int64(e.ShareCapWeekly) {
			continue
		}
		ch, err := model.GetChannelById(e.ChannelId, true)
		if err != nil || ch.Status != common.ChannelStatusEnabled {
			continue
		}
		return ch, nil
	}
	return nil, errors.New("no contributed account has surplus capacity for this model right now")
}
