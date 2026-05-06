package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/skip2/go-qrcode"
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
)

var oauthConfig = &oauth2.Config{
	ClientID:     googleClientID,
	ClientSecret: googleClientSecret,
	RedirectURL:  "",
	Scopes:       []string{"openid", "email", "profile"},
	Endpoint:     google.Endpoint,
}

// ─── State ───────────────────────────────────────────────────────────────────

type Peer struct {
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	PublicKey  string    `json:"public_key"`
	PrivateKey string    `json:"private_key"`
	AssignedIP string    `json:"assigned_ip"`
	CreatedAt  time.Time `json:"created_at"`
	Blocked    bool      `json:"blocked"`
}

type State struct {
	Peers []Peer `json:"peers"`
}

var (
	stateMu sync.Mutex
	state   State
)

func loadState() error {
	data, err := os.ReadFile(stateFile)
	if os.IsNotExist(err) {
		state = State{}
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &state)
}

func saveState() error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFile, data, 0600)
}

func findPeerByEmail(email string) *Peer {
	for i := range state.Peers {
		if state.Peers[i].Email == email {
			return &state.Peers[i]
		}
	}
	return nil
}

func nextIP() string {
	used := map[string]bool{"1": true}
	for _, p := range state.Peers {
		parts := strings.Split(p.AssignedIP, ".")
		if len(parts) == 4 {
			used[parts[3]] = true
		}
	}
	for i := 2; i < 254; i++ {
		if !used[fmt.Sprintf("%d", i)] {
			return fmt.Sprintf("%s.%d", vpnSubnet, i)
		}
	}
	return ""
}

// ─── WireGuard ───────────────────────────────────────────────────────────────

func generateKeyPair() (privateKey, publicKey string, err error) {
	if os.Getenv("DEV_MODE") == "true" {
		return "dev-private-key", "dev-public-key", nil
	}
	privOut, err := exec.Command("wg", "genkey").Output()
	if err != nil {
		return "", "", fmt.Errorf("genkey: %w", err)
	}
	privKey := strings.TrimSpace(string(privOut))

	pubCmd := exec.Command("wg", "pubkey")
	pubCmd.Stdin = strings.NewReader(privKey)
	pubOut, err := pubCmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("pubkey: %w", err)
	}
	return privKey, strings.TrimSpace(string(pubOut)), nil
}

func addWGPeer(publicKey, ip string) error {
	if os.Getenv("DEV_MODE") == "true" {
		log.Printf("[DEV] Skipping wg set for peer %s at %s", publicKey, ip)
		return nil
	}
	cmd := exec.Command("wg", "set", wgInterface,
		"peer", publicKey,
		"allowed-ips", ip+"/32",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wg set: %s: %w", out, err)
	}
	saveCmd := exec.Command("wg-quick", "save", wgInterface)
	if out, err := saveCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wg-quick save: %s: %w", out, err)
	}
	return nil
}

func buildConfig(peer *Peer) string {
	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s/32
DNS = 1.1.1.1

[Peer]
PublicKey = %s
Endpoint = %s:51820
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
`, peer.PrivateKey, peer.AssignedIP, serverPublicKey, serverEndpoint)
}

// ─── Sessions ─────────────────────────────────────────────────────────────────

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

// ─── Handlers ─────────────────────────────────────────────────────────────────

func handleHome(w http.ResponseWriter, r *http.Request) {
	email, name, ok := getSession(r)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	stateMu.Lock()
	peer := findPeerByEmail(email)
	var conf, qrData string
	if peer != nil {
		conf = buildConfig(peer)
		qrData = conf
	}
	peerCopy := peer
	stateMu.Unlock()

	data := map[string]interface{}{
		"Name":    name,
		"Email":   email,
		"HasPeer": peerCopy != nil,
		"Config":  conf,
		"QRData":  qrData,
		"IP": func() string {
			if peerCopy != nil {
				return peerCopy.AssignedIP
			}
			return ""
		}(),
	}
	renderTemplate(w, tmplHome, data)
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	email, name, ok := getSession(r)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	stateMu.Lock()
	defer stateMu.Unlock()

	if findPeerByEmail(email) != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	ip := nextIP()
	if ip == "" {
		http.Error(w, "No IPs available", http.StatusInternalServerError)
		return
	}

	privKey, pubKey, err := generateKeyPair()
	if err != nil {
		http.Error(w, "Failed to generate keys: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := addWGPeer(pubKey, ip); err != nil {
		http.Error(w, "Failed to add peer: "+err.Error(), http.StatusInternalServerError)
		return
	}

	peer := Peer{
		Email:      email,
		Name:       name,
		PublicKey:  pubKey,
		PrivateKey: privKey,
		AssignedIP: ip,
		CreatedAt:  time.Now(),
	}
	state.Peers = append(state.Peers, peer)
	saveState()

	http.Redirect(w, r, "/", http.StatusFound)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	email, _, ok := getSession(r)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	stateMu.Lock()
	peer := findPeerByEmail(email)
	var conf string
	if peer != nil {
		conf = buildConfig(peer)
	}
	stateMu.Unlock()

	if peer == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=rzilient-vpn.conf")
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, conf)
}

func handleQR(w http.ResponseWriter, r *http.Request) {
	email, _, ok := getSession(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	stateMu.Lock()
	peer := findPeerByEmail(email)
	var conf string
	if peer != nil {
		conf = buildConfig(peer)
	}
	stateMu.Unlock()

	if peer == nil {
		http.Error(w, "No config found", http.StatusNotFound)
		return
	}

	png, err := qrcode.Encode(conf, qrcode.Medium, 256)
	if err != nil {
		http.Error(w, "QR generation failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

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
	http.Redirect(w, r, "https://rzilient.tech", http.StatusFound)
}

// ─── WireGuard Stats ─────────────────────────────────────────────────────────

type WGStats struct {
	PublicKey     string `json:"public_key"`
	Endpoint      string `json:"endpoint"`
	LastHandshake int64  `json:"last_handshake"`
	RxBytes       int64  `json:"rx_bytes"`
	TxBytes       int64  `json:"tx_bytes"`
}

func getWGStats() (map[string]WGStats, error) {
	if os.Getenv("DEV_MODE") == "true" {
		// Return fake stats in dev mode
		return map[string]WGStats{
			"dev-public-key": {
				PublicKey:     "dev-public-key",
				Endpoint:      "1.2.3.4:51820",
				LastHandshake: time.Now().Add(-30 * time.Second).Unix(),
				RxBytes:       1024 * 1024 * 42,
				TxBytes:       1024 * 1024 * 128,
			},
		}, nil
	}

	out, err := exec.Command("wg", "show", wgInterface, "dump").Output()
	if err != nil {
		return nil, fmt.Errorf("wg show dump: %w", err)
	}

	stats := make(map[string]WGStats)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")

	for i, line := range lines {
		if i == 0 {
			continue // skip interface line
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}

		pubKey := fields[0]
		endpoint := fields[2]
		lastHandshake := parseInt64(fields[4])
		rxBytes := parseInt64(fields[5])
		txBytes := parseInt64(fields[6])

		stats[pubKey] = WGStats{
			PublicKey:     pubKey,
			Endpoint:      endpoint,
			LastHandshake: lastHandshake,
			RxBytes:       rxBytes,
			TxBytes:       txBytes,
		}
	}

	return stats, nil
}

func parseInt64(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// ─── Admin ───────────────────────────────────────────────────────────────────

func adminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if adminToken == "" || token != adminToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	stateMu.Lock()
	peers := make([]Peer, len(state.Peers))
	copy(peers, state.Peers)
	stateMu.Unlock()

	token := r.URL.Query().Get("token")
	renderTemplate(w, tmplAdmin, map[string]interface{}{
		"Peers": peers,
		"Token": token,
	})
}

func handleAdminBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin?token="+r.URL.Query().Get("token"), http.StatusFound)
		return
	}

	pubKey := r.FormValue("public_key")
	token := r.URL.Query().Get("token")

	stateMu.Lock()
	defer stateMu.Unlock()

	for i := range state.Peers {
		if state.Peers[i].PublicKey == pubKey {
			state.Peers[i].Blocked = true
			exec.Command("wg", "set", wgInterface, "peer", pubKey, "remove").Run()
			exec.Command("wg-quick", "save", wgInterface).Run()
			log.Printf("[admin] blocked peer %s (%s)", state.Peers[i].Email, pubKey[:8])
			break
		}
	}
	saveState()
	http.Redirect(w, r, "/admin?token="+token, http.StatusFound)
}

func handleAdminUnblock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin?token="+r.URL.Query().Get("token"), http.StatusFound)
		return
	}

	pubKey := r.FormValue("public_key")
	token := r.URL.Query().Get("token")

	stateMu.Lock()
	defer stateMu.Unlock()

	for i := range state.Peers {
		if state.Peers[i].PublicKey == pubKey {
			state.Peers[i].Blocked = false
			exec.Command("wg", "set", wgInterface,
				"peer", pubKey,
				"allowed-ips", state.Peers[i].AssignedIP+"/32",
			).Run()
			exec.Command("wg-quick", "save", wgInterface).Run()
			log.Printf("[admin] unblocked peer %s (%s)", state.Peers[i].Email, pubKey[:8])
			break
		}
	}
	saveState()
	http.Redirect(w, r, "/admin?token="+token, http.StatusFound)
}

func handleAdminRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin?token="+r.URL.Query().Get("token"), http.StatusFound)
		return
	}

	pubKey := r.FormValue("public_key")
	token := r.URL.Query().Get("token")

	stateMu.Lock()
	defer stateMu.Unlock()

	for i, p := range state.Peers {
		if p.PublicKey == pubKey {
			if os.Getenv("DEV_MODE") != "true" {
				exec.Command("wg", "set", wgInterface, "peer", pubKey, "remove").Run()
				exec.Command("wg-quick", "save", wgInterface).Run()
			}
			state.Peers = append(state.Peers[:i], state.Peers[i+1:]...)
			log.Printf("[admin] revoked peer %s (%s)", p.Email, pubKey[:8])
			break
		}
	}
	saveState()
	http.Redirect(w, r, "/admin?token="+token, http.StatusFound)
}

func handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	pubKey := r.URL.Query().Get("key")
	if pubKey == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}

	stateMu.Lock()
	var peer *Peer
	for i := range state.Peers {
		if state.Peers[i].PublicKey == pubKey {
			peer = &state.Peers[i]
			break
		}
	}
	stateMu.Unlock()

	if peer == nil {
		http.Error(w, "peer not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, buildConfig(peer))
}

func handleAdminStats(w http.ResponseWriter, r *http.Request) {
	stats, err := getWGStats()
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// Enrich with formatted values
	type StatResponse struct {
		PublicKey     string `json:"public_key"`
		Endpoint      string `json:"endpoint"`
		LastHandshake int64  `json:"last_handshake"`
		RxBytes       int64  `json:"rx_bytes"`
		TxBytes       int64  `json:"tx_bytes"`
		RxFormatted   string `json:"rx_formatted"`
		TxFormatted   string `json:"tx_formatted"`
		Online        bool   `json:"online"`
	}

	result := make(map[string]StatResponse)
	now := time.Now().Unix()

	for k, s := range stats {
		online := s.LastHandshake > 0 && (now-s.LastHandshake) < 180 // online if handshake < 3 min ago
		result[k] = StatResponse{
			PublicKey:     s.PublicKey,
			Endpoint:      s.Endpoint,
			LastHandshake: s.LastHandshake,
			RxBytes:       s.RxBytes,
			TxBytes:       s.TxBytes,
			RxFormatted:   formatBytes(s.RxBytes),
			TxFormatted:   formatBytes(s.TxBytes),
			Online:        online,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ─── Templates ────────────────────────────────────────────────────────────────

var tmplHome *template.Template
var tmplUnauthorized *template.Template
var tmplAdmin *template.Template

func init() {
	godotenv.Load()
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
	}

	tmplHome = template.Must(template.ParseFiles("templates/home.html"))
	tmplUnauthorized = template.Must(template.ParseFiles("templates/unauthorized.html"))
	tmplAdmin = template.Must(template.New("admin.html").Funcs(funcMap).ParseFiles("templates/admin.html"))

	oauthConfig.ClientID = googleClientID
	oauthConfig.ClientSecret = googleClientSecret
}

func renderTemplate(w http.ResponseWriter, tmpl *template.Template, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, tmpl.Name(), data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func isDomainAllowed(domain string) bool {
	for _, d := range allowedDomains {
		if strings.TrimSpace(d) == domain {
			return true
		}
	}
	return false
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	if err := loadState(); err != nil {
		log.Fatalf("Failed to load state: %v", err)
	}
	log.Printf("Loaded %d peers from state", len(state.Peers))

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/generate", handleGenerate)
	mux.HandleFunc("/download", handleDownload)
	mux.HandleFunc("/qr", handleQR)
	mux.HandleFunc("/auth/login", handleLoginStart)
	mux.HandleFunc("/auth/callback", handleLoginCallback)
	mux.HandleFunc("/auth/logout", handleLogout)
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		http.ServeFile(w, r, "manifest.json")
	})
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("/admin", adminAuth(handleAdmin))
	mux.HandleFunc("/admin/block", adminAuth(handleAdminBlock))
	mux.HandleFunc("/admin/unblock", adminAuth(handleAdminUnblock))
	mux.HandleFunc("/admin/revoke", adminAuth(handleAdminRevoke))
	mux.HandleFunc("/admin/stats", adminAuth(handleAdminStats))
	mux.HandleFunc("/admin/config", adminAuth(handleAdminConfig))

	addr := ":" + port
	log.Printf("Starting VPN portal on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
