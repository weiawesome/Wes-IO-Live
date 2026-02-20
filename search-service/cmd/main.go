package main

import (
	"fmt"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/gin-gonic/gin"

	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/search-service/internal/cache"
	"github.com/weiawesome/wes-io-live/search-service/internal/config"
	"github.com/weiawesome/wes-io-live/search-service/internal/handler"
	"github.com/weiawesome/wes-io-live/search-service/internal/repository"
	"github.com/weiawesome/wes-io-live/search-service/internal/service"
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
		ServiceName: "search-service",
	})
	logger := pkglog.L()

	// Initialize Elasticsearch client
	esClient, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: cfg.Elasticsearch.Addresses,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create elasticsearch client")
	}

	// Verify ES connection
	res, err := esClient.Info()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to elasticsearch")
	}
	res.Body.Close()
	logger.Info().Strs("addresses", cfg.Elasticsearch.Addresses).Msg("elasticsearch connected")

	// Initialize repository
	searchRepo := repository.NewESSearchRepository(
		esClient,
		cfg.Elasticsearch.IndexUsers,
		cfg.Elasticsearch.IndexRooms,
	)

	// Initialize Redis cache
	searchCache, err := cache.NewRedisSearchCache(cfg.Redis, cfg.Cache.Prefix)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer searchCache.Close()
	logger.Info().Str("addr", cfg.Redis.Address).Msg("redis connected")

	// Initialize service
	searchService := service.NewSearchService(searchRepo, searchCache, cfg.Cache.TTL)

	// Initialize HTTP handler
	httpHandler := handler.NewHandler(searchService)

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
	logger.Info().Str("addr", addr).Msg("search-service starting")
	if err := r.Run(addr); err != nil {
		logger.Fatal().Err(err).Msg("failed to start server")
	}
}
