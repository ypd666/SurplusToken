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
