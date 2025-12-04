package models

import (
	"time"
)

type CurrentWeather struct {
	City        string    `json:"city"`
	Temperature float64   `json:"temperature"`
	FeelsLike   float64   `json:"feels_like"`
	Humidity    float64   `json:"humidity"`
	Pressure    float64   `json:"pressure"`
	WindSpeed   float64   `json:"wind_speed"`
	WindDegree  float64   `json:"wind_degree"`
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	Timestamp   time.Time `json:"timestamp"`
	Source      string    `json:"source"`
}

type ForecastDay struct {
	Date        time.Time `json:"date"`
	MaxTemp     float64   `json:"max_temp"`
	MinTemp     float64   `json:"min_temp"`
	AvgTemp     float64   `json:"avg_temp"`
	Humidity    float64   `json:"humidity"`
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	Precipitation float64 `json:"precipitation"`
}

type WeatherForecast struct {
	City     string       `json:"city"`
	Forecast []ForecastDay `json:"forecast"`
	Source   string       `json:"source"`
}

type AggregatedCurrentWeather struct {
	City        string    `json:"city"`
	Temperature float64   `json:"temperature"`
	FeelsLike   float64   `json:"feels_like"`
	Humidity    float64   `json:"humidity"`
	Pressure    float64   `json:"pressure"`
	WindSpeed   float64   `json:"wind_speed"`
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	LastUpdated time.Time `json:"last_updated"`
	Sources     []string  `json:"sources"`
	Confidence  float64   `json:"confidence"`
}

type AggregatedForecast struct {
	City     string        `json:"city"`
	Days     []ForecastDay `json:"days"`
	LastUpdated time.Time  `json:"last_updated"`
	Sources  []string      `json:"sources"`
}

type APIResponse struct {
	Current  *CurrentWeather
	Forecast *WeatherForecast
	Error    error
	Source   string
}

type WeatherData struct {
	City      string
	Current   map[string]*CurrentWeather  // source -> current weather
	Forecasts map[string]*WeatherForecast // source -> forecast
	Timestamp time.Time
}