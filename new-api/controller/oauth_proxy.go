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
			}
			return
		}
	}
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
		ownerId := owners[n]
		f["owner_user_id"] = ownerId
		f["is_mine"] = ownerId != 0 && ownerId == userId
		// untracked accounts (ownerId == 0) are admin-only by design
		f["can_delete"] = isAdmin || (ownerId != 0 && ownerId == userId)
		if ownerId != 0 {
			if name, errName := model.GetUsernameById(ownerId, false); errName == nil {
				f["owner_name"] = name
			}
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

	ownerId, found := model.GetOAuthFileOwner(name)
	if role < common.RoleAdminUser {
		if !found {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "This account was not contributed by you; only an admin can disconnect it."})
			return
		}
		if ownerId != userId {
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
