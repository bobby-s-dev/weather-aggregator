package services

import (
	"sync"
	"time"

	"weather-aggregator/internal/models"
	"go.uber.org/zap"
)

type CacheItem struct {
	Data       interface{}
	ExpiresAt  time.Time
}

type WeatherCache struct {
	mu               sync.RWMutex
	currentWeather   map[string]CacheItem
	forecast         map[string]map[int]CacheItem // city -> days -> cache item
	logger           *zap.Logger
	defaultDuration  time.Duration
	maxSize          int
	cleanupInterval  time.Duration
	stopCleanup      chan bool
}

func NewWeatherCache(defaultDuration time.Duration, maxSize int, logger *zap.Logger) *WeatherCache {
	cache := &WeatherCache{
		currentWeather:  make(map[string]CacheItem),
		forecast:        make(map[string]map[int]CacheItem),
		logger:          logger,
		defaultDuration: defaultDuration,
		maxSize:         maxSize,
		cleanupInterval: time.Minute,
		stopCleanup:     make(chan bool),
	}
	
	go cache.startCleanup()
	
	return cache
}

func (c *WeatherCache) SetCurrentWeather(city string, weather *models.AggregatedCurrentWeather) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Evict if cache is too large
	if len(c.currentWeather) >= c.maxSize {
		c.evictOldestCurrent()
	}
	
	c.currentWeather[city] = CacheItem{
		Data:      weather,
		ExpiresAt: time.Now().Add(c.defaultDuration),
	}
	
	c.logger.Debug("Current weather cached",
		zap.String("city", city),
		zap.Time("expires_at", time.Now().Add(c.defaultDuration)))
}

func (c *WeatherCache) GetCurrentWeather(city string) (*models.AggregatedCurrentWeather, bool) {
	c.mu.RLock()
	item, exists := c.currentWeather[city]
	c.mu.RUnlock()
	
	if !exists {
		return nil, false
	}
	
	if time.Now().After(item.ExpiresAt) {
		c.mu.Lock()
		delete(c.currentWeather, city)
		c.mu.Unlock()
		return nil, false
	}
	
	weather, ok := item.Data.(*models.AggregatedCurrentWeather)
	return weather, ok
}

func (c *WeatherCache) SetForecast(city string, days int, forecast *models.AggregatedForecast) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if _, exists := c.forecast[city]; !exists {
		c.forecast[city] = make(map[int]CacheItem)
	}
	
	// Check total cache size
	totalItems := len(c.currentWeather)
	for _, cityForecasts := range c.forecast {
		totalItems += len(cityForecasts)
	}
	
	if totalItems >= c.maxSize {
		c.evictOldestForecast()
	}
	
	c.forecast[city][days] = CacheItem{
		Data:      forecast,
		ExpiresAt: time.Now().Add(c.defaultDuration),
	}
	
	c.logger.Debug("Forecast cached",
		zap.String("city", city),
		zap.Int("days", days),
		zap.Time("expires_at", time.Now().Add(c.defaultDuration)))
}

func (c *WeatherCache) GetForecast(city string, days int) (*models.AggregatedForecast, bool) {
	c.mu.RLock()
	cityForecasts, cityExists := c.forecast[city]
	if !cityExists {
		c.mu.RUnlock()
		return nil, false
	}
	
	item, exists := cityForecasts[days]
	c.mu.RUnlock()
	
	if !exists {
		return nil, false
	}
	
	if time.Now().After(item.ExpiresAt) {
		c.mu.Lock()
		delete(c.forecast[city], days)
		c.mu.Unlock()
		return nil, false
	}
	
	forecast, ok := item.Data.(*models.AggregatedForecast)
	return forecast, ok
}

func (c *WeatherCache) evictOldestCurrent() {
	var oldestKey string
	var oldestTime time.Time
	
	for key, item := range c.currentWeather {
		if oldestKey == "" || item.ExpiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = item.ExpiresAt
		}
	}
	
	if oldestKey != "" {
		delete(c.currentWeather, oldestKey)
		c.logger.Debug("Evicted oldest current weather from cache",
			zap.String("city", oldestKey))
	}
}

func (c *WeatherCache) evictOldestForecast() {
	var oldestCity string
	var oldestDays int
	var oldestTime time.Time
	
	for city, forecasts := range c.forecast {
		for days, item := range forecasts {
			if oldestCity == "" || item.ExpiresAt.Before(oldestTime) {
				oldestCity = city
				oldestDays = days
				oldestTime = item.ExpiresAt
			}
		}
	}
	
	if oldestCity != "" {
		delete(c.forecast[oldestCity], oldestDays)
		c.logger.Debug("Evicted oldest forecast from cache",
			zap.String("city", oldestCity),
			zap.Int("days", oldestDays))
	}
}

func (c *WeatherCache) startCleanup() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

func (c *WeatherCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	now := time.Now()
	expiredCount := 0
	
	// Clean current weather
	for city, item := range c.currentWeather {
		if now.After(item.ExpiresAt) {
			delete(c.currentWeather, city)
			expiredCount++
		}
	}
	
	// Clean forecast
	for city, forecasts := range c.forecast {
		for days, item := range forecasts {
			if now.After(item.ExpiresAt) {
				delete(forecasts, days)
				expiredCount++
			}
		}
		
		if len(forecasts) == 0 {
			delete(c.forecast, city)
		}
	}
	
	if expiredCount > 0 {
		c.logger.Debug("Cleaned expired cache items",
			zap.Int("count", expiredCount))
	}
}

func (c *WeatherCache) Stop() {
	close(c.stopCleanup)
}

func (c *WeatherCache) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return map[string]interface{}{
		"current_weather_items": len(c.currentWeather),
		"forecast_items":        len(c.forecast),
		"max_size":              c.maxSize,
		"default_duration":      c.defaultDuration.String(),
	}
}