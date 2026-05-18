package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ─── Admin auth ───────────────────────────────────────────────────────────────

func adminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !getAdminSession(r) {
			http.Redirect(w, r, "/admin/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

// ─── Admin handlers ───────────────────────────────────────────────────────────

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	stateMu.Lock()
	peers := make([]Peer, len(state.Peers))
	copy(peers, state.Peers)
	stateMu.Unlock()

	renderTemplate(w, tmplAdmin, map[string]interface{}{
		"Peers": peers,
		"Error": r.URL.Query().Get("error"),
	})
}

func handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if getAdminSession(r) {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			renderTemplate(w, tmplAdminLogin, map[string]interface{}{"Error": "invalid_request"})
			return
		}
		token := r.FormValue("token")
		if adminToken == "" || token != adminToken {
			renderTemplate(w, tmplAdminLogin, map[string]interface{}{"Error": "invalid_token"})
			return
		}
		setAdminSession(w)
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	renderTemplate(w, tmplAdminLogin, nil)
}

func handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	clearAdminSession(w, r)
	http.Redirect(w, r, "/admin/login", http.StatusFound)
}

func handleAdminBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	pubKey := r.FormValue("public_key")
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
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func handleAdminUnblock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	pubKey := r.FormValue("public_key")
	stateMu.Lock()
	defer stateMu.Unlock()
	for i := range state.Peers {
		if state.Peers[i].PublicKey == pubKey {
			state.Peers[i].Blocked = false
			exec.Command("wg", "set", wgInterface, "peer", pubKey, "allowed-ips", state.Peers[i].AssignedIP+"/32").Run()
			exec.Command("wg-quick", "save", wgInterface).Run()
			log.Printf("[admin] unblocked peer %s (%s)", state.Peers[i].Email, pubKey[:8])
			break
		}
	}
	saveState()
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func handleAdminRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	pubKey := r.FormValue("public_key")
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
	http.Redirect(w, r, "/admin", http.StatusFound)
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

func handleAdminAddPeer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	name := strings.TrimSpace(r.FormValue("name"))
	if email == "" || name == "" {
		http.Redirect(w, r, "/admin?error=missing_fields", http.StatusFound)
		return
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	if findPeerByEmail(email) != nil {
		http.Redirect(w, r, "/admin?error=already_exists", http.StatusFound)
		return
	}
	ip := nextIP()
	if ip == "" {
		http.Redirect(w, r, "/admin?error=no_ips", http.StatusFound)
		return
	}
	privKey, pubKey, err := generateKeyPair()
	if err != nil {
		http.Redirect(w, r, "/admin?error=keygen_failed", http.StatusFound)
		return
	}
	if err := addWGPeer(pubKey, ip); err != nil {
		http.Redirect(w, r, "/admin?error=wg_failed", http.StatusFound)
		return
	}
	state.Peers = append(state.Peers, Peer{
		Email:      email,
		Name:       name,
		PublicKey:  pubKey,
		PrivateKey: privKey,
		AssignedIP: ip,
		CreatedAt:  time.Now(),
	})
	saveState()
	log.Printf("[admin] added peer %s (%s)", email, pubKey[:8])
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func handleAdminStats(w http.ResponseWriter, r *http.Request) {
	stats, err := getWGStats()
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

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
		result[k] = StatResponse{
			PublicKey:     s.PublicKey,
			Endpoint:      s.Endpoint,
			LastHandshake: s.LastHandshake,
			RxBytes:       s.RxBytes,
			TxBytes:       s.TxBytes,
			RxFormatted:   formatBytes(s.RxBytes),
			TxFormatted:   formatBytes(s.TxBytes),
			Online:        s.LastHandshake > 0 && (now-s.LastHandshake) < 180,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
