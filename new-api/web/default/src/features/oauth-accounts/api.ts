// Copyright (C) 2026 SurplusToken Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
// This file is part of SurplusToken.

import { api } from '@/lib/api'
import type {
  OAuthProvider,
  OAuthAuthorizeResponse,
  OAuthAuthFilesResponse,
  OAuthStatusResponse,
  CPAHealthResponse,
} from './types'

export async function getOAuthProviders(): Promise<{ success: boolean; data: OAuthProvider[] }> {
  const res = await api.get('/api/oauth-provider/providers')
  return res.data
}

export async function getOAuthAuthorizeURL(
  provider: string,
): Promise<OAuthAuthorizeResponse> {
  const res = await api.get(`/api/oauth-provider/${provider}/authorize`)
  return res.data
}

export async function getOAuthAuthFiles(): Promise<OAuthAuthFilesResponse> {
  const res = await api.get('/api/oauth-provider/auth-files')
  return res.data
}

export async function deleteOAuthAuthFile(id: string): Promise<{ success: boolean; message?: string }> {
  const res = await api.delete(`/api/oauth-provider/auth-files/${id}`)
  return res.data
}

export async function getOAuthStatus(state: string): Promise<OAuthStatusResponse> {
  const res = await api.get('/api/oauth-provider/status', { params: { state } })
  return res.data
}

export async function getCPAHealth(): Promise<CPAHealthResponse> {
  const res = await api.get('/api/oauth-provider/health')
  return res.data
}

export async function updateAccountCaps(
  id: string,
  caps: { share_cap_5h?: number; share_cap_weekly?: number },
): Promise<{ success: boolean; message?: string }> {
  const res = await api.put(`/api/oauth-provider/auth-files/${id}/caps`, caps)
  return res.data
}

export async function getContribution(): Promise<{
  success: boolean
  data?: { accrued: number; transferred: number }
}> {
  const res = await api.get('/api/oauth-provider/contribution')
  return res.data
}

export async function transferContribution(): Promise<{
  success: boolean
  data?: { moved: number }
  message?: string
}> {
  const res = await api.post('/api/oauth-provider/contribution/transfer')
  return res.data
}
