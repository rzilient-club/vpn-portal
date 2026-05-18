package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	if err := loadState(); err != nil {
		log.Fatalf("Failed to load state: %v", err)
	}
	log.Printf("Loaded %d peers from state", len(state.Peers))

	mux := http.NewServeMux()

	// ── User routes ───────────────────────────────────────────────────────────
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/generate", handleGenerate)
	mux.HandleFunc("/download", handleDownload)
	mux.HandleFunc("/qr", handleQR)

	// ── Auth routes ───────────────────────────────────────────────────────────
	mux.HandleFunc("/auth/login", handleLoginStart)
	mux.HandleFunc("/auth/callback", handleLoginCallback)
	mux.HandleFunc("/auth/logout", handleLogout)
	mux.HandleFunc("/auth/magic", handleMagicLinkRequest)
	mux.HandleFunc("/auth/magic/verify", handleMagicLinkVerify)

	// ── Admin routes ──────────────────────────────────────────────────────────
	mux.HandleFunc("/admin/login", handleAdminLogin)
	mux.HandleFunc("/admin/logout", handleAdminLogout)
	mux.HandleFunc("/admin", adminAuth(handleAdmin))
	mux.HandleFunc("/admin/block", adminAuth(handleAdminBlock))
	mux.HandleFunc("/admin/unblock", adminAuth(handleAdminUnblock))
	mux.HandleFunc("/admin/revoke", adminAuth(handleAdminRevoke))
	mux.HandleFunc("/admin/stats", adminAuth(handleAdminStats))
	mux.HandleFunc("/admin/config", adminAuth(handleAdminConfig))
	mux.HandleFunc("/admin/add-peer", adminAuth(handleAdminAddPeer))
	mux.HandleFunc("/admin/version", adminAuth(handleAdminVersion))
	mux.HandleFunc("/admin/update", adminAuth(handleAdminUpdate))

	// ── System routes ─────────────────────────────────────────────────────────
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","service_url":"%s","sha":"%s"}`, baseURL, buildSHA)
	})
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		http.ServeFile(w, r, "manifest.json")
	})
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	addr := ":" + port
	log.Printf("Starting VPN portal on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
