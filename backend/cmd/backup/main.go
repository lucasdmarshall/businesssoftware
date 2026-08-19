package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type manifest struct {
	File       string `json:"file"`
	SHA256     string `json:"sha256"`
	VerifiedAt string `json:"verified_at"`
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fail("DATABASE_URL is required")
	}
	backupDir := os.Getenv("BACKUP_PATH")
	if backupDir == "" {
		backupDir = "data/backups"
	}
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		fail(err.Error())
	}
	name := fmt.Sprintf("name-%s.dump", time.Now().UTC().Format("20060102-150405"))
	path := filepath.Join(backupDir, name)
	if err := exec.Command("pg_dump", "--format=custom", "--no-owner", "--file", path, databaseURL).Run(); err != nil {
		_ = os.Remove(path)
		fail(fmt.Sprintf("pg_dump failed: %v", err))
	}
	if err := exec.Command("pg_restore", "--list", path).Run(); err != nil {
		_ = os.Remove(path)
		fail(fmt.Sprintf("backup verification failed: %v", err))
	}
	file, err := os.Open(path)
	if err != nil {
		fail(err.Error())
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		fail(err.Error())
	}
	m := manifest{File: name, SHA256: hex.EncodeToString(hash.Sum(nil)), VerifiedAt: time.Now().UTC().Format(time.RFC3339)}
	data, _ := json.MarshalIndent(m, "", "  ")
	if err = os.WriteFile(path+".manifest.json", data, 0600); err != nil {
		fail(err.Error())
	}
	fmt.Printf("verified backup: %s\nsha256: %s\n", path, m.SHA256)
}

func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
