package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

// ─── Magic link store ─────────────────────────────────────────────────────────

type magicLinkEntry struct {
	email  string
	expiry time.Time
}

var (
	magicLinkMu    sync.Mutex
	magicLinkStore = map[string]magicLinkEntry{}
)

func newMagicToken(email string) string {
	b := make([]byte, 32)
	rand.Read(b)
	token := base64.URLEncoding.EncodeToString(b)
	magicLinkMu.Lock()
	magicLinkStore[token] = magicLinkEntry{email: email, expiry: time.Now().Add(1 * time.Hour)}
	magicLinkMu.Unlock()
	return token
}

func consumeMagicToken(token string) (string, bool) {
	magicLinkMu.Lock()
	defer magicLinkMu.Unlock()
	entry, ok := magicLinkStore[token]
	if !ok || time.Now().After(entry.expiry) {
		delete(magicLinkStore, token)
		return "", false
	}
	delete(magicLinkStore, token)
	return entry.email, true
}

// ─── SMTP ─────────────────────────────────────────────────────────────────────

func sendMagicLinkEmail(toEmail, magicURL string) error {
	if smtpUsername == "" || smtpPassword == "" {
		return fmt.Errorf("smtp not configured")
	}

	fromAddr := smtpUsername
	fromHeader := smtpFrom
	if fromHeader == "" {
		fromHeader = smtpUsername
	}

	subject := "Your VPN login link"
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n"+
			`<p>Click the link below to log in to the VPN portal. This link expires in <strong>1 hour</strong>.</p>`+
			`<p><a href="%s" style="background:#B4EA1F;color:#0a0a0a;padding:12px 24px;border-radius:8px;text-decoration:none;font-weight:bold;">Log in to VPN portal</a></p>`+
			`<p style="color:#888;font-size:12px;">If you did not request this, ignore this email.</p>`,
		fromHeader, toEmail, subject, magicURL,
	)

	auth := smtp.PlainAuth("", smtpUsername, smtpPassword, smtpHost)
	return smtp.SendMail(smtpHost+":"+smtpPort, auth, fromAddr, []string{toEmail}, []byte(msg))
}

// ─── Handlers ────────────────────────────────────────────────────────────────

func handleMagicLinkRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			renderTemplate(w, tmplMagicLink, map[string]interface{}{"Error": "invalid_request"})
			return
		}
		email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
		if email == "" {
			renderTemplate(w, tmplMagicLink, map[string]interface{}{"Error": "missing_email"})
			return
		}
		parts := strings.Split(email, "@")
		if len(parts) != 2 || !isDomainAllowed(parts[1]) {
			renderTemplate(w, tmplMagicLink, map[string]interface{}{"Error": "domain_not_allowed"})
			return
		}
		token := newMagicToken(email)
		magicURL := fmt.Sprintf("%s/auth/magic/verify?token=%s", baseURL, token)
		if err := sendMagicLinkEmail(email, magicURL); err != nil {
			log.Printf("[magic] failed to send email to %s: %v", email, err)
			renderTemplate(w, tmplMagicLink, map[string]interface{}{"Error": "send_failed"})
			return
		}
		log.Printf("[magic] sent link to %s", email)
		renderTemplate(w, tmplMagicSent, map[string]interface{}{"Email": email})
		return
	}
	renderTemplate(w, tmplMagicLink, map[string]interface{}{"Error": r.URL.Query().Get("error")})
}

func handleMagicLinkVerify(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, "/auth/magic?error=missing_token", http.StatusFound)
		return
	}
	email, ok := consumeMagicToken(token)
	if !ok {
		http.Redirect(w, r, "/auth/magic?error=invalid_or_expired", http.StatusFound)
		return
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 || !isDomainAllowed(parts[1]) {
		http.Redirect(w, r, "/auth/magic?error=domain_not_allowed", http.StatusFound)
		return
	}
	nameParts := strings.Split(parts[0], ".")
	for i, p := range nameParts {
		if len(p) > 0 {
			nameParts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	setSession(w, email, strings.Join(nameParts, " "))
	log.Printf("[magic] verified login for %s", email)
	http.Redirect(w, r, "/", http.StatusFound)
}
