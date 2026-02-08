package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/weiawesome/wes-io-live/pkg/database"
	"github.com/weiawesome/wes-io-live/pkg/middleware"
	"github.com/weiawesome/wes-io-live/user-service/internal/config"
	"github.com/weiawesome/wes-io-live/user-service/internal/domain"
	"github.com/weiawesome/wes-io-live/user-service/internal/handler"
	"github.com/weiawesome/wes-io-live/user-service/internal/repository"
	"github.com/weiawesome/wes-io-live/user-service/internal/service"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database using GORM
	dbConfig := &database.Config{
		Driver:          cfg.Database.Driver,
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		User:            cfg.Database.User,
		Password:        cfg.Database.Password,
		DBName:          cfg.Database.DBName,
		SSLMode:         cfg.Database.SSLMode,
		FilePath:        cfg.Database.FilePath,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
	}

	db, err := database.New(dbConfig)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate
	if err := database.AutoMigrate(db, &domain.UserModel{}); err != nil {
		log.Fatalf("Failed to auto-migrate: %v", err)
	}
	log.Println("Database migration completed")

	// Initialize repository
	userRepo := repository.NewGormUserRepository(db)

	// Initialize service
	userService, err := service.NewUserService(userRepo, cfg.AuthService.GRPCAddress)
	if err != nil {
		log.Fatalf("Failed to create user service: %v", err)
	}

	// Initialize auth middleware
	authMiddleware, err := middleware.NewAuthMiddleware(cfg.AuthService.GRPCAddress)
	if err != nil {
		log.Fatalf("Failed to create auth middleware: %v", err)
	}

	// Initialize HTTP handler
	httpHandler := handler.NewHandler(userService, authMiddleware)

	// Setup Gin router
	r := gin.Default()

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Register routes
	httpHandler.RegisterRoutes(r)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("User Service starting on %s (driver: %s)", addr, cfg.Database.Driver)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
