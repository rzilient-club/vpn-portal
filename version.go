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

	registry := getEnv("REGISTRY", "registry.digitalocean.com/rzilient-do-containers")
	image := registry + "/vpn-portal:latest"

	// Pull latest to update local cache
	exec.Command("docker", "pull", image).Run()

	// Get image ID of running container
	currentID := "unknown"
	out, err := exec.Command("docker", "inspect",
		"--format", "{{.Image}}", "vpn-portal").Output()
	if err == nil {
		currentID = strings.TrimSpace(string(out))
	}

	// Get image ID of local :latest tag
	latestID := "unknown"
	out2, err := exec.Command("docker", "inspect",
		"--format", "{{.Id}}", image).Output()
	if err == nil {
		latestID = strings.TrimSpace(string(out2))
	}

	updateAvailable := currentID != "unknown" &&
		latestID != "unknown" &&
		currentID != latestID

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"current_sha":      buildSHA,
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

	// Run completely detached from this process so it survives container death
	cmd := exec.Command("nsenter", "-t", "1", "-m", "-u", "-i", "-n", "-p",
		"--", "/bin/bash", "-c",
		"nohup /usr/local/bin/vpn-update >> /var/log/vpn-update.log 2>&1 &")
	cmd.Env = append(os.Environ(), "PATH=/usr/local/bin:/usr/bin:/bin:/sbin")
	if err := cmd.Start(); err != nil {
		log.Printf("[update] failed to start: %v", err)
		updateMu.Lock()
		updateRunning = false
		updateMu.Unlock()
	} else {
		log.Printf("[update] script launched via nsenter pid %d", cmd.Process.Pid)
		cmd.Process.Release()
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"update started"}`)
}
