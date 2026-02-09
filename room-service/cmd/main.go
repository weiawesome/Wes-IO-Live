package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/weiawesome/wes-io-live/pkg/database"
	"github.com/weiawesome/wes-io-live/pkg/middleware"
	"github.com/weiawesome/wes-io-live/room-service/internal/config"
	"github.com/weiawesome/wes-io-live/room-service/internal/domain"
	"github.com/weiawesome/wes-io-live/room-service/internal/handler"
	"github.com/weiawesome/wes-io-live/room-service/internal/repository"
	"github.com/weiawesome/wes-io-live/room-service/internal/service"
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
	if err := database.AutoMigrate(db, &domain.RoomModel{}); err != nil {
		log.Fatalf("Failed to auto-migrate: %v", err)
	}
	log.Println("Database migration completed")

	// Initialize repository
	roomRepo := repository.NewGormRoomRepository(db)

	// Initialize service
	roomService := service.NewRoomService(roomRepo, cfg.Room.MaxRoomsPerUser)

	// Initialize auth middleware
	authMiddleware, err := middleware.NewAuthMiddleware(cfg.AuthService.GRPCAddress)
	if err != nil {
		log.Fatalf("Failed to create auth middleware: %v", err)
	}

	// Initialize HTTP handler
	httpHandler := handler.NewHandler(roomService, authMiddleware)

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
	log.Printf("Room Service starting on %s (driver: %s, max rooms per user: %d)", addr, cfg.Database.Driver, cfg.Room.MaxRoomsPerUser)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
