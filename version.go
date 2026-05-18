package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// ─── Version & Update ────────────────────────────────────────────────────────

// buildSHA is set at build time via -ldflags "-X main.buildSHA=..."
var buildSHA = "unknown"

var (
	updateRunning bool
	updateMu      sync.Mutex
)

func getLatestImageSHA() string {
	registry := getEnv("REGISTRY", "registry.digitalocean.com/rzilient-do-containers")
	out, err := exec.Command("docker", "manifest", "inspect", "--verbose",
		registry+"/vpn-portal:latest").Output()
	if err != nil {
		return "unknown"
	}
	str := string(out)
	if idx := strings.Index(str, `"digest": "sha256:`); idx != -1 {
		start := idx + 18
		end := start + 7
		if end > len(str) {
			end = len(str)
		}
		return str[start:end]
	}
	return "unknown"
}

func handleAdminVersion(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("DEV_MODE") == "true" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"current_sha":      "dev",
			"update_available": false,
		})
		return
	}

	currentDigest := "unknown"
	out, err := exec.Command("docker", "inspect",
		"--format", "{{index .RepoDigests 0}}", "vpn-portal").Output()
	if err == nil {
		d := strings.TrimSpace(string(out))
		if idx := strings.Index(d, "sha256:"); idx != -1 {
			hash := d[idx+7:]
			if len(hash) > 7 {
				hash = hash[:7]
			}
			currentDigest = hash
		}
	}

	latestDigest := getLatestImageSHA()
	updateAvailable := currentDigest != "unknown" &&
		latestDigest != "unknown" &&
		currentDigest != latestDigest

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"current_sha":      buildSHA,
		"current_digest":   currentDigest,
		"latest_digest":    latestDigest,
		"update_available": updateAvailable,
	})
}

func handleAdminUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if os.Getenv("DEV_MODE") == "true" {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"update started","dev_mode":true}`)
		return
	}
	updateMu.Lock()
	if updateRunning {
		updateMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"status":"update already in progress"}`)
		return
	}
	updateRunning = true
	updateMu.Unlock()

	go func() {
		defer func() {
			updateMu.Lock()
			updateRunning = false
			updateMu.Unlock()
		}()
		out, err := exec.Command("/usr/local/bin/vpn-update").CombinedOutput()
		if err != nil {
			log.Printf("[update] failed: %s: %v", out, err)
		} else {
			log.Printf("[update] completed: %s", out)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"update started"}`)
}
