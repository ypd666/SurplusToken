// Copyright (C) 2026 SurplusToken Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
// This file is part of SurplusToken, a derivative work of New API
// (Copyright (C) 2023-2026 QuantumNous, AGPLv3).

import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Spinner } from '@/components/ui/spinner'
import { AlertDialog, AlertDialogTrigger, AlertDialogContent, AlertDialogHeader, AlertDialogFooter, AlertDialogTitle, AlertDialogDescription } from '@/components/ui/alert-dialog'
import { toast } from 'sonner'
import {
  getOAuthProviders,
  getOAuthAuthFiles,
  getOAuthAuthorizeURL,
  deleteOAuthAuthFile,
  updateAccountCaps,
  getContribution,
  transferContribution,
} from './api'
import type { OAuthProvider, OAuthAuthFile } from './types'

function ProviderIcon({ icon }: { icon: string }) {
  const icons: Record<string, string> = {
    claude: '🧠',
    codex: '🤖',
    gemini: '💎',
    antigravity: '🌌',
    kimi: '🌙',
    grok: '⚡',
  }
  return <span className='text-xl'>{icons[icon] || '🔌'}</span>
}

export function OAuthAccounts() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [connectingProvider, setConnectingProvider] = useState<string | null>(null)

  // Fetch providers
  const providersQuery = useQuery({
    queryKey: ['oauth-providers'],
    queryFn: getOAuthProviders,
    staleTime: 5 * 60 * 1000,
  })

  // Fetch connected accounts
  const authFilesQuery = useQuery({
    queryKey: ['oauth-auth-files'],
    queryFn: getOAuthAuthFiles,
    refetchInterval: 10_000, // auto-refresh every 10s
  })

  // Delete mutation
  const deleteMutation = useMutation({
    mutationFn: deleteOAuthAuthFile,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['oauth-auth-files'] })
      toast.success(t('Account disconnected successfully'))
    },
    onError: () => {
      toast.error(t('Failed to disconnect account'))
    },
  })

  // Contribution reward balance
  const contributionQuery = useQuery({
    queryKey: ['oauth-contribution'],
    queryFn: getContribution,
    refetchInterval: 30_000,
  })
  const transferMutation = useMutation({
    mutationFn: transferContribution,
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ['oauth-contribution'] })
      toast.success(t('Transferred {{n}} quota to your wallet', { n: res.data?.moved ?? 0 }))
    },
    onError: () => toast.error(t('Transfer failed')),
  })
  const capsMutation = useMutation({
    mutationFn: ({ id, caps }: { id: string; caps: { share_cap_5h?: number; share_cap_weekly?: number } }) =>
      updateAccountCaps(id, caps),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['oauth-auth-files'] })
      toast.success(t('Caps updated'))
    },
    onError: () => toast.error(t('Failed to update caps')),
  })

  const handleEditCaps = (af: OAuthAuthFile) => {
    const in5h = window.prompt(t('Max quota others may use per 5h window'), String(af.share_cap_5h ?? 0))
    if (in5h === null) return
    const inWk = window.prompt(t('Max quota others may use per week'), String(af.share_cap_weekly ?? 0))
    if (inWk === null) return
    capsMutation.mutate({
      id: encodeURIComponent(af.name || af.id),
      caps: { share_cap_5h: Number(in5h), share_cap_weekly: Number(inWk) },
    })
  }

  const handleConnect = async (provider: OAuthProvider) => {
    setConnectingProvider(provider.id)
    try {
      const res = await getOAuthAuthorizeURL(provider.id)
      if (res.success && res.data?.url) {
        // Open OAuth flow in a popup or redirect
        const width = 600
        const height = 700
        const left = window.screenX + (window.outerWidth - width) / 2
        const top = window.screenY + (window.outerHeight - height) / 2
        const popup = window.open(
          res.data.url,
          `oauth-${provider.id}`,
          `width=${width},height=${height},left=${left},top=${top}`,
        )
        if (popup) {
          // Poll for popup close, then refresh
          const timer = setInterval(() => {
            if (popup.closed) {
              clearInterval(timer)
              queryClient.invalidateQueries({ queryKey: ['oauth-auth-files'] })
              setConnectingProvider(null)
              toast.success(t('OAuth flow completed'))
            }
          }, 1000)
        } else {
          // Fallback: direct redirect
          window.location.href = res.data.url
        }
      } else {
        toast.error(res.message || t('Failed to get authorization URL'))
        setConnectingProvider(null)
      }
    } catch {
      toast.error(t('Failed to connect to CPA'))
      setConnectingProvider(null)
    }
  }

  const handleDisconnect = (authFile: OAuthAuthFile) => {
    // CPA identifies auth-files by name; fall back to id if name is absent.
    deleteMutation.mutate(encodeURIComponent(authFile.name || authFile.id))
  }

  const authFiles = authFilesQuery.data?.data || []
  const providers = providersQuery.data?.data || []

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('OAuth Accounts')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {/* Available Providers */}
        <Card className='mb-6'>
          <CardHeader>
            <CardTitle>{t('Connect an Account')}</CardTitle>
            <p className='text-muted-foreground text-sm'>
              {t('Authorize OAuth accounts to share with the platform. Your credentials are stored securely and tokens are auto-refreshed.')}
            </p>
          </CardHeader>
          <CardContent>
            {providersQuery.isLoading ? (
              <div className='flex justify-center py-4'>
                <Spinner />
              </div>
            ) : (
              <div className='grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-4'>
                {providers.map((provider: OAuthProvider) => {
                  const isConnected = authFiles.some((af: OAuthAuthFile) => af.type === provider.id)
                  const isConnecting = connectingProvider === provider.id
                  return (
                    <Button
                      key={provider.id}
                      variant='outline'
                      className='flex h-auto flex-col items-center gap-2 p-4'
                      disabled={isConnected || isConnecting}
                      onClick={() => handleConnect(provider)}
                    >
                      <ProviderIcon icon={provider.icon} />
                      <span className='text-sm font-medium'>{provider.name}</span>
                      {isConnected && (
                        <Badge variant='success' className='text-xs'>
                          {t('Connected')}
                        </Badge>
                      )}
                      {isConnecting && <Spinner className='h-4 w-4' />}
                    </Button>
                  )
                })}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Contribution reward */}
        <Card className='mb-6'>
          <CardHeader>
            <CardTitle>{t('Contribution Reward')}</CardTitle>
            <p className='text-muted-foreground text-sm'>
              {t('Quota you earn when other users consume the accounts you contributed.')}
            </p>
          </CardHeader>
          <CardContent>
            <div className='flex items-center justify-between'>
              <div className='text-sm'>
                <div>
                  {t('Available')}:{' '}
                  <span className='font-medium'>{contributionQuery.data?.data?.accrued ?? 0}</span>
                </div>
                <div className='text-muted-foreground'>
                  {t('Transferred')}: {contributionQuery.data?.data?.transferred ?? 0}
                </div>
              </div>
              <Button
                size='sm'
                disabled={transferMutation.isPending || (contributionQuery.data?.data?.accrued ?? 0) <= 0}
                onClick={() => transferMutation.mutate()}
              >
                {transferMutation.isPending ? <Spinner className='mr-2 h-4 w-4' /> : null}
                {t('Transfer to wallet')}
              </Button>
            </div>
          </CardContent>
        </Card>

        {/* Connected Accounts */}
        <Card>
          <CardHeader>
            <CardTitle>{t('Connected Accounts')}</CardTitle>
            <p className='text-muted-foreground text-sm'>
              {t('These OAuth accounts are being used to serve AI requests. Only the contributor or an admin can disconnect an account.')}
            </p>
          </CardHeader>
          <CardContent>
            {authFilesQuery.isLoading ? (
              <div className='flex justify-center py-8'>
                <Spinner />
              </div>
            ) : authFiles.length === 0 ? (
              <div className='py-8 text-center text-muted-foreground'>
                {t('No accounts connected yet. Connect your first account above.')}
              </div>
            ) : (
              <div className='overflow-x-auto'>
                <table className='w-full text-sm'>
                  <thead>
                    <tr className='border-b text-left'>
                      <th className='pb-2 font-medium'>{t('Provider')}</th>
                      <th className='pb-2 font-medium'>{t('Account')}</th>
                      <th className='pb-2 font-medium'>{t('Contributor')}</th>
                      <th className='pb-2 font-medium'>{t('Status')}</th>
                      <th className='pb-2 font-medium'>{t('Shared (5h / week)')}</th>
                      <th className='pb-2 text-right font-medium'>{t('Actions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {authFiles.map((af: OAuthAuthFile) => (
                      <tr key={af.id} className='border-b last:border-0'>
                        <td className='py-3'>
                          <div className='flex items-center gap-2'>
                            <ProviderIcon icon={af.type} />
                            <span className='capitalize'>{af.type}</span>
                          </div>
                        </td>
                        <td className='py-3 text-muted-foreground'>
                          {af.email || af.name || af.id}
                        </td>
                        <td className='py-3'>
                          {af.is_mine ? (
                            <Badge variant='success' className='text-xs'>{t('You')}</Badge>
                          ) : af.owner_name ? (
                            <span className='text-muted-foreground'>{af.owner_name}</span>
                          ) : (
                            <span className='text-muted-foreground'>—</span>
                          )}
                        </td>
                        <td className='py-3'>
                          {af.disabled ? (
                            <Badge variant='destructive'>{t('Disabled')}</Badge>
                          ) : af.quota?.exceeded ? (
                            <Badge variant='warning'>{t('Rate Limited')}</Badge>
                          ) : (
                            <Badge variant='success'>{t('Active')}</Badge>
                          )}
                        </td>
                        <td className='py-3 text-muted-foreground'>
                          {af.share_cap_5h !== undefined ? (
                            <span className='whitespace-nowrap'>
                              {(af.others_usage_5h ?? 0)}/{af.share_cap_5h}
                              {' · '}
                              {(af.others_usage_weekly ?? 0)}/{af.share_cap_weekly}
                            </span>
                          ) : (
                            <span>—</span>
                          )}
                        </td>
                        <td className='py-3 text-right'>
                          {af.is_mine && af.share_cap_5h !== undefined && (
                            <Button variant='ghost' size='sm' onClick={() => handleEditCaps(af)}>
                              {t('Edit caps')}
                            </Button>
                          )}
                          {af.can_delete === false ? (
                            <Button
                              variant='ghost'
                              size='sm'
                              disabled
                              title={t('Only the contributor or an admin can disconnect this account')}
                            >
                              {t('Disconnect')}
                            </Button>
                          ) : (
                            <AlertDialog>
                              <AlertDialogTrigger asChild>
                                <Button variant='ghost' size='sm'>
                                  {t('Disconnect')}
                                </Button>
                              </AlertDialogTrigger>
                              <AlertDialogContent>
                                <AlertDialogHeader>
                                  <AlertDialogTitle>{t('Disconnect Account')}</AlertDialogTitle>
                                  <AlertDialogDescription>
                                    {t('Are you sure you want to disconnect this account? It will no longer be available for AI requests.')}
                                  </AlertDialogDescription>
                                </AlertDialogHeader>
                                <AlertDialogFooter>
                                  <Button
                                    variant='destructive'
                                    onClick={() => handleDisconnect(af)}
                                    disabled={deleteMutation.isPending}
                                  >
                                    {deleteMutation.isPending ? <Spinner className='mr-2 h-4 w-4' /> : null}
                                    {t('Disconnect')}
                                  </Button>
                                </AlertDialogFooter>
                              </AlertDialogContent>
                            </AlertDialog>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </CardContent>
        </Card>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
