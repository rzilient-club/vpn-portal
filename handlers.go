package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/skip2/go-qrcode"
)

// ─── User handlers ────────────────────────────────────────────────────────────

func handleHome(w http.ResponseWriter, r *http.Request) {
	email, name, ok := getSession(r)
	if !ok {
		http.Redirect(w, r, "/auth/magic", http.StatusFound)
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

	ip := ""
	if peerCopy != nil {
		ip = peerCopy.AssignedIP
	}

	renderTemplate(w, tmplHome, map[string]interface{}{
		"Name":    name,
		"Email":   email,
		"HasPeer": peerCopy != nil,
		"Config":  conf,
		"QRData":  qrData,
		"IP":      ip,
	})
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	email, name, ok := getSession(r)
	if !ok {
		http.Redirect(w, r, "/auth/magic", http.StatusFound)
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

	state.Peers = append(state.Peers, Peer{
		Email:      email,
		Name:       name,
		PublicKey:  pubKey,
		PrivateKey: privKey,
		AssignedIP: ip,
		CreatedAt:  time.Now(),
	})
	saveState()
	http.Redirect(w, r, "/", http.StatusFound)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	email, _, ok := getSession(r)
	if !ok {
		http.Redirect(w, r, "/auth/magic", http.StatusFound)
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
