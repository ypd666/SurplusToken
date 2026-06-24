// Copyright (C) 2026 SurplusToken Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
// This file is part of SurplusToken, a derivative work of New API
// (Copyright (C) 2023-2026 QuantumNous, AGPLv3).

package service

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// PoolGroup is the user group whose requests are routed through the contributed
// account reservation pool.
const PoolGroup = "pool"

const (
	fiveHourSeconds = 5 * 3600
	weekSeconds     = 7 * 24 * 3600
)

// ResolvePoolChannel chooses the upstream channel for a pool-group request.
//
//   - If the requester contributed an account that can serve the model, route to
//     their OWN account (the owner is never capped on their own account).
//   - Otherwise pick a contributed account whose consumption by OTHER users is
//     still under its 5h and weekly share caps.
//
// Returns an error (surfaced as 503) when no account has surplus capacity.
func ResolvePoolChannel(c *gin.Context, modelName string) (*model.Channel, error) {
	requesterId := c.GetInt("id")

	// owner path — prefer the requester's own contributed account
	if owned, _ := model.ListEnabledPoolChannelsForOwner(requesterId); len(owned) > 0 {
		for _, e := range owned {
			if !model.IsChannelEnabledForGroupModel(PoolGroup, modelName, e.ChannelId) {
				continue
			}
			if ch, err := model.GetChannelById(e.ChannelId, true); err == nil && ch.Status == common.ChannelStatusEnabled {
				return ch, nil
			}
		}
	}

	// non-owner path — first account with remaining shared capacity
	entries, _ := model.ListEnabledPoolEntries()
	now := time.Now().Unix()
	since5h := now - fiveHourSeconds
	since7d := now - weekSeconds
	for _, e := range entries {
		if e.OwnerUserId == requesterId {
			continue
		}
		if !model.IsChannelEnabledForGroupModel(PoolGroup, modelName, e.ChannelId) {
			continue
		}
		usage5h, err := model.SumOthersQuota(e.ChannelId, e.OwnerUserId, since5h)
		if err != nil || usage5h >= int64(e.ShareCap5h) {
			continue
		}
		usageWeek, err := model.SumOthersQuota(e.ChannelId, e.OwnerUserId, since7d)
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
