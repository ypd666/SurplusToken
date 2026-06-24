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
  listAccountOwners,
  addAccountOwner,
  removeAccountOwner,
  approveAccountOwner,
  requestJoinAccount,
  completeOAuthCallback,
} from './api'
import type { OAuthProvider, OAuthAuthFile, AccountOwner } from './types'
import { useIsAdmin } from '@/hooks/use-admin'
import { formatQuotaWithCurrency } from '@/lib/currency'

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
  const [pasteDialogOpen, setPasteDialogOpen] = useState(false)
  const [pasteUrl, setPasteUrl] = useState('')

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
      toast.success(t('Transferred {{n}} to your wallet', { n: formatQuotaWithCurrency(res.data?.moved ?? 0) }))
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

  // Multi-owner management
  const isAdmin = useIsAdmin()
  const ownerId = (af: OAuthAuthFile) => encodeURIComponent(af.name || af.id)
  const [ownersFor, setOwnersFor] = useState<OAuthAuthFile | null>(null)
  const ownersQuery = useQuery({
    queryKey: ['oauth-owners', ownersFor ? ownerId(ownersFor) : null],
    queryFn: () => listAccountOwners(ownerId(ownersFor!)),
    enabled: !!ownersFor,
  })
  const joinMutation = useMutation({
    mutationFn: (af: OAuthAuthFile) => requestJoinAccount(ownerId(af)),
    onSuccess: (res) => toast.success(res.message || t('Join request submitted')),
    onError: () => toast.error(t('Failed to request join')),
  })
  const ownerMutation = useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['oauth-owners'] })
      queryClient.invalidateQueries({ queryKey: ['oauth-auth-files'] })
    },
    onError: () => toast.error(t('Operation failed')),
  })
  const handleAddOwner = (af: OAuthAuthFile) => {
    const uid = window.prompt(t('User ID to add as co-owner'))
    if (!uid) return
    ownerMutation.mutate(() => addAccountOwner(ownerId(af), Number(uid)))
  }

  const handleConnect = async (provider: OAuthProvider) => {
    setConnectingProvider(provider.id)
    try {
      const res = await getOAuthAuthorizeURL(provider.id)
      if (res.success && res.data?.url) {
        const width = 600
        const height = 700
        const left = window.screenX + (window.outerWidth - width) / 2
        const top = window.screenY + (window.outerHeight - height) / 2
        window.open(
          res.data.url,
          `oauth-${provider.id}`,
          `width=${width},height=${height},left=${left},top=${top}`,
        )
        // The provider redirects to http://localhost:1455/... which can't reach
        // CPA from a remote browser — so collect the redirect URL via the dialog.
        setPasteUrl('')
        setPasteDialogOpen(true)
      } else {
        toast.error(res.message || t('Failed to get authorization URL'))
      }
    } catch {
      toast.error(t('Failed to connect to CPA'))
    } finally {
      setConnectingProvider(null)
    }
  }

  const completeMutation = useMutation({
    mutationFn: completeOAuthCallback,
    onSuccess: (res) => {
      if (res.success) {
        setPasteDialogOpen(false)
        setPasteUrl('')
        queryClient.invalidateQueries({ queryKey: ['oauth-auth-files'] })
        toast.success(t('Account connected successfully'))
      } else {
        toast.error(res.message || t('Authorization failed'))
      }
    },
    onError: () => toast.error(t('Authorization failed')),
  })

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
                  <span className='font-medium'>
                    {formatQuotaWithCurrency(contributionQuery.data?.data?.accrued ?? 0)}
                  </span>
                </div>
                <div className='text-muted-foreground'>
                  {t('Transferred')}: {formatQuotaWithCurrency(contributionQuery.data?.data?.transferred ?? 0)}
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
                          {(af.owner_count ?? 0) > 1 && (
                            <span className='text-muted-foreground ml-1 text-xs'>+{(af.owner_count ?? 1) - 1}</span>
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
                          {!af.is_mine && (
                            <Button variant='ghost' size='sm' disabled={joinMutation.isPending} onClick={() => joinMutation.mutate(af)}>
                              {t('Request to join')}
                            </Button>
                          )}
                          {isAdmin && (
                            <Button variant='ghost' size='sm' onClick={() => setOwnersFor(af)}>
                              {t('Owners')}
                            </Button>
                          )}
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
        {/* Finish-connect dialog: paste the localhost redirect URL */}
        <AlertDialog open={pasteDialogOpen} onOpenChange={(o) => !o && setPasteDialogOpen(false)}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t('Finish connecting your account')}</AlertDialogTitle>
              <AlertDialogDescription>
                {t('Authorization redirects to a localhost address that only works on the server. Complete it here by pasting that address.')}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <ol className='list-decimal space-y-1 py-2 pl-5 text-sm text-muted-foreground'>
              <li>{t('Log in and authorize in the popup window.')}</li>
              <li>{t('After you authorize, the browser opens an http://localhost:1455/... page that fails to load — that is expected.')}</li>
              <li>{t('Copy the FULL address from that page address bar (it starts with http://localhost and contains code= and state=).')}</li>
              <li>{t('Paste it below and click Finish.')}</li>
            </ol>
            <textarea
              className='border-input bg-background mt-1 w-full rounded-md border p-2 text-sm'
              rows={3}
              placeholder='http://localhost:1455/auth/callback?code=...&state=...'
              value={pasteUrl}
              onChange={(e) => setPasteUrl(e.target.value)}
            />
            <AlertDialogFooter>
              <Button variant='ghost' size='sm' onClick={() => setPasteDialogOpen(false)}>
                {t('Cancel')}
              </Button>
              <Button
                size='sm'
                disabled={completeMutation.isPending || !pasteUrl.trim()}
                onClick={() => completeMutation.mutate(pasteUrl.trim())}
              >
                {completeMutation.isPending ? <Spinner className='mr-2 h-4 w-4' /> : null}
                {t('Finish connecting')}
              </Button>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>

        {/* Owners management dialog (admin) */}
        <AlertDialog open={!!ownersFor} onOpenChange={(o) => !o && setOwnersFor(null)}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t('Account Owners')}</AlertDialogTitle>
              <AlertDialogDescription>
                {ownersFor?.email || ownersFor?.name || ownersFor?.id}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <div className='space-y-2 py-2'>
              {(ownersQuery.data?.data || []).map((o: AccountOwner) => (
                <div key={o.user_id} className='flex items-center justify-between text-sm'>
                  <span>
                    {o.username || o.user_id}
                    {o.primary && <span className='text-muted-foreground'> ({t('primary')})</span>}
                    {o.status === 'pending' && (
                      <Badge variant='warning' className='ml-1 text-xs'>{t('pending')}</Badge>
                    )}
                  </span>
                  <span className='flex gap-1'>
                    {o.status === 'pending' && (
                      <Button
                        size='sm'
                        variant='ghost'
                        onClick={() => ownerMutation.mutate(() => approveAccountOwner(ownerId(ownersFor!), o.user_id))}
                      >
                        {t('Approve')}
                      </Button>
                    )}
                    {!o.primary && (
                      <Button
                        size='sm'
                        variant='ghost'
                        onClick={() => ownerMutation.mutate(() => removeAccountOwner(ownerId(ownersFor!), o.user_id))}
                      >
                        {t('Remove')}
                      </Button>
                    )}
                  </span>
                </div>
              ))}
            </div>
            <AlertDialogFooter>
              <Button variant='outline' size='sm' onClick={() => ownersFor && handleAddOwner(ownersFor)}>
                {t('Add co-owner')}
              </Button>
              <Button variant='ghost' size='sm' onClick={() => setOwnersFor(null)}>
                {t('Close')}
              </Button>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
