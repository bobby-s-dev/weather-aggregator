package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"weather-aggregator/internal/models"
	"go.uber.org/zap"
)

type OpenWeatherClient struct {
	*BaseClient
	apiKey string
	baseURL string
}

type OpenWeatherCurrentResponse struct {
	Coord struct {
		Lon float64 `json:"lon"`
		Lat float64 `json:"lat"`
	} `json:"coord"`
	Weather []struct {
		ID          int    `json:"id"`
		Main        string `json:"main"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
	} `json:"weather"`
	Main struct {
		Temp      float64 `json:"temp"`
		FeelsLike float64 `json:"feels_like"`
		TempMin   float64 `json:"temp_min"`
		TempMax   float64 `json:"temp_max"`
		Pressure  float64 `json:"pressure"`
		Humidity  float64 `json:"humidity"`
	} `json:"main"`
	Wind struct {
		Speed float64 `json:"speed"`
		Deg   float64 `json:"deg"`
	} `json:"wind"`
	Clouds struct {
		All int `json:"all"`
	} `json:"clouds"`
	Dt  int64  `json:"dt"`
	Sys struct {
		Country string `json:"country"`
		Sunrise int64  `json:"sunrise"`
		Sunset  int64  `json:"sunset"`
	} `json:"sys"`
	Timezone int    `json:"timezone"`
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Cod      int    `json:"cod"`
}

type OpenWeatherForecastResponse struct {
	Cod     string `json:"cod"`
	Message int    `json:"message"`
	Cnt     int    `json:"cnt"`
	List    []struct {
		Dt   int64 `json:"dt"`
		Main struct {
			Temp      float64 `json:"temp"`
			FeelsLike float64 `json:"feels_like"`
			TempMin   float64 `json:"temp_min"`
			TempMax   float64 `json:"temp_max"`
			Pressure  float64 `json:"pressure"`
			SeaLevel  int     `json:"sea_level"`
			GrndLevel int     `json:"grnd_level"`
			Humidity  int     `json:"humidity"`
			TempKf    float64 `json:"temp_kf"`
		} `json:"main"`
		Weather []struct {
			ID          int    `json:"id"`
			Main        string `json:"main"`
			Description string `json:"description"`
			Icon        string `json:"icon"`
		} `json:"weather"`
		Clouds struct {
			All int `json:"all"`
		} `json:"clouds"`
		Wind struct {
			Speed float64 `json:"speed"`
			Deg   float64 `json:"deg"`
			Gust  float64 `json:"gust"`
		} `json:"wind"`
		Visibility int     `json:"visibility"`
		Pop        float64 `json:"pop"`
		Sys        struct {
			Pod string `json:"pod"`
		} `json:"sys"`
		DtTxt string `json:"dt_txt"`
	} `json:"list"`
	City struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Coord struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		} `json:"coord"`
		Country string `json:"country"`
		Population int `json:"population"`
		Timezone int `json:"timezone"`
		Sunrise  int `json:"sunrise"`
		Sunset   int `json:"sunset"`
	} `json:"city"`
}

func NewOpenWeatherClient(apiKey string, config ClientConfig, logger *zap.Logger) *OpenWeatherClient {
	baseClient := NewBaseClient("openweather", config, logger)
	return &OpenWeatherClient{
		BaseClient: baseClient,
		apiKey:     apiKey,
		baseURL:    "https://api.openweathermap.org/data/2.5",
	}
}

func (c *OpenWeatherClient) GetCurrentWeather(ctx context.Context, city string) (*models.CurrentWeather, error) {
	url := fmt.Sprintf("%s/weather?q=%s&appid=%s&units=metric", c.baseURL, city, c.apiKey)
	
	data, err := c.GetWithRetry(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch current weather: %w", err)
	}
	
	var response OpenWeatherCurrentResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	if response.Cod != 200 {
		return nil, fmt.Errorf("API error: %d", response.Cod)
	}
	
	weather := &models.CurrentWeather{
		City:        response.Name,
		Temperature: response.Main.Temp,
		FeelsLike:   response.Main.FeelsLike,
		Humidity:    float64(response.Main.Humidity),
		Pressure:    float64(response.Main.Pressure),
		WindSpeed:   response.Wind.Speed,
		WindDegree:  response.Wind.Deg,
		Description: response.Weather[0].Description,
		Icon:        response.Weather[0].Icon,
		Timestamp:   time.Unix(response.Dt, 0),
		Source:      "openweathermap",
	}
	
	return weather, nil
}

func (c *OpenWeatherClient) GetForecast(ctx context.Context, city string, days int) (*models.WeatherForecast, error) {
	// OpenWeatherMap provides forecast for 5 days with 3-hour intervals
	url := fmt.Sprintf("%s/forecast?q=%s&appid=%s&units=metric&cnt=%d", c.baseURL, city, c.apiKey, days*8)
	
	data, err := c.GetWithRetry(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch forecast: %w", err)
	}
	
	var response OpenWeatherForecastResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse forecast response: %w", err)
	}
	
	if response.Cod != "200" {
		return nil, fmt.Errorf("API error: %s", response.Cod)
	}
	
	// Group forecast by day
	forecastByDay := make(map[string][]OpenWeatherForecastResponse.List)
	for _, item := range response.List {
		date := time.Unix(item.Dt, 0).Format("2006-01-02")
		forecastByDay[date] = append(forecastByDay[date], item)
	}
	
	forecast := &models.WeatherForecast{
		City:     response.City.Name,
		Forecast: make([]models.ForecastDay, 0, days),
		Source:   "openweathermap",
	}
	
	// Calculate daily aggregates
	for dateStr, items := range forecastByDay {
		if len(forecast.Forecast) >= days {
			break
		}
		
		date, _ := time.Parse("2006-01-02", dateStr)
		var dayForecast models.ForecastDay
		dayForecast.Date = date
		
		var totalTemp, maxTemp, minTemp, totalHumidity float64
		maxTemp = -100
		minTemp = 100
		
		for _, item := range items {
			temp := item.Main.Temp
			totalTemp += temp
			totalHumidity += float64(item.Main.Humidity)
			
			if temp > maxTemp {
				maxTemp = temp
			}
			if temp < minTemp {
				minTemp = temp
			}
		}
		
		dayForecast.AvgTemp = totalTemp / float64(len(items))
		dayForecast.MaxTemp = maxTemp
		dayForecast.MinTemp = minTemp
		dayForecast.Humidity = totalHumidity / float64(len(items))
		
		// Use the most common weather description for the day
		if len(items) > 0 && len(items[0].Weather) > 0 {
			dayForecast.Description = items[0].Weather[0].Description
			dayForecast.Icon = items[0].Weather[0].Icon
		}
		
		forecast.Forecast = append(forecast.Forecast, dayForecast)
	}
	
	return forecast, nil
}