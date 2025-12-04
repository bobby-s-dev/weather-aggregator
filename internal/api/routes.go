package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"go.uber.org/zap"
)

func SetupRoutes(app *fiber.App, handler *Handler, log *zap.Logger) {
	// Middleware
	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,HEAD,PUT,DELETE,PATCH",
	}))
	
	// Custom logger middleware
	app.Use(logger.New(logger.Config{
		Format: "${time} ${pid} ${locals:requestid} ${status} - ${method} ${path}\n",
		TimeFormat: time.RFC3339,
	}))
	
	// API v1 routes
	api := app.Group("/api/v1")
	
	// Health check
	api.Get("/health", handler.GetHealth)
	
	// Metrics
	api.Get("/metrics", handler.GetMetrics)
	
	// Cities
	api.Get("/cities", handler.GetCities)
	
	// Weather routes
	weather := api.Group("/weather")
	weather.Get("/current", handler.GetCurrentWeather)
	weather.Get("/forecast", handler.GetForecast)
	
	// 404 handler
	app.Use(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Endpoint not found",
			"path":  c.Path(),
		})
	})
}