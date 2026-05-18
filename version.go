package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// ─── Version & Update ────────────────────────────────────────────────────────

// buildSHA is set at build time via -ldflags "-X main.buildSHA=..."
var buildSHA = "unknown"

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

	// Run vpn-update in a sibling container via Docker socket
	// Independent of vpn-portal container lifecycle
	cmd := exec.Command("docker", "run", "--rm",
		"--name", "vpn-updater",
		"-v", "/usr/local/bin/vpn-update:/vpn-update:ro",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"-v", "/etc/vpn-portal.env:/etc/vpn-portal.env:ro",
		"-v", "/usr/local/bin/doctl:/usr/local/bin/doctl:ro",
		"-v", "/root/.docker:/root/.docker",
		"-v", "/root/.config/doctl:/root/.config/doctl:ro",
		"-v", "/etc/wireguard:/etc/wireguard",
		"-v", "/var/log:/var/log",
		"alpine",
		"sh", "/vpn-update",
	)
	cmd.Env = append(os.Environ(), "PATH=/usr/local/bin:/usr/bin:/bin:/sbin")
	if err := cmd.Start(); err != nil {
		log.Printf("[update] failed to start: %v", err)
	} else {
		log.Printf("[update] sibling container launched pid %d", cmd.Process.Pid)
		cmd.Process.Release()
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"update started"}`)
}
