package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"weather-aggregator/internal/api"
	"weather-aggregator/internal/config"
	"weather-aggregator/internal/scheduler"
	"weather-aggregator/internal/services"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	
	zap.ReplaceGlobals(logger)
	logger.Info("Starting Weather Data Aggregator Service")
	
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}
	
	// Initialize aggregator
	aggregator, err := services.NewAggregator(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to initialize aggregator", zap.Error(err))
	}
	
	// Initialize scheduler
	weatherScheduler := scheduler.NewScheduler(
		aggregator,
		cfg.Scheduler.DefaultCities,
		cfg.Scheduler.FetchInterval,
		logger,
	)
	
	// Create Fiber app
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		JSONEncoder:  fiber.DefaultJSONEncoder,
		ErrorHandler: errorHandler,
	})
	
	// Setup handlers and routes
	handler := api.NewHandler(aggregator, logger)
	api.SetupRoutes(app, handler, logger)
	
	// Start scheduler
	weatherScheduler.Start()
	
	// Start server in goroutine
	go func() {
		addr := ":" + cfg.Server.Port
		logger.Info("Starting server", zap.String("address", addr))
		
		if err := app.Listen(addr); err != nil {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()
	
	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	
	logger.Info("Shutting down server...")
	
	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Stop scheduler
	weatherScheduler.Stop()
	
	// Shutdown Fiber app
	if err := app.ShutdownWithContext(ctx); err != nil {
		logger.Error("Server shutdown failed", zap.Error(err))
	}
	
	logger.Info("Server stopped")
}

func errorHandler(c *fiber.Ctx, err error) error {
	zap.L().Error("HTTP error",
		zap.String("method", c.Method()),
		zap.String("path", c.Path()),
		zap.Error(err))
	
	// Default to 500 status code
	code := fiber.StatusInternalServerError
	
	// Check if it's a Fiber error
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	
	return c.Status(code).JSON(fiber.Map{
		"error":   err.Error(),
		"success": false,
	})
}