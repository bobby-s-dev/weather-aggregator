package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"weather-aggregator/internal/models"
	"go.uber.org/zap"
)

type OpenMeteoClient struct {
	*BaseClient
	baseURL string
}

type OpenMeteoCurrentResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Current   struct {
		Time          string  `json:"time"`
		Interval      int     `json:"interval"`
		Temperature2M float64 `json:"temperature_2m"`
		WindSpeed10M  float64 `json:"wind_speed_10m"`
		WindDirection float64 `json:"wind_direction_10m"`
		RelativeHumidity2M int `json:"relative_humidity_2m"`
		PressureMSL    float64 `json:"pressure_msl"`
		WeatherCode   int     `json:"weather_code"`
	} `json:"current"`
	CurrentUnits struct {
		Time          string `json:"time"`
		Temperature2M string `json:"temperature_2m"`
		WindSpeed10M  string `json:"wind_speed_10m"`
	} `json:"current_units"`
}

type OpenMeteoForecastResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Daily     struct {
		Time []string `json:"time"`
		Temperature2MMax []float64 `json:"temperature_2m_max"`
		Temperature2MMin []float64 `json:"temperature_2m_min"`
		PrecipitationSum []float64 `json:"precipitation_sum"`
		WeatherCode      []int     `json:"weather_code"`
	} `json:"daily"`
	DailyUnits struct {
		Time          string `json:"time"`
		Temperature2MMax string `json:"temperature_2m_max"`
		Temperature2MMin string `json:"temperature_2m_min"`
	} `json:"daily_units"`
}

func NewOpenMeteoClient(config ClientConfig, logger *zap.Logger) *OpenMeteoClient {
	baseClient := NewBaseClient("openmeteo", config, logger)
	return &OpenMeteoClient{
		BaseClient: baseClient,
		baseURL:    "https://api.open-meteo.com/v1",
	}
}

func (c *OpenMeteoClient) GetCurrentWeather(ctx context.Context, city string) (*models.CurrentWeather, error) {
	// Note: Open-Meteo requires coordinates, not city names
	// For simplicity, we'll use hardcoded coordinates for major cities
	coordinates := map[string]string{
		"Prague":  "50.0755,14.4378",
		"London":  "51.5074,-0.1278",
		"NewYork": "40.7128,-74.0060",
		"Tokyo":   "35.6762,139.6503",
		"Sydney":  "-33.8688,151.2093",
	}
	
	coords, ok := coordinates[city]
	if !ok {
		return nil, fmt.Errorf("coordinates not found for city: %s", city)
	}
	
	url := fmt.Sprintf("%s/forecast?latitude=%s&longitude=%s&current=temperature_2m,relative_humidity_2m,pressure_msl,wind_speed_10m,wind_direction_10m,weather_code", 
		c.baseURL, coords, coords[len(coords)/2:])
	
	data, err := c.GetWithRetry(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch current weather: %w", err)
	}
	
	var response OpenMeteoCurrentResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	currentTime, _ := time.Parse(time.RFC3339, response.Current.Time)
	weatherDesc := c.weatherCodeToDescription(response.Current.WeatherCode)
	
	weather := &models.CurrentWeather{
		City:        city,
		Temperature: response.Current.Temperature2M,
		FeelsLike:   response.Current.Temperature2M, // Open-Meteo doesn't provide feels like
		Humidity:    float64(response.Current.RelativeHumidity2M),
		Pressure:    response.Current.PressureMSL,
		WindSpeed:   response.Current.WindSpeed10M,
		WindDegree:  response.Current.WindDirection,
		Description: weatherDesc,
		Icon:        c.weatherCodeToIcon(response.Current.WeatherCode),
		Timestamp:   currentTime,
		Source:      "open-meteo",
	}
	
	return weather, nil
}

func (c *OpenMeteoClient) GetForecast(ctx context.Context, city string, days int) (*models.WeatherForecast, error) {
	coordinates := map[string]string{
		"Prague":  "50.0755,14.4378",
		"London":  "51.5074,-0.1278",
		"NewYork": "40.7128,-74.0060",
		"Tokyo":   "35.6762,139.6503",
		"Sydney":  "-33.8688,151.2093",
	}
	
	coords, ok := coordinates[city]
	if !ok {
		return nil, fmt.Errorf("coordinates not found for city: %s", city)
	}
	
	url := fmt.Sprintf("%s/forecast?latitude=%s&longitude=%s&daily=temperature_2m_max,temperature_2m_min,precipitation_sum,weather_code&forecast_days=%d",
		c.baseURL, coords, coords[len(coords)/2:], days)
	
	data, err := c.GetWithRetry(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch forecast: %w", err)
	}
	
	var response OpenMeteoForecastResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse forecast response: %w", err)
	}
	
	forecast := &models.WeatherForecast{
		City:     city,
		Forecast: make([]models.ForecastDay, 0, days),
		Source:   "open-meteo",
	}
	
	for i := 0; i < days && i < len(response.Daily.Time); i++ {
		date, _ := time.Parse("2006-01-02", response.Daily.Time[i])
		weatherDesc := c.weatherCodeToDescription(response.Daily.WeatherCode[i])
		
		dayForecast := models.ForecastDay{
			Date:         date,
			MaxTemp:      response.Daily.Temperature2MMax[i],
			MinTemp:      response.Daily.Temperature2MMin[i],
			AvgTemp:      (response.Daily.Temperature2MMax[i] + response.Daily.Temperature2MMin[i]) / 2,
			Description:  weatherDesc,
			Icon:         c.weatherCodeToIcon(response.Daily.WeatherCode[i]),
			Precipitation: response.Daily.PrecipitationSum[i],
		}
		
		forecast.Forecast = append(forecast.Forecast, dayForecast)
	}
	
	return forecast, nil
}

func (c *OpenMeteoClient) weatherCodeToDescription(code int) string {
	// WMO Weather interpretation codes
	weatherCodes := map[int]string{
		0: "Clear sky",
		1: "Mainly clear", 
		2: "Partly cloudy",
		3: "Overcast",
		45: "Foggy",
		48: "Depositing rime fog",
		51: "Light drizzle",
		53: "Moderate drizzle",
		55: "Dense drizzle",
		56: "Light freezing drizzle",
		57: "Dense freezing drizzle",
		61: "Slight rain",
		63: "Moderate rain",
		65: "Heavy rain",
		66: "Light freezing rain",
		67: "Heavy freezing rain",
		71: "Slight snow fall",
		73: "Moderate snow fall",
		75: "Heavy snow fall",
		77: "Snow grains",
		80: "Slight rain showers",
		81: "Moderate rain showers",
		82: "Violent rain showers",
		85: "Slight snow showers",
		86: "Heavy snow showers",
		95: "Thunderstorm",
		96: "Thunderstorm with slight hail",
		99: "Thunderstorm with heavy hail",
	}
	
	if desc, ok := weatherCodes[code]; ok {
		return desc
	}
	return "Unknown"
}

func (c *OpenMeteoClient) weatherCodeToIcon(code int) string {
	// Map weather codes to icon names
	if code == 0 {
		return "01d"
	} else if code <= 3 {
		return "02d"
	} else if code <= 48 {
		return "50d"
	} else if code <= 67 {
		return "10d"
	} else if code <= 77 {
		return "13d"
	} else if code <= 82 {
		return "09d"
	} else if code <= 86 {
		return "13d"
	} else {
		return "11d"
	}
}