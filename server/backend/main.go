package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/altlimit/dsorm"
	"github.com/altlimit/dsorm/ds/local"
	rs "github.com/altlimit/restruct"
	"google.golang.org/appengine/v2"
	"passedbox.com/api"
	"passedbox.com/model"
	"passedbox.com/push"
)

//go:embed public
var publicFS embed.FS

func main() {
	ctx := context.Background()

	// Initialize dsorm client
	dataDir := os.Getenv("DATA_DIR")
	var opts []dsorm.Option
	if dataDir != "" {
		// Local SQLite mode
		store := local.NewStore(dataDir)
		opts = append(opts, dsorm.WithStore(store))
	}

	encKey := os.Getenv("ENCRYPTION_KEY")
	if encKey == "" {
		// Determine key file path: DATA_DIR/enc.key or ./enc.key
		keyDir := dataDir
		if keyDir == "" {
			keyDir = "."
		}
		keyPath := filepath.Join(keyDir, "enc.key")
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			raw := make([]byte, 16)
			if _, err := rand.Read(raw); err != nil {
				slog.Error("Failed to generate encryption key", "error", err)
				os.Exit(1)
			}
			encKey = hex.EncodeToString(raw)
			if err := os.WriteFile(keyPath, []byte(encKey), 0600); err != nil {
				slog.Error("Failed to write encryption key file", "error", err, "path", keyPath)
				os.Exit(1)
			}
			slog.Info("Generated new encryption key", "path", keyPath)
		} else {
			encKey = strings.TrimSpace(string(keyData))
		}
	}
	if encKey != "" {
		opts = append(opts, dsorm.WithEncryptionKey([]byte(encKey)))
	}

	client, err := dsorm.New(ctx, opts...)
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	model.Client = client
	api.Cache = client.Cache()
	// Initialize admin auth (auto-generates password on first run)
	authConfig, err := api.InitAdmin(ctx, client)
	if err != nil {
		slog.Error("Failed to initialize admin auth", "error", err)
		os.Exit(1)
	}

	// Load Stripe / payment configuration from env vars
	config, err := model.Config(ctx)
	if err != nil {
		slog.Debug("No existing server config, using defaults", "error", err)
	}

	configChanged := false
	if v := os.Getenv("BASE_URL"); v != "" {
		config.BaseURL = v
		configChanged = true
	}
	if configChanged {
		if err := client.Put(ctx, config); err != nil {
			slog.Error("Failed to save server config", "error", err)
		}
	}

	pushSender := &push.Sender{DB: client}

	vaultAPI := api.VaultAPI{DB: client, Push: pushSender, Auth: authConfig}

	// Start background worker
	workerInterval := 12 * time.Hour
	if intervalStr := os.Getenv("WORKER_INTERVAL"); intervalStr != "" {
		if d, err := time.ParseDuration(intervalStr); err == nil {
			workerInterval = d
		}
	}
	task := NewTask(client, pushSender, &vaultAPI)
	svr := NewServer(vaultAPI)

	if useHttpTask := os.Getenv("HTTP_TASK"); useHttpTask != "" {
		if useHttpTask != "true" {
			task.taskKey = useHttpTask
		}
		svr.Task = task
	} else {
		go task.StartWorker(ctx, workerInterval)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8085"
	}

	slog.Info("Server started", "addr", "http://localhost:"+port)
	rs.Handle("/", svr)

	if appengine.IsAppEngine() {
		appengine.Main()
	} else {
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			slog.Error("Server failed", "error", err)
		}
	}
}
