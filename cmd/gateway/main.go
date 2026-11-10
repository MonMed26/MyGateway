// Command gateway runs the MyGateway semi payment gateway: HTTP API, SQLite
// persistence, provider polling and webhook delivery.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MonMed26/MyGateway/internal/db"
	"github.com/MonMed26/MyGateway/internal/gateway"
	"github.com/MonMed26/MyGateway/internal/httpapi"
	"github.com/MonMed26/MyGateway/internal/repo"
	"github.com/MonMed26/MyGateway/internal/security"
	"github.com/MonMed26/MyGateway/internal/webhook"
	"github.com/MonMed26/MyGateway/internal/webui"
	"github.com/MonMed26/MyGateway/utils"

	"golang.org/x/crypto/bcrypt"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	addr := flag.String("addr", envOr("GATEWAY_ADDR", ":8080"), "HTTP listen address")
	dbPath := flag.String("db", envOr("GATEWAY_DB", "./data/gateway.db"), "SQLite database path")
	keyFile := flag.String("master-key-file", envOr("GATEWAY_MASTER_KEY_FILE", "./data/master.key"), "master key file (used when GATEWAY_MASTER_KEY is unset)")
	adminUser := flag.String("admin-user", envOr("GATEWAY_ADMIN_USER", "admin"), "initial admin username")
	adminPass := flag.String("admin-pass", envOr("GATEWAY_ADMIN_PASS", "admin123"), "initial admin password (only used when no admin exists)")
	flag.Parse()

	logger := utils.NewConsoleLogger(utils.LevelInfo)

	conn, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer conn.Close()

	store := repo.New(conn)

	if err := store.EnsureSettings(security.NewToken(32), security.NewToken(24)); err != nil {
		log.Fatalf("seed settings: %v", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(*adminPass), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash admin password: %v", err)
	}
	created, err := store.EnsureAdmin(*adminUser, string(hash))
	if err != nil {
		log.Fatalf("seed admin: %v", err)
	}
	if created {
		logger.Warn("initial admin account created — change the password before exposing the dashboard", map[string]any{
			"username": *adminUser,
		})
	}

	masterKey, err := security.LoadOrCreateMasterKey(os.Getenv("GATEWAY_MASTER_KEY"), *keyFile)
	if err != nil {
		log.Fatalf("load master key: %v", err)
	}

	dispatcher := webhook.New(store, logger)
	dispatcher.Start()
	defer dispatcher.Stop()

	manager := gateway.NewManager(store, masterKey, logger, dispatcher)
	manager.StartAll(context.Background())
	defer manager.Close()

	ui := webui.Handler()
	api := httpapi.New(store, manager, dispatcher, logger, ui)

	server := &http.Server{
		Addr:              *addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("gateway listening", map[string]any{"addr": *addr, "db": *dbPath})
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
