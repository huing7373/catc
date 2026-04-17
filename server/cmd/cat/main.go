package main

import (
	"flag"

	"github.com/huing/cat/server/internal/config"
)

var configPath = flag.String("config", "config/default.toml", "path to toml config")
func main() {
	flag.Parse()
	cfg := config.MustLoad(*configPath)
	app := initialize(cfg)
	app.Run()
}
