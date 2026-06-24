// Copyright (C) 2026 SurplusToken Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
// This file is part of SurplusToken.

// Types for the OAuth Accounts feature
export interface OAuthProvider {
  id: string
  name: string
  icon: string
}

export interface OAuthAuthFile {
  id: string
  type: string
  email?: string
  name?: string
  status?: string
  disabled?: boolean
  quota?: {
    exceeded?: boolean
    next_retry_after?: string
  }
  models?: string[]
  // Contribution-protection metadata (added by the backend proxy)
  owner_user_id?: number
  owner_name?: string
  is_mine?: boolean
  can_delete?: boolean
  // Reservation pool metadata (present when the account is in the pool)
  share_cap_5h?: number
  share_cap_weekly?: number
  others_usage_5h?: number
  others_usage_weekly?: number
  reward_ratio?: number
  pool_enabled?: boolean
  owner_count?: number
  owner_names?: string[]
}

export interface AccountOwner {
  user_id: number
  username?: string
  status: string
  primary: boolean
}

export interface OAuthAuthorizeResponse {
  success: boolean
  data?: {
    url: string
    state: string
  }
  message?: string
}

export interface OAuthAuthFilesResponse {
  success: boolean
  data?: OAuthAuthFile[]
  message?: string
}

export interface OAuthStatusResponse {
  success: boolean
  data?: {
    status: string
    auth_file?: OAuthAuthFile
  }
  message?: string
}

export interface CPAHealthResponse {
  success: boolean
  data?: {
    cpa_reachable: boolean
    cpa_base_url: string
    error?: string
  }
  message?: string
}
