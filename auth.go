package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// ─── OAuth state ──────────────────────────────────────────────────────────────

var (
	oauthStateMu    sync.Mutex
	oauthStateStore = map[string]time.Time{}
)

func newOAuthState() string {
	b := make([]byte, 16)
	rand.Read(b)
	s := base64.URLEncoding.EncodeToString(b)
	oauthStateMu.Lock()
	oauthStateStore[s] = time.Now().Add(5 * time.Minute)
	oauthStateMu.Unlock()
	return s
}

func validOAuthState(s string) bool {
	oauthStateMu.Lock()
	defer oauthStateMu.Unlock()
	exp, ok := oauthStateStore[s]
	if !ok {
		return false
	}
	delete(oauthStateStore, s)
	return time.Now().Before(exp)
}

// ─── Handlers ────────────────────────────────────────────────────────────────

func handleLoginStart(w http.ResponseWriter, r *http.Request) {
	oauthConfig.RedirectURL = baseURL + "/auth/callback"
	state := newOAuthState()
	url := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOnline)
	http.Redirect(w, r, url, http.StatusFound)
}

func handleLoginCallback(w http.ResponseWriter, r *http.Request) {
	if !validOAuthState(r.URL.Query().Get("state")) {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	oauthConfig.RedirectURL = baseURL + "/auth/callback"
	token, err := oauthConfig.Exchange(context.Background(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "Token exchange failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	client := oauthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var userInfo struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		http.Error(w, "Failed to parse user info", http.StatusInternalServerError)
		return
	}

	parts := strings.Split(userInfo.Email, "@")
	if len(parts) != 2 || !isDomainAllowed(parts[1]) {
		renderTemplate(w, tmplUnauthorized, map[string]interface{}{
			"Email":   userInfo.Email,
			"Domains": strings.Join(allowedDomains, ", "),
		})
		return
	}

	setSession(w, userInfo.Email, userInfo.Name)
	http.Redirect(w, r, "/", http.StatusFound)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSession(w)
	http.Redirect(w, r, "/auth/magic", http.StatusFound)
}
