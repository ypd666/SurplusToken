// Copyright (C) 2026 SurplusToken Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
// This file is part of SurplusToken, a derivative work of New API
// (Copyright (C) 2023-2026 QuantumNous, AGPLv3).

import { createFileRoute } from '@tanstack/react-router'
import { OAuthAccounts } from '@/features/oauth-accounts'

export const Route = createFileRoute('/_authenticated/oauth-accounts/')({
  component: OAuthAccounts,
})
