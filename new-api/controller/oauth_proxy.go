// Copyright (C) 2026 SurplusToken Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This file is part of SurplusToken, a derivative work of New API
// (Copyright (C) 2023-2026 QuantumNous, AGPLv3).

package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// =============================================================================
// CPA (CLIProxyAPI) OAuth Proxy Controller
// Proxies New API frontend requests to CPA's management API (/v0/management/*)
// so users can manage their OAuth upstream accounts through the New API UI.
// =============================================================================

const (
	// CPA_BASE_URL is the CPA gateway internal address.
	// Override with CPA_BASE_URL env var.
	defaultCPABaseURL = "http://cpa:8317"
)

var (
	cpaClientOnce sync.Once
	cpaClient     *http.Client
)

func getCPAClient() *http.Client {
	cpaClientOnce.Do(func() {
		cpaClient = &http.Client{
			Timeout: 15 * time.Second,
		}
	})
	return cpaClient
}

// cpaBaseURL returns the CPA base URL from environment or default.
func cpaBaseURL() string {
	u := common.GetEnvOrDefaultString("CPA_BASE_URL", defaultCPABaseURL)
	return strings.TrimRight(u, "/")
}

// cpaManagementKey returns the CPA management key for auth.
func cpaManagementKey() string {
	return common.GetEnvOrDefaultString("CPA_MANAGEMENT_KEY", "")
}

// cpaRequest makes an authenticated request to CPA's management API.
func cpaRequest(c *gin.Context, method, path string, body io.Reader) (*http.Response, error) {
	baseURL := cpaBaseURL()
	url := baseURL + path

	req, err := http.NewRequestWithContext(c.Request.Context(), method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// CPA management auth: Bearer token
	if key := cpaManagementKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	// Also support X-Management-Key header
	if key := cpaManagementKey(); key != "" {
		req.Header.Set("X-Management-Key", key)
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return getCPAClient().Do(req)
}

// readCPAResponse reads the CPA response body and writes it to the Gin context.
func readCPAResponse(c *gin.Context, resp *http.Response) {
	defer resp.Body.Close()

	// Read up to 1MB to avoid OOM
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		common.ApiErrorMsg(c, "Read CPA response: "+err.Error())
		return
	}

	// Try to parse as JSON for forwarding
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
		return
	}

	c.JSON(resp.StatusCode, result)
}

// =============================================================================
// Provider list
// =============================================================================

// supportedOAuthProviders lists CPA's built-in OAuth providers.
var supportedOAuthProviders = []gin.H{
	{"id": "claude", "name": "Anthropic Claude", "icon": "claude"},
	{"id": "codex", "name": "OpenAI Codex", "icon": "codex"},
	{"id": "gemini", "name": "Google Gemini", "icon": "gemini"},
	{"id": "antigravity", "name": "Antigravity", "icon": "antigravity"},
	{"id": "kimi", "name": "Kimi (Moonshot)", "icon": "kimi"},
	{"id": "xai", "name": "xAI Grok", "icon": "grok"},
}

// GetOAuthProviders returns the list of supported OAuth providers.
func GetOAuthProviders(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    supportedOAuthProviders,
	})
}

// =============================================================================
// OAuth Authorization URL
// =============================================================================

// GetOAuthAuthorizeURL initiates an OAuth flow and returns the authorization URL.
// GET /api/oauth-provider/:provider/authorize
func GetOAuthAuthorizeURL(c *gin.Context) {
	provider := c.Param("provider")

	// Map provider ID to CPA's auth URL endpoint
	cpaPath := fmt.Sprintf("/v0/management/%s-auth-url", provider)
	resp, err := cpaRequest(c, http.MethodGet, cpaPath, nil)
	if err != nil {
		common.ApiErrorMsg(c, "CPA unreachable: "+err.Error())
		return
	}
	readCPAResponse(c, resp)
}

// =============================================================================
// OAuth Auth Files (credentials) CRUD
// =============================================================================

// GetOAuthAuthFiles lists all OAuth credentials from CPA.
// GET /api/oauth-provider/auth-files
func GetOAuthAuthFiles(c *gin.Context) {
	resp, err := cpaRequest(c, http.MethodGet, "/v0/management/auth-files", nil)
	if err != nil {
		common.ApiErrorMsg(c, "CPA unreachable: "+err.Error())
		return
	}
	readCPAResponse(c, resp)
}

// DeleteOAuthAuthFile deletes a single OAuth credential.
// DELETE /api/oauth-provider/auth-files/:id
func DeleteOAuthAuthFile(c *gin.Context) {
	authID := c.Param("id")
	cpaPath := fmt.Sprintf("/v0/management/auth-files?id=%s", authID)
	resp, err := cpaRequest(c, http.MethodDelete, cpaPath, nil)
	if err != nil {
		common.ApiErrorMsg(c, "CPA unreachable: "+err.Error())
		return
	}
	readCPAResponse(c, resp)
}

// =============================================================================
// OAuth Status Polling
// =============================================================================

// GetOAuthStatus polls the OAuth flow status from CPA.
// GET /api/oauth-provider/status?state=xxx
func GetOAuthStatus(c *gin.Context) {
	state := c.Query("state")
	cpaPath := fmt.Sprintf("/v0/management/get-auth-status?state=%s", state)
	resp, err := cpaRequest(c, http.MethodGet, cpaPath, nil)
	if err != nil {
		common.ApiErrorMsg(c, "CPA unreachable: "+err.Error())
		return
	}
	readCPAResponse(c, resp)
}

// =============================================================================
// CPA Health Check
// =============================================================================

// GetCPAHealth checks if CPA is reachable.
// GET /api/oauth-provider/health
func GetCPAHealth(c *gin.Context) {
	baseURL := cpaBaseURL()
	resp, err := cpaClient.Get(baseURL + "/v0/management/get-auth-status?state=ping")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"cpa_reachable": false,
				"cpa_base_url":  baseURL,
				"error":         err.Error(),
			},
		})
		return
	}
	resp.Body.Close()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"cpa_reachable": resp.StatusCode < 500,
			"cpa_base_url":  baseURL,
		},
	})
}
