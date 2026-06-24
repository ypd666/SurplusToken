// Copyright (C) 2026 SurplusToken Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This file is part of SurplusToken, a derivative work of New API
// (Copyright (C) 2023-2026 QuantumNous, AGPLv3).

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// =============================================================================
// CPA (CLIProxyAPI) OAuth Proxy Controller
// Proxies New API frontend requests to CPA's management API (/v0/management/*)
// so users can manage their OAuth upstream accounts through the New API UI, and
// wraps the raw CPA payloads in New API's {success,data} envelope.
//
// Contribution-protection policy (option B): the account pool is visible to all
// authenticated users, but only the contributor or an admin may disconnect an
// account. Ownership is tracked per auth-file name in model.OAuthFileOwner.
// =============================================================================

const (
	// defaultCPABaseURL is the CPA gateway internal address (override via CPA_BASE_URL).
	defaultCPABaseURL = "http://cpa:8317"
)

var (
	cpaClientOnce sync.Once
	cpaClient     *http.Client
)

func getCPAClient() *http.Client {
	cpaClientOnce.Do(func() {
		cpaClient = &http.Client{Timeout: 15 * time.Second}
	})
	return cpaClient
}

func cpaBaseURL() string {
	return strings.TrimRight(common.GetEnvOrDefaultString("CPA_BASE_URL", defaultCPABaseURL), "/")
}

func cpaManagementKey() string {
	return common.GetEnvOrDefaultString("CPA_MANAGEMENT_KEY", "")
}

// cpaDo issues an authenticated request to CPA's management API.
func cpaDo(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, cpaBaseURL()+path, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if key := cpaManagementKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("X-Management-Key", key)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return getCPAClient().Do(req)
}

// fetchCPAAuthFiles returns CPA's auth-file list (the {"files":[...]} payload).
func fetchCPAAuthFiles(ctx context.Context) ([]map[string]interface{}, error) {
	resp, err := cpaDo(ctx, http.MethodGet, "/v0/management/auth-files", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed struct {
		Files []map[string]interface{} `json:"files"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse auth-files: %w", err)
	}
	return parsed.Files, nil
}

// authFileName returns the auth-file name (the key CPA's delete endpoint uses).
func authFileName(f map[string]interface{}) string {
	if v, ok := f["name"].(string); ok && v != "" {
		return v
	}
	switch v := f["id"].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

// containsIntC reports whether v is in s.
func containsIntC(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// authFileID returns the CPA auth.ID (used as the X-Pinned-Auth-Id value).
func authFileID(f map[string]interface{}) string {
	switch v := f["id"].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

// =============================================================================
// Provider list
// =============================================================================

// supportedOAuthProviders is intentionally limited to OpenAI Codex.
var supportedOAuthProviders = []gin.H{
	{"id": "codex", "name": "OpenAI Codex", "icon": "codex"},
}

// GetOAuthProviders returns the list of supported OAuth providers.
func GetOAuthProviders(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": supportedOAuthProviders})
}

// =============================================================================
// OAuth Authorization URL
// =============================================================================

// GetOAuthAuthorizeURL initiates an OAuth flow and returns the authorization URL.
// It also starts background attribution so the resulting credential is recorded
// as contributed by the current user.
// GET /api/oauth-provider/:provider/authorize
func GetOAuthAuthorizeURL(c *gin.Context) {
	provider := c.Param("provider")
	userId := c.GetInt("id")

	// snapshot existing auth-file names so we can attribute the newly created one
	before := map[string]bool{}
	if files, err := fetchCPAAuthFiles(c.Request.Context()); err == nil {
		for _, f := range files {
			if n := authFileName(f); n != "" {
				before[n] = true
			}
		}
	}

	resp, err := cpaDo(c.Request.Context(), http.MethodGet, fmt.Sprintf("/v0/management/%s-auth-url", provider), nil)
	if err != nil {
		common.ApiErrorMsg(c, "CPA unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed struct {
		Status string `json:"status"`
		URL    string `json:"url"`
		State  string `json:"state"`
		Error  string `json:"error"`
	}
	_ = json.Unmarshal(raw, &parsed)
	if parsed.Status != "ok" || parsed.URL == "" {
		msg := parsed.Error
		if msg == "" {
			msg = "failed to get authorization URL"
		}
		c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
		return
	}

	if userId != 0 && parsed.State != "" {
		go captureOAuthOwnership(parsed.State, provider, userId, before)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"url": parsed.URL, "state": parsed.State},
	})
}

// captureOAuthOwnership polls CPA until the OAuth flow finishes, then records the
// current user as the contributor for any auth-file that did not exist before.
func captureOAuthOwnership(state, provider string, userId int, before map[string]bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		resp, err := cpaDo(ctx, http.MethodGet, "/v0/management/get-auth-status?state="+url.QueryEscape(state), nil)
		if err != nil {
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		var st struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(raw, &st)
		switch st.Status {
		case "wait", "":
			continue
		case "error":
			return
		default: // "ok" -> the flow completed (session removed)
			files, err := fetchCPAAuthFiles(ctx)
			if err != nil {
				return
			}
			for _, f := range files {
				n := authFileName(f)
				if n == "" || before[n] {
					continue
				}
				_ = model.SetOAuthFileOwner(n, userId, provider)
				provisionPoolAccount(ctx, n, authFileID(f), provider, userId)
			}
			return
		}
	}
}

// =============================================================================
// Pool channel provisioning (contribution reservation)
//
// Each contributed account becomes a dedicated OpenAI-compatible channel in the
// "pool" group, carrying an X-Pinned-Auth-Id header so the (patched) CPA pins
// every request routed through that channel to this exact upstream account.
// =============================================================================

const poolChannelGroup = "pool"

// cpaChannelTemplate clones base_url + inbound key from an existing CPA channel
// so pool channels target the same gateway identically.
func cpaChannelTemplate() (baseURL, key string) {
	baseURL, key = "http://cpa:8317", "sk-surplustoken-gateway-internal"
	var ch model.Channel
	if err := model.DB.Where("base_url LIKE ?", "%cpa:8317%").First(&ch).Error; err == nil {
		if ch.BaseURL != nil && *ch.BaseURL != "" {
			baseURL = *ch.BaseURL
		}
		if ch.Key != "" {
			key = ch.Key
		}
	}
	return
}

// fetchAuthFileModels returns the model ids a given auth-file can serve.
func fetchAuthFileModels(ctx context.Context, authFile string) []string {
	resp, err := cpaDo(ctx, http.MethodGet, "/v0/management/auth-files/models?name="+url.QueryEscape(authFile), nil)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed struct {
		Models []map[string]interface{} `json:"models"`
	}
	if json.Unmarshal(raw, &parsed) != nil {
		return nil
	}
	out := make([]string, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		if id, ok := m["id"].(string); ok && id != "" {
			out = append(out, id)
		}
	}
	return out
}

// provisionPoolAccount creates the pool channel + entry for a contributed account.
func provisionPoolAccount(ctx context.Context, authFile, authId, provider string, ownerUserId int) {
	if authId == "" {
		return
	}
	if e, ok := model.GetAccountPoolEntryByAuthFile(authFile); ok && e.ChannelId != 0 {
		return // already provisioned
	}
	models := fetchAuthFileModels(ctx, authFile)
	if len(models) == 0 {
		common.SysLog("pool: no models for auth-file " + authFile + "; skipping channel creation")
		return
	}
	baseURL, key := cpaChannelTemplate()
	headerOverride := fmt.Sprintf(`{"X-Pinned-Auth-Id":%q}`, authId)
	ch := model.Channel{
		Type:           constant.ChannelTypeOpenAI,
		Name:           "pool:" + authFile,
		Key:            key,
		BaseURL:        &baseURL,
		Group:          poolChannelGroup,
		Models:         strings.Join(models, ","),
		Status:         common.ChannelStatusEnabled,
		HeaderOverride: &headerOverride,
	}
	if err := ch.Insert(); err != nil {
		common.SysLog("pool: failed to create channel for " + authFile + ": " + err.Error())
		return
	}
	_ = model.UpsertAccountPoolEntry(&model.AccountPoolEntry{
		AuthFile:       authFile,
		AuthId:         authId,
		OwnerUserId:    ownerUserId,
		ChannelId:      ch.Id,
		ShareCap5h:     model.DefaultShareCap5h,
		ShareCapWeekly: model.DefaultShareCapWeekly,
		RewardRatio:    model.DefaultRewardRatio,
		Enabled:        true,
	})
	model.InitChannelCache()
}

// deprovisionPoolAccount removes the channel + pool entry for a disconnected account.
func deprovisionPoolAccount(authFile string) {
	if e, ok := model.GetAccountPoolEntryByAuthFile(authFile); ok && e.ChannelId != 0 {
		ch := model.Channel{Id: e.ChannelId}
		_ = ch.Delete()
	}
	_ = model.DeleteAccountPoolEntryByAuthFile(authFile)
	model.InitChannelCache()
}

// =============================================================================
// OAuth Auth Files (the shared account pool)
// =============================================================================

// GetOAuthAuthFiles lists the shared pool. Every authenticated user sees all
// contributed accounts; each entry carries owner_user_id / owner_name / is_mine /
// can_delete so the UI can enforce "only the contributor or an admin may delete".
// GET /api/oauth-provider/auth-files
func GetOAuthAuthFiles(c *gin.Context) {
	userId := c.GetInt("id")
	role := c.GetInt("role")
	files, err := fetchCPAAuthFiles(c.Request.Context())
	if err != nil {
		common.ApiErrorMsg(c, "CPA error: "+err.Error())
		return
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		if n := authFileName(f); n != "" {
			names = append(names, n)
		}
	}
	owners := model.GetOAuthFileOwners(names)
	isAdmin := role >= common.RoleAdminUser
	for _, f := range files {
		n := authFileName(f)
		primaryId := owners[n]
		ownerIds := model.GetActiveOwnerIds(n)
		isMine := containsIntC(ownerIds, userId)
		f["owner_user_id"] = primaryId
		f["is_mine"] = isMine
		f["owner_count"] = len(ownerIds)
		// only the sole owner (or an admin) may disconnect; multi-owner = admin-only
		f["can_delete"] = isAdmin || (len(ownerIds) == 1 && ownerIds[0] == userId)
		if primaryId != 0 {
			if name, errName := model.GetUsernameById(primaryId, false); errName == nil {
				f["owner_name"] = name
			}
		}
		ownerNames := make([]string, 0, len(ownerIds))
		for _, oid := range ownerIds {
			if name, e := model.GetUsernameById(oid, false); e == nil {
				ownerNames = append(ownerNames, name)
			}
		}
		f["owner_names"] = ownerNames
		// pool reservation status: caps + how much NON-owners consumed per window
		if entry, ok := model.GetAccountPoolEntryByAuthFile(n); ok {
			now := time.Now().Unix()
			u5h, _ := model.SumOthersQuota(entry.ChannelId, ownerIds, now-5*3600)
			uWk, _ := model.SumOthersQuota(entry.ChannelId, ownerIds, now-7*24*3600)
			f["share_cap_5h"] = entry.ShareCap5h
			f["share_cap_weekly"] = entry.ShareCapWeekly
			f["others_usage_5h"] = u5h
			f["others_usage_weekly"] = uWk
			f["reward_ratio"] = entry.RewardRatio
			f["pool_enabled"] = entry.Enabled
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": files})
}

// DeleteOAuthAuthFile disconnects an account by its auth-file name. Non-admins
// may only delete accounts they contributed; untracked accounts are admin-only.
// DELETE /api/oauth-provider/auth-files/:id   (the path value is the auth-file name)
func DeleteOAuthAuthFile(c *gin.Context) {
	name := c.Param("id")
	userId := c.GetInt("id")
	role := c.GetInt("role")

	ownerIds := model.GetActiveOwnerIds(name)
	if role < common.RoleAdminUser {
		if len(ownerIds) == 0 {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "This account was not contributed by you; only an admin can disconnect it."})
			return
		}
		if len(ownerIds) >= 2 {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "This account has multiple owners; only an admin can disconnect it."})
			return
		}
		if ownerIds[0] != userId {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "You can only disconnect accounts you contributed."})
			return
		}
	}

	resp, err := cpaDo(c.Request.Context(), http.MethodDelete, "/v0/management/auth-files?name="+url.QueryEscape(name), nil)
	if err != nil {
		common.ApiErrorMsg(c, "CPA unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_ = model.DeleteOAuthFileOwner(name)
		_ = model.DeleteCoOwnersByAuthFile(name)
		deprovisionPoolAccount(name)
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "disconnected"})
		return
	}
	var result interface{}
	if json.Unmarshal(raw, &result) == nil {
		c.JSON(resp.StatusCode, gin.H{"success": false, "data": result})
		return
	}
	c.JSON(resp.StatusCode, gin.H{"success": false, "message": string(raw)})
}

// =============================================================================
// OAuth Status Polling
// =============================================================================

// GetOAuthStatus forwards CPA's flow status inside a New API envelope.
// GET /api/oauth-provider/status?state=xxx
func GetOAuthStatus(c *gin.Context) {
	state := c.Query("state")
	resp, err := cpaDo(c.Request.Context(), http.MethodGet, "/v0/management/get-auth-status?state="+url.QueryEscape(state), nil)
	if err != nil {
		common.ApiErrorMsg(c, "CPA unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var st map[string]interface{}
	_ = json.Unmarshal(raw, &st)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": st})
}

// =============================================================================
// CPA Health Check
// =============================================================================

// GetCPAHealth checks if CPA is reachable.
// GET /api/oauth-provider/health
func GetCPAHealth(c *gin.Context) {
	resp, err := cpaDo(c.Request.Context(), http.MethodGet, "/v0/management/get-auth-status?state=ping", nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{
			"cpa_reachable": false, "cpa_base_url": cpaBaseURL(), "error": err.Error(),
		}})
		return
	}
	resp.Body.Close()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{
		"cpa_reachable": resp.StatusCode < 500, "cpa_base_url": cpaBaseURL(),
	}})
}

// =============================================================================
// Reservation caps + contribution reward (owner-facing)
// =============================================================================

// UpdateOAuthAccountCaps lets the contributor (or an admin) set how much OTHER
// users may consume from their account per 5h window / per week.
// PUT /api/oauth-provider/auth-files/:id/caps
func UpdateOAuthAccountCaps(c *gin.Context) {
	name := c.Param("id")
	userId := c.GetInt("id")
	role := c.GetInt("role")
	entry, ok := model.GetAccountPoolEntryByAuthFile(name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "account is not in the pool"})
		return
	}
	if role < common.RoleAdminUser && entry.OwnerUserId != userId {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "only the contributor or an admin can change caps"})
		return
	}
	var body struct {
		ShareCap5h     *int `json:"share_cap_5h"`
		ShareCapWeekly *int `json:"share_cap_weekly"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid body"})
		return
	}
	cap5h, capWk := entry.ShareCap5h, entry.ShareCapWeekly
	if body.ShareCap5h != nil && *body.ShareCap5h >= 0 {
		cap5h = *body.ShareCap5h
	}
	if body.ShareCapWeekly != nil && *body.ShareCapWeekly >= 0 {
		capWk = *body.ShareCapWeekly
	}
	if err := model.UpdateAccountPoolCaps(entry.Id, cap5h, capWk); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetContributionBalance returns the caller's accrued/transferred reward quota.
// GET /api/oauth-provider/contribution
func GetContributionBalance(c *gin.Context) {
	l, err := model.GetContributionLedger(c.GetInt("id"))
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"accrued": l.Accrued, "transferred": l.Transferred,
	}})
}

// TransferContribution moves accrued reward quota into the caller's wallet.
// POST /api/oauth-provider/contribution/transfer
func TransferContribution(c *gin.Context) {
	moved, err := model.TransferContributionToWallet(c.GetInt("id"))
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"moved": moved}})
}

// =============================================================================
// Multi-owner: co-ownership management (admin designate / user join + approve)
// =============================================================================

// ListAccountOwners returns the active + pending owners of an account.
// GET /api/oauth-provider/auth-files/:id/owners
func ListAccountOwners(c *gin.Context) {
	name := c.Param("id")
	out := make([]gin.H, 0, 4)
	if pid, ok := model.GetOAuthFileOwner(name); ok && pid != 0 {
		nm, _ := model.GetUsernameById(pid, false)
		out = append(out, gin.H{"user_id": pid, "username": nm, "status": "active", "primary": true})
	}
	co, _ := model.ListCoOwners(name)
	for _, r := range co {
		nm, _ := model.GetUsernameById(r.UserId, false)
		out = append(out, gin.H{"user_id": r.UserId, "username": nm, "status": r.Status, "primary": false})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// AddAccountOwner designates a co-owner as active (admin only).
// POST /api/oauth-provider/auth-files/:id/owners   {"user_id": N}
func AddAccountOwner(c *gin.Context) {
	name := c.Param("id")
	var body struct {
		UserId int `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.UserId == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_id required"})
		return
	}
	if _, err := model.GetUserById(body.UserId, false); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user not found"})
		return
	}
	if err := model.AddCoOwner(name, body.UserId, model.OwnerStatusActive); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// RemoveAccountOwner removes a co-owner / rejects a pending join (admin only).
// DELETE /api/oauth-provider/auth-files/:id/owners/:userId
func RemoveAccountOwner(c *gin.Context) {
	name := c.Param("id")
	uid, _ := strconv.Atoi(c.Param("userId"))
	if err := model.RemoveCoOwner(name, uid); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ApproveAccountOwner approves a pending join request (admin only).
// POST /api/oauth-provider/auth-files/:id/owners/:userId/approve
func ApproveAccountOwner(c *gin.Context) {
	name := c.Param("id")
	uid, _ := strconv.Atoi(c.Param("userId"))
	if err := model.ApproveCoOwner(name, uid); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// RequestJoinAccount lets a user request to co-own an account (pending approval).
// POST /api/oauth-provider/auth-files/:id/join
func RequestJoinAccount(c *gin.Context) {
	name := c.Param("id")
	userId := c.GetInt("id")
	if model.IsActiveOwner(name, userId) {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "already an owner"})
		return
	}
	if err := model.AddCoOwner(name, userId, model.OwnerStatusPending); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "join request submitted; awaiting admin approval"})
}

// =============================================================================
// Manual OAuth callback (remote/web contributors)
//
// Codex/Claude OAuth redirect to a fixed http://localhost:<port>/auth/callback,
// which only works when the browser runs on the CPA host. For web contributors
// the redirect lands on THEIR machine, so CPA never gets it. This endpoint lets
// the user paste the redirect URL (code+state) and forwards it to CPA's
// oauth-callback to finish the flow. The connect-time goroutine then provisions
// ownership + the pool channel as usual.
// POST /api/oauth-provider/callback  {"redirect_url": "..."}  (or {code, state})
// =============================================================================
func CompleteOAuthCallback(c *gin.Context) {
	var body struct {
		RedirectURL string `json:"redirect_url"`
		Code        string `json:"code"`
		State       string `json:"state"`
	}
	_ = c.ShouldBindJSON(&body)
	code, state := strings.TrimSpace(body.Code), strings.TrimSpace(body.State)
	if body.RedirectURL != "" {
		if u, err := url.Parse(strings.TrimSpace(body.RedirectURL)); err == nil {
			q := u.Query()
			if v := q.Get("code"); v != "" {
				code = v
			}
			if v := q.Get("state"); v != "" {
				state = v
			}
		}
	}
	if code == "" || state == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Could not read code/state — paste the full address the popup redirected to."})
		return
	}
	payload := strings.NewReader(fmt.Sprintf(`{"code":%q,"state":%q}`, code, state))
	resp, err := cpaDo(c.Request.Context(), http.MethodPost, "/v0/management/oauth-callback", payload)
	if err != nil {
		common.ApiErrorMsg(c, "CPA unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	_ = json.Unmarshal(raw, &parsed)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || parsed.Status == "error" {
		msg := parsed.Error
		if msg == "" {
			msg = "authorization callback failed"
		}
		c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
		return
	}

	// The callback only writes the file; CPA exchanges the code for tokens
	// asynchronously. Poll get-auth-status so we report the REAL outcome (and
	// surface the actual error, e.g. a blocked token exchange) instead of a
	// premature success.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(1500 * time.Millisecond)
		sresp, serr := cpaDo(c.Request.Context(), http.MethodGet, "/v0/management/get-auth-status?state="+url.QueryEscape(state), nil)
		if serr != nil {
			continue
		}
		sraw, _ := io.ReadAll(io.LimitReader(sresp.Body, 1<<20))
		sresp.Body.Close()
		var st struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		_ = json.Unmarshal(sraw, &st)
		switch st.Status {
		case "ok":
			c.JSON(http.StatusOK, gin.H{"success": true})
			return
		case "error":
			msg := st.Error
			if msg == "" {
				msg = "authorization failed"
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": false, "message": "still processing — refresh Connected Accounts in a moment"})
}
