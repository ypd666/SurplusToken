// Copyright (C) 2026 SurplusToken Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
// This file is part of SurplusToken, a derivative work of New API
// (Copyright (C) 2023-2026 QuantumNous, AGPLv3).

package model

import "time"

// OAuthFileOwner records which New API user contributed a given CPA OAuth
// auth-file (keyed by the auth-file name, which is the identifier CPA's delete
// endpoint uses). It backs the contribution-protection policy: only the
// contributor or an admin may disconnect a contributed account.
type OAuthFileOwner struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	AuthFile  string `json:"auth_file" gorm:"column:auth_file;type:varchar(191);uniqueIndex"`
	UserId    int    `json:"user_id" gorm:"index"`
	Provider  string `json:"provider" gorm:"type:varchar(64)"`
	CreatedAt int64  `json:"created_at"`
}

func (OAuthFileOwner) TableName() string { return "oauth_file_owners" }

// SetOAuthFileOwner records ownership for an auth-file. The first contributor
// keeps ownership; an existing record is left untouched.
func SetOAuthFileOwner(authFile string, userId int, provider string) error {
	if authFile == "" || userId == 0 {
		return nil
	}
	var existing OAuthFileOwner
	if err := DB.Where("auth_file = ?", authFile).First(&existing).Error; err == nil {
		return nil
	}
	return DB.Create(&OAuthFileOwner{
		AuthFile:  authFile,
		UserId:    userId,
		Provider:  provider,
		CreatedAt: time.Now().Unix(),
	}).Error
}

// GetOAuthFileOwner returns (userId, true) when ownership is recorded.
func GetOAuthFileOwner(authFile string) (int, bool) {
	var o OAuthFileOwner
	if err := DB.Where("auth_file = ?", authFile).First(&o).Error; err != nil {
		return 0, false
	}
	return o.UserId, true
}

// GetOAuthFileOwners returns a map authFile -> userId for the given names.
func GetOAuthFileOwners(authFiles []string) map[string]int {
	out := make(map[string]int, len(authFiles))
	if len(authFiles) == 0 {
		return out
	}
	var rows []OAuthFileOwner
	if err := DB.Where("auth_file IN ?", authFiles).Find(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		out[r.AuthFile] = r.UserId
	}
	return out
}

// DeleteOAuthFileOwner removes the ownership record for an auth-file.
func DeleteOAuthFileOwner(authFile string) error {
	return DB.Where("auth_file = ?", authFile).Delete(&OAuthFileOwner{}).Error
}
