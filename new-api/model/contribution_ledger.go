// Copyright (C) 2026 SurplusToken Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
// This file is part of SurplusToken, a derivative work of New API
// (Copyright (C) 2023-2026 QuantumNous, AGPLv3).

package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ContributionLedger accumulates the reward quota a contributor has earned from
// other users consuming their pooled accounts (Accrued), and how much has been
// moved into their wallet (Transferred).
type ContributionLedger struct {
	Id          int   `json:"id" gorm:"primaryKey"`
	UserId      int   `json:"user_id" gorm:"uniqueIndex"`
	Accrued     int64 `json:"accrued"`
	Transferred int64 `json:"transferred"`
}

func (ContributionLedger) TableName() string { return "contribution_ledgers" }

// GetContributionLedger returns the user's ledger, or a zero-value ledger if none.
func GetContributionLedger(userId int) (*ContributionLedger, error) {
	var l ContributionLedger
	err := DB.Where("user_id = ?", userId).First(&l).Error
	if err == gorm.ErrRecordNotFound {
		return &ContributionLedger{UserId: userId}, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// AccrueContributionReward atomically adds reward quota to the user's ledger.
func AccrueContributionReward(userId int, amount int64) error {
	if amount <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var l ContributionLedger
		err := tx.Where("user_id = ?", userId).First(&l).Error
		if err == gorm.ErrRecordNotFound {
			return tx.Create(&ContributionLedger{UserId: userId, Accrued: amount}).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&ContributionLedger{}).Where("user_id = ?", userId).
			Update("accrued", gorm.Expr("accrued + ?", amount)).Error
	})
}

// TransferContributionToWallet moves all accrued reward into the user's wallet
// quota atomically and returns the moved amount. The user-quota Redis cache is
// refreshed afterwards to stay consistent.
func TransferContributionToWallet(userId int) (int64, error) {
	var moved int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var l ContributionLedger
		if err := tx.Where("user_id = ?", userId).First(&l).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		moved = l.Accrued
		if moved <= 0 {
			return nil
		}
		if err := tx.Model(&ContributionLedger{}).Where("user_id = ?", userId).Updates(map[string]interface{}{
			"accrued":     0,
			"transferred": gorm.Expr("transferred + ?", moved),
		}).Error; err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", userId).
			Update("quota", gorm.Expr("quota + ?", moved)).Error
	})
	if err != nil {
		return 0, err
	}
	if moved > 0 {
		// keep the user-quota cache in sync with the DB change made in the tx
		if cErr := cacheIncrUserQuota(userId, moved); cErr != nil {
			common.SysLog("contribution transfer: failed to refresh user quota cache: " + cErr.Error())
		}
	}
	return moved, nil
}
