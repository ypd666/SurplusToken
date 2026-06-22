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
