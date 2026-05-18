package main

import (
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// ─── Config ──────────────────────────────────────────────────────────────────

var (
	googleClientID     = getEnv("GOOGLE_CLIENT_ID", "")
	googleClientSecret = getEnv("GOOGLE_CLIENT_SECRET", "")
	allowedDomains     = strings.Split(getEnv("ALLOWED_DOMAINS", "rzilient.club,rzilient.tech"), ",")
	baseURL            = getEnv("BASE_URL", "https://vpn.rzilient.tech")
	stateFile          = getEnv("STATE_FILE", "/etc/wireguard/peers/state.json")
	wgInterface        = getEnv("WG_INTERFACE", "wg0")
	serverPublicKey    = getEnv("WG_SERVER_PUBLIC_KEY", "")
	serverEndpoint     = getEnv("WG_SERVER_ENDPOINT", "")
	vpnSubnet          = getEnv("VPN_SUBNET", "10.8.0")
	sessionSecret      = getEnv("SESSION_SECRET", "change-me-in-production")
	port               = getEnv("PORT", "8080")
	adminToken         = getEnv("ADMIN_TOKEN", "")
	smtpHost           = getEnv("SMTP_HOST", "smtp.eu.mailgun.org")
	smtpPort           = getEnv("SMTP_PORT", "587")
	smtpUsername       = getEnv("SMTP_USERNAME", "")
	smtpPassword       = getEnv("SMTP_PASSWORD", "")
	smtpFrom           = getEnv("SMTP_FROM", "")
)

var oauthConfig = &oauth2.Config{
	ClientID:     googleClientID,
	ClientSecret: googleClientSecret,
	RedirectURL:  "",
	Scopes:       []string{"openid", "email", "profile"},
	Endpoint:     google.Endpoint,
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func isDomainAllowed(domain string) bool {
	for _, d := range allowedDomains {
		if strings.TrimSpace(d) == domain {
			return true
		}
	}
	return false
}
