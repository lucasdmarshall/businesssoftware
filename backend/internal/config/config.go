package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port           string
	DatabaseURL    string
	StoragePath    string
	ClamScanPath   string
	RetentionDays  int
	BackupVerified bool
}

func FromEnvironment() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "data/uploads"
	}
	retentionDays, _ := strconv.Atoi(os.Getenv("ATTACHMENT_RETENTION_DAYS"))
	if retentionDays == 0 {
		retentionDays = 365
	}

	return Config{
		Port:           port,
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		StoragePath:    storagePath,
		ClamScanPath:   os.Getenv("CLAMDSCAN_PATH"),
		RetentionDays:  retentionDays,
		BackupVerified: os.Getenv("BACKUP_VERIFIED") == "true",
	}
}
