// Copyright (C) 2026 SurplusToken Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
// This file is part of SurplusToken, a derivative work of New API
// (Copyright (C) 2023-2026 QuantumNous, AGPLv3).

package model

import "time"

// Default reservation settings applied when an account is first contributed.
// A large default cap means the account shares generously until the owner
// restricts it; the owner can lower the caps from the OAuth Accounts page.
const (
	DefaultShareCap5h     = 1_000_000_000
	DefaultShareCapWeekly = 1_000_000_000
	DefaultRewardRatio    = 0.5
)

// AccountPoolEntry links a contributed CPA OAuth account (auth-file) to a New
// API channel and its owner, and stores the per-account reservation policy:
// how much OTHER users may consume from the account per 5h window / per week,
// and the reward ratio credited to the owner for that shared consumption.
type AccountPoolEntry struct {
	Id             int     `json:"id" gorm:"primaryKey"`
	AuthFile       string  `json:"auth_file" gorm:"column:auth_file;type:varchar(191);uniqueIndex"`
	AuthId         string  `json:"auth_id" gorm:"column:auth_id;type:varchar(191);index"`
	OwnerUserId    int     `json:"owner_user_id" gorm:"index"`
	ChannelId      int     `json:"channel_id" gorm:"index"`
	ShareCap5h     int     `json:"share_cap_5h" gorm:"column:share_cap_5h"`
	ShareCapWeekly int     `json:"share_cap_weekly" gorm:"column:share_cap_weekly"`
	RewardRatio    float64 `json:"reward_ratio"`
	Enabled        bool    `json:"enabled"`
	CreatedAt      int64   `json:"created_at"`
}

func (AccountPoolEntry) TableName() string { return "account_pool_entries" }

// UpsertAccountPoolEntry creates or updates the entry keyed by auth-file name.
func UpsertAccountPoolEntry(e *AccountPoolEntry) error {
	var existing AccountPoolEntry
	err := DB.Where("auth_file = ?", e.AuthFile).First(&existing).Error
	if err == nil {
		return DB.Model(&AccountPoolEntry{}).Where("id = ?", existing.Id).Updates(map[string]interface{}{
			"auth_id":          e.AuthId,
			"owner_user_id":    e.OwnerUserId,
			"channel_id":       e.ChannelId,
			"share_cap_5h":     e.ShareCap5h,
			"share_cap_weekly": e.ShareCapWeekly,
			"reward_ratio":     e.RewardRatio,
			"enabled":          e.Enabled,
		}).Error
	}
	if e.CreatedAt == 0 {
		e.CreatedAt = time.Now().Unix()
	}
	return DB.Create(e).Error
}

func GetAccountPoolEntryByChannel(channelId int) (*AccountPoolEntry, bool) {
	var e AccountPoolEntry
	if err := DB.Where("channel_id = ?", channelId).First(&e).Error; err != nil {
		return nil, false
	}
	return &e, true
}

func GetAccountPoolEntryByAuthFile(authFile string) (*AccountPoolEntry, bool) {
	var e AccountPoolEntry
	if err := DB.Where("auth_file = ?", authFile).First(&e).Error; err != nil {
		return nil, false
	}
	return &e, true
}

// ListEnabledPoolEntries returns all enabled pool entries (non-owner scheduling).
func ListEnabledPoolEntries() ([]AccountPoolEntry, error) {
	var rows []AccountPoolEntry
	err := DB.Where("enabled = ?", true).Find(&rows).Error
	return rows, err
}

// ListEnabledPoolChannelsForOwner returns enabled entries owned by a user (owner routing).
func ListEnabledPoolChannelsForOwner(ownerUserId int) ([]AccountPoolEntry, error) {
	var rows []AccountPoolEntry
	err := DB.Where("enabled = ? AND owner_user_id = ?", true, ownerUserId).Find(&rows).Error
	return rows, err
}

// ListPoolEntriesByOwner returns all entries owned by a user (UI display).
func ListPoolEntriesByOwner(ownerUserId int) ([]AccountPoolEntry, error) {
	var rows []AccountPoolEntry
	err := DB.Where("owner_user_id = ?", ownerUserId).Find(&rows).Error
	return rows, err
}

func DeleteAccountPoolEntryByAuthFile(authFile string) error {
	return DB.Where("auth_file = ?", authFile).Delete(&AccountPoolEntry{}).Error
}

func UpdateAccountPoolCaps(id, cap5h, capWeekly int) error {
	return DB.Model(&AccountPoolEntry{}).Where("id = ?", id).Updates(map[string]interface{}{
		"share_cap_5h":     cap5h,
		"share_cap_weekly": capWeekly,
	}).Error
}

// SumOthersQuota returns the quota consumed on a pool channel by users NOT in
// the account's active owner set since the given unix timestamp. It backs the
// per-account 5h / weekly share-cap enforcement (owner usage never counts).
func SumOthersQuota(channelId int, ownerIds []int, since int64) (int64, error) {
	var total int64
	q := LOG_DB.Model(&Log{}).Where("channel_id = ? AND created_at >= ?", channelId, since)
	if len(ownerIds) > 0 {
		q = q.Where("user_id NOT IN ?", ownerIds)
	}
	err := q.Select("COALESCE(SUM(quota),0)").Scan(&total).Error
	return total, err
}
