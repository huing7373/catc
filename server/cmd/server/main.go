package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/huing7373/catc/server/internal/config"
	"github.com/huing7373/catc/server/internal/handler"
	"github.com/huing7373/catc/server/internal/middleware"
	catredis "github.com/huing7373/catc/server/pkg/redis"
)

func main() {
	// Init logger
	middleware.InitLogger()

	// Load config
	cfg := config.Load()

	// Init database
	db := initDB(cfg)

	// Run migrations
	runMigrations(cfg)

	// Init Redis
	rdb := initRedis(cfg)

	// Init services (placeholder — services will be added in subsequent stories)
	handlers := initServices(db, rdb)

	// Init router
	r := initRouter(cfg, handlers.health)

	// Run
	fmt.Printf("cat server starting on port %s...\n", cfg.ServerPort)
	if err := r.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}

func initDB(cfg *config.Config) *gorm.DB {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	return db
}

func runMigrations(cfg *config.Config) {
	// Determine migrations path
	migrationsPath := "file://migrations"
	if _, err := os.Stat("migrations"); os.IsNotExist(err) {
		// Try relative to working directory
		migrationsPath = "file://../../migrations"
	}

	m, err := migrate.New(migrationsPath, cfg.MigrationDSN())
	if err != nil {
		log.Fatalf("failed to create migrate instance: %v", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("failed to run migrations: %v", err)
	}
}

func initRedis(cfg *config.Config) *catredis.Client {
	rdb, err := catredis.New(cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	return rdb
}

// handlers groups all HTTP handlers for dependency injection into the router.
type handlers struct {
	health *handler.HealthHandler
}

// initServices creates repositories, services, and handlers.
// In this story only HealthHandler exists; future stories will add
// authService, userService, etc. following the same pattern.
func initServices(db *gorm.DB, rdb *catredis.Client) *handlers {
	healthHandler := handler.NewHealthHandler(db, rdb)
	return &handlers{
		health: healthHandler,
	}
}

func initRouter(cfg *config.Config, healthHandler *handler.HealthHandler) *gin.Engine {
	gin.SetMode(cfg.GinMode)

	r := gin.New()

	// Global middleware
	r.Use(middleware.RequestLogger())
	r.Use(gin.Recovery())

	// Public routes
	r.GET("/health", healthHandler.Health)

	// API v1 route group (placeholder for future stories)
	_ = r.Group("/v1")

	return r
}
