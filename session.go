package main

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ─── User session ─────────────────────────────────────────────────────────────

func setSession(w http.ResponseWriter, email, name string) {
	val := base64.StdEncoding.EncodeToString([]byte(email + "|" + name))
	http.SetCookie(w, &http.Cookie{
		Name:     "vpn_session",
		Value:    val,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   86400 * 7,
		SameSite: http.SameSiteLaxMode,
	})
}

func getSession(r *http.Request) (email, name string, ok bool) {
	c, err := r.Cookie("vpn_session")
	if err != nil {
		return "", "", false
	}
	b, err := base64.StdEncoding.DecodeString(c.Value)
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   "vpn_session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

// ─── Admin session ────────────────────────────────────────────────────────────

var (
	adminSessionMu    sync.Mutex
	adminSessionStore = map[string]time.Time{}
)

func setAdminSession(w http.ResponseWriter) {
	b := make([]byte, 32)
	rand.Read(b)
	val := base64.StdEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name:     "vpn_admin_session",
		Value:    val,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   86400 * 1,
		SameSite: http.SameSiteLaxMode,
	})
	adminSessionMu.Lock()
	adminSessionStore[val] = time.Now().Add(24 * time.Hour)
	adminSessionMu.Unlock()
}

func getAdminSession(r *http.Request) bool {
	c, err := r.Cookie("vpn_admin_session")
	if err != nil {
		return false
	}
	adminSessionMu.Lock()
	defer adminSessionMu.Unlock()
	exp, ok := adminSessionStore[c.Value]
	if !ok || time.Now().After(exp) {
		delete(adminSessionStore, c.Value)
		return false
	}
	return true
}

func clearAdminSession(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("vpn_admin_session")
	if err == nil {
		adminSessionMu.Lock()
		delete(adminSessionStore, c.Value)
		adminSessionMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "vpn_admin_session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}
