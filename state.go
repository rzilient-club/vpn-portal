package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

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
