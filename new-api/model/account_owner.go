// Copyright (C) 2026 SurplusToken Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
// This file is part of SurplusToken, a derivative work of New API
// (Copyright (C) 2023-2026 QuantumNous, AGPLv3).

package model

import "time"

const (
	OwnerStatusActive  = "active"
	OwnerStatusPending = "pending"
)

// AccountOwner holds ADDITIONAL co-owners of a contributed account, beyond the
// primary contributor recorded in oauth_file_owners. A co-owner is either added
// directly by an admin (status=active) or requested by a user (status=pending)
// and later approved by an admin. The active owner set of an account is the
// primary contributor plus all active co-owners; contribution reward is split
// evenly across that set.
type AccountOwner struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	AuthFile  string `json:"auth_file" gorm:"column:auth_file;type:varchar(191);index;uniqueIndex:uniq_authfile_user"`
	UserId    int    `json:"user_id" gorm:"index;uniqueIndex:uniq_authfile_user"`
	Status    string `json:"status" gorm:"type:varchar(16)"`
	CreatedAt int64  `json:"created_at"`
}

func (AccountOwner) TableName() string { return "account_co_owners" }

func dedupeInts(in []int) []int {
	seen := make(map[int]struct{}, len(in))
	out := make([]int, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// AddCoOwner creates a co-owner row (or upgrades its status if it already exists).
func AddCoOwner(authFile string, userId int, status string) error {
	var existing AccountOwner
	err := DB.Where("auth_file = ? AND user_id = ?", authFile, userId).First(&existing).Error
	if err == nil {
		if existing.Status != status {
			return DB.Model(&AccountOwner{}).Where("id = ?", existing.Id).Update("status", status).Error
		}
		return nil
	}
	return DB.Create(&AccountOwner{AuthFile: authFile, UserId: userId, Status: status, CreatedAt: time.Now().Unix()}).Error
}

// ApproveCoOwner marks a pending co-owner active.
func ApproveCoOwner(authFile string, userId int) error {
	return DB.Model(&AccountOwner{}).Where("auth_file = ? AND user_id = ?", authFile, userId).
		Update("status", OwnerStatusActive).Error
}

// RemoveCoOwner removes a co-owner (reject or revoke).
func RemoveCoOwner(authFile string, userId int) error {
	return DB.Where("auth_file = ? AND user_id = ?", authFile, userId).Delete(&AccountOwner{}).Error
}

// DeleteCoOwnersByAuthFile clears all co-owners for an account (on disconnect).
func DeleteCoOwnersByAuthFile(authFile string) error {
	return DB.Where("auth_file = ?", authFile).Delete(&AccountOwner{}).Error
}

// ListCoOwners returns all co-owner rows (active + pending) for an account.
func ListCoOwners(authFile string) ([]AccountOwner, error) {
	var rows []AccountOwner
	err := DB.Where("auth_file = ?", authFile).Order("id").Find(&rows).Error
	return rows, err
}

// GetActiveOwnerIds returns the active owner set: the primary contributor
// (oauth_file_owners) plus all active co-owners.
func GetActiveOwnerIds(authFile string) []int {
	ids := make([]int, 0, 4)
	if primary, ok := GetOAuthFileOwner(authFile); ok && primary != 0 {
		ids = append(ids, primary)
	}
	var rows []AccountOwner
	DB.Where("auth_file = ? AND status = ?", authFile, OwnerStatusActive).Find(&rows)
	for _, r := range rows {
		ids = append(ids, r.UserId)
	}
	return dedupeInts(ids)
}

// IsActiveOwner reports whether userId is in the account's active owner set.
func IsActiveOwner(authFile string, userId int) bool {
	for _, id := range GetActiveOwnerIds(authFile) {
		if id == userId {
			return true
		}
	}
	return false
}
