package main

import (
	"log"

	"sandbox-voice-ai/internal/app"
)

func main() {
	cfg := app.LoadConfig()

	server, err := app.NewServer(cfg)
	if err != nil {
		log.Fatalf("build server: %v", err)
	}

	log.Printf("listening on %s", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
