//go:build tools

// This file declares tool/future dependencies that are required by the project
// but not yet imported in source code. This prevents `go mod tidy` from removing them.
package tools

import (
	_ "github.com/golang-jwt/jwt/v5"
	_ "github.com/gorilla/websocket"
	_ "github.com/robfig/cron/v3"
	_ "github.com/sideshow/apns2"
)
