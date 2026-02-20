package main

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/weiawesome/wes-io-live/pkg/database"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/pkg/middleware"
	"github.com/weiawesome/wes-io-live/user-service/internal/cache"
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
		l := pkglog.L()
		l.Fatal().Err(err).Msg("failed to load config")
	}

	// Initialize structured logger
	pkglog.Init(pkglog.Config{
		Level:       cfg.Log.Level,
		Pretty:      cfg.Log.Level == "debug",
		ServiceName: "user-service",
	})
	logger := pkglog.L()

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
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}

	// Auto-migrate
	if err := database.AutoMigrate(db, &domain.UserModel{}); err != nil {
		logger.Fatal().Err(err).Msg("failed to auto-migrate")
	}
	logger.Info().Msg("database migration completed")

	// Initialize repository
	userRepo := repository.NewGormUserRepository(db)

	// Initialize Redis cache
	userCache, err := cache.NewRedisUserCache(cfg.Redis, cfg.Cache.Prefix)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer userCache.Close()
	logger.Info().Str("addr", cfg.Redis.Address).Msg("redis connected")

	// Initialize service
	userService, err := service.NewUserService(userRepo, cfg.AuthService.GRPCAddress, userCache, cfg.Cache.TTL)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create user service")
	}

	// Initialize auth middleware
	authMiddleware, err := middleware.NewAuthMiddleware(cfg.AuthService.GRPCAddress)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create auth middleware")
	}

	// Initialize HTTP handler
	httpHandler := handler.NewHandler(userService, authMiddleware)

	// Setup Gin router
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(pkglog.GinMiddleware(logger))

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Register routes
	httpHandler.RegisterRoutes(r)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Info().Str("addr", addr).Str("driver", cfg.Database.Driver).Msg("user-service starting")
	if err := r.Run(addr); err != nil {
		logger.Fatal().Err(err).Msg("failed to start server")
	}
}
