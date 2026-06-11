package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var (
	socketPath = flag.String("socket", "/run/weldr/api.socket", "Unix socket path")
	cacheDir   = flag.String("cache-dir", "/var/cache/weldr-shim", "Cache directory path")
)

func main() {
	flag.Parse()

	log.SetPrefix("weldr-shim: ")
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Detect image-builder binary
	if err := DetectImageBuilder(); err != nil {
		log.Fatalf("Image builder detection failed: %v", err)
	}
	log.Printf("Using binary: %s", imageBuilderBinary)

	// Initialize storage
	log.Printf("Initializing storage in %s", *cacheDir)
	if err := InitializeStorage(*cacheDir); err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	// Load image types cache
	log.Printf("Loading image types cache from %s", imageBuilderBinary)
	if err := LoadImageTypesCache(); err != nil {
		log.Fatalf("Failed to load image types cache: %v", err)
	}
	log.Printf("Loaded %d distro/arch/image-type combinations", len(imageTypesCache))

	// Set up HTTP routes
	mux := http.NewServeMux()
	setupRoutes(mux)

	// Remove old socket if exists
	if err := os.Remove(*socketPath); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: failed to remove old socket: %v", err)
	}

	// Ensure socket directory exists
	socketDir := filepath.Dir(*socketPath)
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		log.Fatalf("Failed to create socket directory: %v", err)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("Failed to create socket listener: %v", err)
	}
	defer listener.Close()

	// Set socket permissions
	if err := os.Chmod(*socketPath, 0666); err != nil {
		log.Printf("Warning: failed to set socket permissions: %v", err)
	}

	// Start queue processor
	go processQueue(*cacheDir)

	// Create HTTP server
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Printf("Shutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	log.Printf("Listening on %s", *socketPath)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	log.Printf("Server stopped")
}

// setupRoutes registers all HTTP handlers
func setupRoutes(mux *http.ServeMux) {
	// Status
	mux.HandleFunc("GET /api/status", handleStatus)

	// Blueprints
	mux.HandleFunc("GET /api/v1/blueprints/list", handleBlueprintsList)
	mux.HandleFunc("GET /api/v1/blueprints/info/", handleBlueprintsInfo)
	mux.HandleFunc("POST /api/v1/blueprints/new", handleBlueprintsNew)
	mux.HandleFunc("DELETE /api/v1/blueprints/delete/", handleBlueprintDelete)

	// Compose
	mux.HandleFunc("GET /api/v1/compose/types", handleComposeTypes)
	mux.HandleFunc("POST /api/v1/compose", handleComposeStart)
	mux.HandleFunc("GET /api/v1/compose/status/", handleComposeStatus)
	mux.HandleFunc("GET /api/v1/compose/info/", handleComposeInfo)
	mux.HandleFunc("GET /api/v1/compose/queue", handleComposeQueue)
	mux.HandleFunc("GET /api/v1/compose/finished", handleComposeFinished)
	mux.HandleFunc("GET /api/v1/compose/failed", handleComposeFailed)
	mux.HandleFunc("GET /api/v1/compose/image/", handleComposeImage)
	mux.HandleFunc("DELETE /api/v1/compose/delete/", handleComposeDelete)
	mux.HandleFunc("DELETE /api/v1/compose/cancel/", handleComposeCancel)

	// Distros
	mux.HandleFunc("GET /api/v1/distros/list", handleDistrosList)

	// Catch-all for not implemented
	mux.HandleFunc("/", handleNotImplemented)
}
