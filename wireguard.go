package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

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
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		stats[fields[0]] = WGStats{
			PublicKey:     fields[0],
			Endpoint:      fields[2],
			LastHandshake: parseInt64(fields[4]),
			RxBytes:       parseInt64(fields[5]),
			TxBytes:       parseInt64(fields[6]),
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
