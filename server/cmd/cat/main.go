// Command cat is the HTTP + WebSocket server that powers the Apple
// Watch kuachacat companion. The entrypoint only parses flags, loads
// config, and hands off to the App container.
package main

import (
	"flag"

	"github.com/huing7373/catc/server/internal/config"
)

var configPath = flag.String("config", "config/local.toml", "path to toml config")

func main() {
	flag.Parse()
	cfg := config.MustLoad(*configPath)
	app := initialize(cfg)
	app.Run()
}
