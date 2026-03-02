package operator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// oauthStateStore holds pending OAuth state tokens mapped to account names.
// States expire after 10 minutes.
type oauthStateStore struct {
	mu     sync.Mutex
	states map[string]oauthPending
}

type oauthPending struct {
	Account   string
	ExpiresAt time.Time
}

var oauthStates = &oauthStateStore{states: make(map[string]oauthPending)}

func (s *oauthStateStore) Set(state, account string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = oauthPending{Account: account, ExpiresAt: time.Now().Add(10 * time.Minute)}
}

func (s *oauthStateStore) Pop(state string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.states[state]
	if !ok {
		return "", false
	}
	delete(s.states, state)
	if time.Now().After(p.ExpiresAt) {
		return "", false
	}
	return p.Account, true
}

// Well-known OAuth provider endpoints.
var oauthProviders = map[string]struct{ AuthURL, TokenURL string }{
	"google": {
		AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
	},
	"github": {
		AuthURL:  "https://github.com/login/oauth/authorize",
		TokenURL: "https://github.com/login/oauth/access_token",
	},
}

// handleOAuthStart initiates an OAuth authorization flow for a connected account.
// GET /oauth/start/{account}
func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	acctName := r.PathValue("account")
	if acctName == "" {
		jsonErr(w, 400, "account name is required")
		return
	}

	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		jsonErr(w, 500, "loading config: "+err.Error())
		return
	}

	acct, ok := cfg.ConnectedAccounts[acctName]
	if !ok {
		jsonErr(w, 404, fmt.Sprintf("connected account %q not found", acctName))
		return
	}
	if acct.Type != "oauth" || acct.OAuth == nil {
		jsonErr(w, 400, fmt.Sprintf("connected account %q is not an oauth account", acctName))
		return
	}

	// Resolve auth URL
	authURL := acct.OAuth.AuthURL
	if authURL == "" {
		if provider, ok := oauthProviders[acct.Provider]; ok {
			authURL = provider.AuthURL
		} else {
			jsonErr(w, 400, fmt.Sprintf("no auth_url configured for provider %q", acct.Provider))
			return
		}
	}

	// Resolve client_id (may contain ${secret:...} references)
	clientID := acct.OAuth.ClientID
	if strings.HasPrefix(clientID, "${secret:") && s.Credentials != nil {
		secretName := strings.TrimSuffix(strings.TrimPrefix(clientID, "${secret:"), "}")
		if val, err := s.Credentials.Get(r.Context(), secretName); err == nil {
			clientID = val
		}
	}
	if clientID == "" {
		jsonErr(w, 400, "oauth client_id is required")
		return
	}

	redirectURL := acct.OAuth.RedirectURL
	if redirectURL == "" {
		redirectURL = "http://localhost:9000/oauth/callback"
	}

	// Generate state token
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		jsonErr(w, 500, "generating state: "+err.Error())
		return
	}
	state := hex.EncodeToString(stateBytes)
	oauthStates.Set(state, acctName)

	// Build authorization URL
	params := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURL},
		"response_type": {"code"},
		"state":         {state},
	}
	if len(acct.OAuth.Scopes) > 0 {
		params.Set("scope", strings.Join(acct.OAuth.Scopes, " "))
	}
	// Google requires access_type=offline for refresh tokens
	if acct.Provider == "google" {
		params.Set("access_type", "offline")
		params.Set("prompt", "consent")
	}

	http.Redirect(w, r, authURL+"?"+params.Encode(), http.StatusFound)
}

// handleOAuthCallback handles the OAuth provider redirect with an authorization code.
// GET /oauth/callback?code=...&state=...
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		// Check for error response from provider
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			desc := r.URL.Query().Get("error_description")
			http.Error(w, fmt.Sprintf("OAuth error: %s — %s", errMsg, desc), http.StatusBadRequest)
			return
		}
		http.Error(w, "Missing code or state parameter", http.StatusBadRequest)
		return
	}

	acctName, ok := oauthStates.Pop(state)
	if !ok {
		http.Error(w, "Invalid or expired OAuth state", http.StatusBadRequest)
		return
	}

	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		http.Error(w, "Loading config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	acct, ok := cfg.ConnectedAccounts[acctName]
	if !ok {
		http.Error(w, "Account not found in config", http.StatusInternalServerError)
		return
	}

	// Exchange code for tokens
	tokenURL := acct.OAuth.TokenURL
	if tokenURL == "" {
		if provider, ok := oauthProviders[acct.Provider]; ok {
			tokenURL = provider.TokenURL
		} else {
			http.Error(w, "No token_url for provider", http.StatusBadRequest)
			return
		}
	}

	clientID := resolveSecret(r.Context(), acct.OAuth.ClientID, s)
	clientSecret := resolveSecret(r.Context(), acct.OAuth.ClientSecret, s)

	redirectURL := acct.OAuth.RedirectURL
	if redirectURL == "" {
		redirectURL = "http://localhost:9000/oauth/callback"
	}

	// Exchange authorization code for access token
	accessToken, err := exchangeOAuthCode(r.Context(), tokenURL, clientID, clientSecret, code, redirectURL)
	if err != nil {
		s.Log.Error("oauth token exchange failed", "account", acctName, "error", err)
		http.Error(w, "Token exchange failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Store the access token as a credential
	if s.Credentials == nil {
		http.Error(w, "Credentials backend not configured", http.StatusInternalServerError)
		return
	}

	secretName := "account-" + acctName
	if err := s.Credentials.Set(r.Context(), secretName, accessToken); err != nil {
		http.Error(w, "Storing token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.Log.Info("oauth account connected", "account", acctName, "provider", acct.Provider)

	// Return a simple success page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>Connected</title></head>
<body style="font-family: system-ui; text-align: center; padding: 60px;">
<h2>%s connected successfully</h2>
<p>You can close this window and return to the terminal.</p>
<p>Run <code>angee deploy</code> to apply the new credentials.</p>
</body></html>`, acctName)
}

// handleOAuthStatus checks if a connected account has valid credentials stored.
// GET /oauth/status/{account}
func (s *Server) handleOAuthStatus(w http.ResponseWriter, r *http.Request) {
	acctName := r.PathValue("account")
	if acctName == "" {
		jsonErr(w, 400, "account name is required")
		return
	}

	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		jsonErr(w, 500, "loading config: "+err.Error())
		return
	}

	acct, ok := cfg.ConnectedAccounts[acctName]
	if !ok {
		jsonErr(w, 404, fmt.Sprintf("connected account %q not found", acctName))
		return
	}

	connected := false
	if s.Credentials != nil {
		secretName := "account-" + acctName
		if _, err := s.Credentials.Get(r.Context(), secretName); err == nil {
			connected = true
		}
	}

	jsonOK(w, map[string]any{
		"account":   acctName,
		"provider":  acct.Provider,
		"type":      acct.Type,
		"connected": connected,
	})
}

// handleConnectedAccountsList returns all connected accounts and their connection status.
// GET /connected-accounts
func (s *Server) handleConnectedAccountsList(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		jsonErr(w, 500, "loading config: "+err.Error())
		return
	}

	type accountInfo struct {
		Name        string `json:"name"`
		Provider    string `json:"provider"`
		Type        string `json:"type"`
		Description string `json:"description,omitempty"`
		Required    bool   `json:"required"`
		Connected   bool   `json:"connected"`
	}

	var accounts []accountInfo
	for name, acct := range cfg.ConnectedAccounts {
		connected := false
		if s.Credentials != nil {
			secretName := "account-" + name
			if _, err := s.Credentials.Get(r.Context(), secretName); err == nil {
				connected = true
			}
		}
		accounts = append(accounts, accountInfo{
			Name:        name,
			Provider:    acct.Provider,
			Type:        acct.Type,
			Description: acct.Description,
			Required:    acct.Required,
			Connected:   connected,
		})
	}

	if accounts == nil {
		accounts = []accountInfo{}
	}

	jsonOK(w, accounts)
}

// resolveSecret dereferences a ${secret:name} value using the credentials backend.
func resolveSecret(ctx context.Context, value string, s *Server) string {
	if !strings.HasPrefix(value, "${secret:") || s.Credentials == nil {
		return value
	}
	secretName := strings.TrimSuffix(strings.TrimPrefix(value, "${secret:"), "}")
	if val, err := s.Credentials.Get(ctx, secretName); err == nil {
		return val
	}
	return value
}

// exchangeOAuthCode exchanges an authorization code for an access token.
func exchangeOAuthCode(ctx context.Context, tokenURL, clientID, clientSecret, code, redirectURL string) (string, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirectURL},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading token response: %w", err)
	}

	// Parse response — handle both JSON and form-encoded (GitHub uses form-encoded)
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/x-www-form-urlencoded") || strings.Contains(contentType, "text/plain") {
		vals, parseErr := url.ParseQuery(string(body))
		if parseErr != nil {
			return "", fmt.Errorf("parsing token response: %w", parseErr)
		}
		token := vals.Get("access_token")
		if token == "" {
			return "", fmt.Errorf("no access_token in response")
		}
		return token, nil
	}

	// JSON response
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}

	if jsonErr := json.Unmarshal(body, &tokenResp); jsonErr != nil {
		return "", fmt.Errorf("parsing token response: %w", jsonErr)
	}
	if tokenResp.Error != "" {
		return "", fmt.Errorf("token error: %s — %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("no access_token in response")
	}
	return tokenResp.AccessToken, nil
}
