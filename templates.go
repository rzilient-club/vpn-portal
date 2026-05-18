package main

import (
	"html/template"
	"log"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// ─── Templates ────────────────────────────────────────────────────────────────

var (
	tmplHome         *template.Template
	tmplUnauthorized *template.Template
	tmplAdmin        *template.Template
	tmplAdminLogin   *template.Template
	tmplMagicLink    *template.Template
	tmplMagicSent    *template.Template
)

func init() {
	godotenv.Load()

	// Reload all config from env
	googleClientID = getEnv("GOOGLE_CLIENT_ID", "")
	googleClientSecret = getEnv("GOOGLE_CLIENT_SECRET", "")
	allowedDomains = strings.Split(getEnv("ALLOWED_DOMAINS", "rzilient.club,rzilient.tech"), ",")
	baseURL = getEnv("BASE_URL", "https://vpn.rzilient.tech")
	stateFile = getEnv("STATE_FILE", "/etc/wireguard/peers/state.json")
	wgInterface = getEnv("WG_INTERFACE", "wg0")
	serverPublicKey = getEnv("WG_SERVER_PUBLIC_KEY", "")
	serverEndpoint = getEnv("WG_SERVER_ENDPOINT", "")
	vpnSubnet = getEnv("VPN_SUBNET", "10.8.0")
	sessionSecret = getEnv("SESSION_SECRET", "change-me-in-production")
	port = getEnv("PORT", "8080")
	adminToken = getEnv("ADMIN_TOKEN", "")
	smtpHost = getEnv("SMTP_HOST", "smtp.eu.mailgun.org")
	smtpPort = getEnv("SMTP_PORT", "587")
	smtpUsername = getEnv("SMTP_USERNAME", "")
	smtpPassword = getEnv("SMTP_PASSWORD", "")
	smtpFrom = getEnv("SMTP_FROM", "")

	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"slice": func(s string, i, j int) string {
			if i > len(s) {
				return s
			}
			if j > len(s) {
				j = len(s)
			}
			return s[i:j]
		},
		"formatDate": func(t time.Time) string {
			return t.Format("2006-01-02")
		},
	}

	tmplHome = template.Must(template.ParseFiles("templates/home.html"))
	tmplUnauthorized = template.Must(template.ParseFiles("templates/unauthorized.html"))
	tmplAdmin = template.Must(template.New("admin.html").Funcs(funcMap).ParseFiles("templates/admin.html"))
	tmplAdminLogin = template.Must(template.ParseFiles("templates/admin-login.html"))
	tmplMagicLink = template.Must(template.ParseFiles("templates/magic-link.html"))
	tmplMagicSent = template.Must(template.ParseFiles("templates/magic-sent.html"))

	oauthConfig.ClientID = googleClientID
	oauthConfig.ClientSecret = googleClientSecret
}

func renderTemplate(w http.ResponseWriter, tmpl *template.Template, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, tmpl.Name(), data); err != nil {
		log.Printf("template error: %v", err)
	}
}
