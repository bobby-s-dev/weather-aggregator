package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"weather-aggregator/internal/models"
	"weather-aggregator/pkg/client"
	"go.uber.org/zap"
)

type Aggregator struct {
	clients        []WeatherClient
	cache          *WeatherCache
	logger         *zap.Logger
	mu             sync.RWMutex
	lastFetchTime  time.Time
	successCount   int
	failureCount   int
	weatherData    map[string]*models.WeatherData // city -> weather data
}

type WeatherClient interface {
	GetCurrentWeather(ctx context.Context, city string) (*models.CurrentWeather, error)
	GetForecast(ctx context.Context, city string, days int) (*models.WeatherForecast, error)
}

func NewAggregator(cfg *config.Config, logger *zap.Logger) (*Aggregator, error) {
	clientConfig := client.ClientConfig{
		Timeout:       10 * time.Second,
		MaxRetries:    cfg.Retry.MaxRetries,
		RetryDelay:    cfg.Retry.Delay,
		Multiplier:    cfg.Retry.Multiplier,
		Threshold:     cfg.CircuitBreaker.Threshold,
		BreakerTimeout: cfg.CircuitBreaker.Timeout,
	}
	
	var clients []WeatherClient
	
	// Initialize OpenWeatherMap client if API key is provided
	if cfg.WeatherAPI.OpenWeatherAPIKey != "" {
		openWeatherClient := client.NewOpenWeatherClient(
			cfg.WeatherAPI.OpenWeatherAPIKey,
			clientConfig,
			logger,
		)
		clients = append(clients, openWeatherClient)
		logger.Info("OpenWeatherMap client initialized")
	}
	
	// Initialize Open-Meteo client (no API key required)
	openMeteoClient := client.NewOpenMeteoClient(clientConfig, logger)
	clients = append(clients, openMeteoClient)
	logger.Info("Open-Meteo client initialized")
	
	// Note: You can add WeatherAPI.com client similarly
	
	if len(clients) == 0 {
		return nil, fmt.Errorf("no weather clients initialized")
	}
	
	cache := NewWeatherCache(cfg.Cache.Duration, cfg.Cache.MaxSize, logger)
	
	return &Aggregator{
		clients:      clients,
		cache:        cache,
		logger:       logger,
		weatherData:  make(map[string]*models.WeatherData),
	}, nil
}

func (a *Aggregator) FetchWeatherData(ctx context.Context, cities []string) error {
	a.mu.Lock()
	a.lastFetchTime = time.Now()
	a.mu.Unlock()
	
	var wg sync.WaitGroup
	errors := make(chan error, len(cities))
	
	startTime := time.Now()
	
	for _, city := range cities {
		wg.Add(1)
		go func(city string) {
			defer wg.Done()
			
			if err := a.fetchCityWeather(ctx, city); err != nil {
				a.logger.Error("Failed to fetch weather for city",
					zap.String("city", city),
					zap.Error(err))
				errors <- err
				a.mu.Lock()
				a.failureCount++
				a.mu.Unlock()
			} else {
				a.mu.Lock()
				a.successCount++
				a.mu.Unlock()
			}
		}(city)
	}
	
	wg.Wait()
	close(errors)
	
	duration := time.Since(startTime)
	a.logger.Info("Weather fetch completed",
		zap.Int("cities", len(cities)),
		zap.Duration("duration", duration),
		zap.Int("success", a.successCount),
		zap.Int("failure", a.failureCount))
	
	// Check if we got any errors
	hasErrors := false
	for err := range errors {
		if err != nil {
			hasErrors = true
			break
		}
	}
	
	if hasErrors {
		return fmt.Errorf("some cities failed to fetch weather data")
	}
	
	return nil
}

func (a *Aggregator) fetchCityWeather(ctx context.Context, city string) error {
	var wg sync.WaitGroup
	responses := make(chan models.APIResponse, len(a.clients))
	
	// Fetch from all clients concurrently
	for _, client := range a.clients {
		wg.Add(1)
		go func(c WeatherClient, source string) {
			defer wg.Done()
			
			response := models.APIResponse{Source: source}
			
			// Fetch current weather
			current, err := c.GetCurrentWeather(ctx, city)
			if err != nil {
				a.logger.Warn("Failed to fetch current weather from source",
					zap.String("source", source),
					zap.String("city", city),
					zap.Error(err))
				response.Error = err
			} else {
				response.Current = current
			}
			
			// Fetch forecast (3 days)
			forecast, err := c.GetForecast(ctx, city, 3)
			if err != nil {
				a.logger.Warn("Failed to fetch forecast from source",
					zap.String("source", source),
					zap.String("city", city),
					zap.Error(err))
				if response.Error == nil {
					response.Error = err
				}
			} else {
				response.Forecast = forecast
			}
			
			responses <- response
		}(client, getSourceName(client))
	}
	
	wg.Wait()
	close(responses)
	
	// Process responses
	weatherData := &models.WeatherData{
		City:      city,
		Current:   make(map[string]*models.CurrentWeather),
		Forecasts: make(map[string]*models.WeatherForecast),
		Timestamp: time.Now(),
	}
	
	successCount := 0
	for response := range responses {
		if response.Current != nil {
			weatherData.Current[response.Source] = response.Current
			successCount++
		}
		if response.Forecast != nil {
			weatherData.Forecasts[response.Source] = response.Forecast
		}
	}
	
	if successCount == 0 {
		return fmt.Errorf("all API calls failed for city %s", city)
	}
	
	a.mu.Lock()
	a.weatherData[city] = weatherData
	a.mu.Unlock()
	
	// Aggregate and cache the results
	a.aggregateAndCache(city)
	
	return nil
}

func (a *Aggregator) aggregateAndCache(city string) {
	a.mu.RLock()
	weatherData, exists := a.weatherData[city]
	a.mu.RUnlock()
	
	if !exists || len(weatherData.Current) == 0 {
		return
	}
	
	// Aggregate current weather
	aggregatedCurrent := a.aggregateCurrentWeather(weatherData)
	a.cache.SetCurrentWeather(city, aggregatedCurrent)
	
	// Aggregate forecast
	for days := 1; days <= 7; days++ {
		aggregatedForecast := a.aggregateForecast(weatherData, days)
		if aggregatedForecast != nil {
			a.cache.SetForecast(city, days, aggregatedForecast)
		}
	}
}

func (a *Aggregator) aggregateCurrentWeather(data *models.WeatherData) *models.AggregatedCurrentWeather {
	if len(data.Current) == 0 {
		return nil
	}
	
	var totalTemp, totalFeelsLike, totalHumidity, totalPressure, totalWindSpeed float64
	var descriptions []string
	var sources []string
	var latestTimestamp time.Time
	
	for source, weather := range data.Current {
		totalTemp += weather.Temperature
		totalFeelsLike += weather.FeelsLike
		totalHumidity += weather.Humidity
		totalPressure += weather.Pressure
		totalWindSpeed += weather.WindSpeed
		descriptions = append(descriptions, weather.Description)
		sources = append(sources, source)
		
		if weather.Timestamp.After(latestTimestamp) {
			latestTimestamp = weather.Timestamp
		}
	}
	
	count := float64(len(data.Current))
	
	// Calculate confidence based on number of sources and variance
	confidence := calculateConfidence(data.Current)
	
	// Find most common description
	description := mostCommonString(descriptions)
	
	// Use icon from first source
	var icon string
	for _, weather := range data.Current {
		icon = weather.Icon
		break
	}
	
	return &models.AggregatedCurrentWeather{
		City:        data.City,
		Temperature: totalTemp / count,
		FeelsLike:   totalFeelsLike / count,
		Humidity:    totalHumidity / count,
		Pressure:    totalPressure / count,
		WindSpeed:   totalWindSpeed / count,
		Description: description,
		Icon:        icon,
		LastUpdated: latestTimestamp,
		Sources:     sources,
		Confidence:  confidence,
	}
}

func (a *Aggregator) aggregateForecast(data *models.WeatherData, days int) *models.AggregatedForecast {
	if len(data.Forecasts) == 0 {
		return nil
	}
	
	// Collect forecasts from all sources
	allForecasts := make([][]models.ForecastDay, 0, len(data.Forecasts))
	var sources []string
	
	for source, forecast := range data.Forecasts {
		if len(forecast.Forecast) >= days {
			allForecasts = append(allForecasts, forecast.Forecast[:days])
			sources = append(sources, source)
		}
	}
	
	if len(allForecasts) == 0 {
		return nil
	}
	
	// Aggregate daily forecasts
	aggregatedDays := make([]models.ForecastDay, days)
	
	for day := 0; day < days; day++ {
		var totalMaxTemp, totalMinTemp, totalAvgTemp, totalHumidity, totalPrecipitation float64
		var dayDescriptions []string
		var date time.Time
		
		dayCount := 0
		for _, forecast := range allForecasts {
			if day < len(forecast) {
				dayForecast := forecast[day]
				totalMaxTemp += dayForecast.MaxTemp
				totalMinTemp += dayForecast.MinTemp
				totalAvgTemp += dayForecast.AvgTemp
				totalHumidity += dayForecast.Humidity
				totalPrecipitation += dayForecast.Precipitation
				dayDescriptions = append(dayDescriptions, dayForecast.Description)
				date = dayForecast.Date
				dayCount++
			}
		}
		
		if dayCount == 0 {
			continue
		}
		
		dayCountFloat := float64(dayCount)
		
		aggregatedDays[day] = models.ForecastDay{
			Date:          date,
			MaxTemp:       totalMaxTemp / dayCountFloat,
			MinTemp:       totalMinTemp / dayCountFloat,
			AvgTemp:       totalAvgTemp / dayCountFloat,
			Humidity:      totalHumidity / dayCountFloat,
			Description:   mostCommonString(dayDescriptions),
			Icon:          allForecasts[0][day].Icon, // Use icon from first source
			Precipitation: totalPrecipitation / dayCountFloat,
		}
	}
	
	return &models.AggregatedForecast{
		City:        data.City,
		Days:        aggregatedDays,
		LastUpdated: time.Now(),
		Sources:     sources,
	}
}

func (a *Aggregator) GetAggregatedCurrentWeather(ctx context.Context, city string) (*models.AggregatedCurrentWeather, error) {
	// Check cache first
	if cached, ok := a.cache.GetCurrentWeather(city); ok {
		a.logger.Debug("Cache hit for current weather", zap.String("city", city))
		return cached, nil
	}
	
	// Fetch fresh data if not in cache
	a.logger.Debug("Cache miss for current weather, fetching fresh data", zap.String("city", city))
	
	// Use a shorter context timeout for this request
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	
	// Fetch from single city
	cities := []string{city}
	if err := a.FetchWeatherData(fetchCtx, cities); err != nil {
		return nil, fmt.Errorf("failed to fetch weather for %s: %w", city, err)
	}
	
	// Get from cache after fetch
	if cached, ok := a.cache.GetCurrentWeather(city); ok {
		return cached, nil
	}
	
	return nil, fmt.Errorf("weather data not available for %s", city)
}

func (a *Aggregator) GetAggregatedForecast(ctx context.Context, city string, days int) (*models.AggregatedForecast, error) {
	// Validate days parameter
	if days < 1 || days > 7 {
		return nil, fmt.Errorf("days must be between 1 and 7")
	}
	
	// Check cache first
	if cached, ok := a.cache.GetForecast(city, days); ok {
		a.logger.Debug("Cache hit for forecast",
			zap.String("city", city),
			zap.Int("days", days))
		return cached, nil
	}
	
	// Fetch fresh data if not in cache
	a.logger.Debug("Cache miss for forecast, fetching fresh data",
		zap.String("city", city),
		zap.Int("days", days))
	
	// Use a shorter context timeout for this request
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	
	// Fetch from single city
	cities := []string{city}
	if err := a.FetchWeatherData(fetchCtx, cities); err != nil {
		return nil, fmt.Errorf("failed to fetch forecast for %s: %w", city, err)
	}
	
	// Get from cache after fetch
	if cached, ok := a.cache.GetForecast(city, days); ok {
		return cached, nil
	}
	
	return nil, fmt.Errorf("forecast data not available for %s", city)
}

func (a *Aggregator) GetLastFetchTime() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastFetchTime
}

func (a *Aggregator) GetStats() map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	cacheStats := a.cache.GetStats()
	
	return map[string]interface{}{
		"last_fetch_time":  a.lastFetchTime,
		"success_count":    a.successCount,
		"failure_count":    a.failureCount,
		"cities_stored":    len(a.weatherData),
		"active_clients":   len(a.clients),
		"cache_stats":      cacheStats,
	}
}

func getSourceName(client interface{}) string {
	switch client.(type) {
	case *client.OpenWeatherClient:
		return "openweathermap"
	case *client.OpenMeteoClient:
		return "open-meteo"
	default:
		return "unknown"
	}
}

func calculateConfidence(currentWeather map[string]*models.CurrentWeather) float64 {
	if len(currentWeather) <= 1 {
		return 0.5
	}
	
	// Calculate variance in temperatures
	var temps []float64
	for _, weather := range currentWeather {
		temps = append(temps, weather.Temperature)
	}
	
	mean := 0.0
	for _, temp := range temps {
		mean += temp
	}
	mean /= float64(len(temps))
	
	variance := 0.0
	for _, temp := range temps {
		diff := temp - mean
		variance += diff * diff
	}
	variance /= float64(len(temps))
	
	// Lower variance = higher confidence
	// Normalize variance to 0-1 range (assuming max variance of 25 degrees)
	normalizedVariance := variance / 25.0
	if normalizedVariance > 1 {
		normalizedVariance = 1
	}
	
	confidence := 1 - normalizedVariance
	
	// Boost confidence with more sources
	sourceBoost := float64(len(currentWeather)-1) * 0.1
	confidence += sourceBoost
	
	if confidence > 1 {
		confidence = 1
	}
	if confidence < 0 {
		confidence = 0
	}
	
	return confidence
}

func mostCommonString(strs []string) string {
	counts := make(map[string]int)
	for _, s := range strs {
		counts[s]++
	}
	
	var mostCommon string
	maxCount := 0
	for s, count := range counts {
		if count > maxCount {
			mostCommon = s
			maxCount = count
		}
	}
	
	return mostCommon
}