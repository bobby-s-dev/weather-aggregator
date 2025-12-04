package api

import (
	"strconv"

	"weather-aggregator/internal/services"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type Handler struct {
	aggregator *services.Aggregator
	logger     *zap.Logger
}

func NewHandler(aggregator *services.Aggregator, logger *zap.Logger) *Handler {
	return &Handler{
		aggregator: aggregator,
		logger:     logger,
	}
}

// GetCurrentWeather handles GET /api/v1/weather/current
func (h *Handler) GetCurrentWeather(c *fiber.Ctx) error {
	city := c.Query("city")
	if city == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "City parameter is required",
		})
	}
	
	h.logger.Info("Fetching current weather", zap.String("city", city))
	
	weather, err := h.aggregator.GetAggregatedCurrentWeather(c.Context(), city)
	if err != nil {
		h.logger.Error("Failed to get current weather",
			zap.String("city", city),
			zap.Error(err))
		
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch weather data",
			"details": err.Error(),
		})
	}
	
	return c.JSON(weather)
}

// GetForecast handles GET /api/v1/weather/forecast
func (h *Handler) GetForecast(c *fiber.Ctx) error {
	city := c.Query("city")
	if city == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "City parameter is required",
		})
	}
	
	daysStr := c.Query("days", "3")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days < 1 || days > 7 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Days parameter must be between 1 and 7",
		})
	}
	
	h.logger.Info("Fetching forecast",
		zap.String("city", city),
		zap.Int("days", days))
	
	forecast, err := h.aggregator.GetAggregatedForecast(c.Context(), city, days)
	if err != nil {
		h.logger.Error("Failed to get forecast",
			zap.String("city", city),
			zap.Int("days", days),
			zap.Error(err))
		
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch forecast data",
			"details": err.Error(),
		})
	}
	
	return c.JSON(forecast)
}

// GetHealth handles GET /api/v1/health
func (h *Handler) GetHealth(c *fiber.Ctx) error {
	lastFetch := h.aggregator.GetLastFetchTime()
	stats := h.aggregator.GetStats()
	
	return c.JSON(fiber.Map{
		"status":    "healthy",
		"timestamp": time.Now(),
		"last_fetch": lastFetch,
		"uptime":    time.Since(startTime).String(),
		"stats":     stats,
	})
}

// GetMetrics handles GET /api/v1/metrics
func (h *Handler) GetMetrics(c *fiber.Ctx) error {
	stats := h.aggregator.GetStats()
	
	return c.JSON(fiber.Map{
		"metrics": stats,
		"timestamp": time.Now(),
	})
}

// GetCities handles GET /api/v1/cities
func (h *Handler) GetCities(c *fiber.Ctx) error {
	// This would typically come from configuration
	// For now, return a hardcoded list
	cities := []string{
		"Prague",
		"London",
		"NewYork",
		"Tokyo",
		"Sydney",
	}
	
	return c.JSON(fiber.Map{
		"cities": cities,
	})
}

var startTime = time.Now()