package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/huing/cat/server/internal/app/bootstrap"
	"github.com/huing/cat/server/internal/infra/config"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "path to config YAML (default: auto-detect server/configs/local.yaml or configs/local.yaml)")
	flag.Parse()

	if configPath == "" {
		p, err := config.LocateDefault()
		if err != nil {
			log.Fatalf("%v", err)
		}
		configPath = p
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}

	log.Printf("config loaded: http_port=%d log.level=%s", cfg.Server.HTTPPort, cfg.Log.Level)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := bootstrap.Run(ctx, cfg); err != nil {
		log.Fatalf("server run failed: %v", err)
	}
}
