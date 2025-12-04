# Weather Data Aggregator Service

A robust, production-ready weather data aggregation service built with Go and Fiber. This service fetches weather data from multiple public APIs, aggregates the information, and exposes it through a REST API.

## Features

- **Multi-source aggregation**: Fetches data from OpenWeatherMap, Open-Meteo (and optionally WeatherAPI.com)
- **Scheduled updates**: Automatic data refresh every 15 minutes (configurable)
- **Intelligent caching**: In-memory cache with configurable TTL
- **Resilient design**: Retry logic, circuit breakers, and graceful degradation
- **RESTful API**: Clean, consistent API endpoints
- **Production-ready**: Structured logging, error handling, metrics

## Architecture

```
┌─────────────────┐
│   HTTP Client   │
│  (Fiber v2)     │
└────────┬────────┘
         │
┌────────▼────────┐
│    Handlers     │
│   (API Layer)   │
└────────┬────────┘
         │
┌────────▼────────┐
│   Aggregator    │
│  (Business Logic)│
└────────┬────────┘
         │
┌────────▼────────┐
│ Weather Clients │
│   (API Clients) │
└────────┬────────┘
         │
┌────────▼────────┐
│   Cache Layer   │
└─────────────────┘
```

## Prerequisites

- Go 1.21 or higher
- API keys for:
  - [OpenWeatherMap](https://openweathermap.org/api) (optional but recommended)
  - [WeatherAPI.com](https://www.weatherapi.com/) (optional)

## Installation

1. Clone the repository:
```bash
git clone https://github.com/bobby-s-dev/weather-aggregator.git
cd weather-aggregator
```

2. Copy the environment file:
```bash
cp .env.example .env
```

3. Edit `.env` with your API keys:
```bash
# Get your API keys from:
# - OpenWeatherMap: https://openweathermap.org/api
# - WeatherAPI: https://www.weatherapi.com/
OPENWEATHER_API_KEY=your_key_here
WEATHERAPI_API_KEY=your_key_here
```

4. Install dependencies:
```bash
go mod download
```

5. Build and run:
```bash
go build -o weather-aggregator ./cmd/server
./weather-aggregator
```

Or run directly:
```bash
go run ./cmd/server
```

## Quick Start with Docker

```bash
# Build the Docker image
docker build -t weather-aggregator .

# Run with environment variables
docker run -p 8080:8080 \
  -e OPENWEATHER_API_KEY=your_key \
  -e WEATHERAPI_API_KEY=your_key \
  weather-aggregator
```

## Configuration

The service is configured via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `FIBER_PORT` | Port for the HTTP server | `8080` |
| `OPENWEATHER_API_KEY` | API key for OpenWeatherMap | - |
| `WEATHERAPI_API_KEY` | API key for WeatherAPI.com | - |
| `FETCH_INTERVAL` | Interval for scheduled fetches | `15m` |
| `DEFAULT_CITIES` | Comma-separated list of cities | `Prague,London,NewYork` |
| `CACHE_DURATION` | Cache TTL for weather data | `10m` |
| `MAX_RETRIES` | Maximum retry attempts for API calls | `3` |
| `CIRCUIT_BREAKER_THRESHOLD` | Failure threshold for circuit breaker | `3` |
| `CIRCUIT_BREAKER_TIMEOUT` | Timeout for circuit breaker reset | `30s` |

## API Endpoints

### Get Current Weather
```http
GET /api/v1/weather/current?city={name}
```

**Example:**
```bash
curl "http://localhost:8080/api/v1/weather/current?city=London"
```

**Response:**
```json
{
  "city": "London",
  "temperature": 15.5,
  "feels_like": 14.8,
  "humidity": 65.5,
  "pressure": 1013.2,
  "wind_speed": 4.2,
  "description": "Partly cloudy",
  "icon": "02d",
  "last_updated": "2024-01-15T14:30:00Z",
  "sources": ["openweathermap", "open-meteo"],
  "confidence": 0.85
}
```

### Get Weather Forecast
```http
GET /api/v1/weather/forecast?city={name}&days={1-7}
```

**Example:**
```bash
curl "http://localhost:8080/api/v1/weather/forecast?city=Prague&days=3"
```

**Response:**
```json
{
  "city": "Prague",
  "days": [
    {
      "date": "2024-01-16T00:00:00Z",
      "max_temp": 12.5,
      "min_temp": 5.2,
      "avg_temp": 8.8,
      "humidity": 70.5,
      "description": "Light rain",
      "icon": "10d",
      "precipitation": 2.5
    }
  ],
  "last_updated": "2024-01-15T14:30:00Z",
  "sources": ["openweathermap", "open-meteo"]
}
```

### Health Check
```http
GET /api/v1/health
```

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T14:30:00Z",
  "last_fetch": "2024-01-15T14:30:00Z",
  "uptime": "5m30s",
  "stats": {
    "success_count": 45,
    "failure_count": 2,
    "cities_stored": 5,
    "cache_stats": {
      "current_weather_items": 5,
      "forecast_items": 15,
      "max_size": 1000
    }
  }
}
```

### Metrics
```http
GET /api/v1/metrics
```

### Available Cities
```http
GET /api/v1/cities
```

## Project Structure

```
weather-aggregator/
├── cmd/server/main.go          # Application entry point
├── internal/
│   ├── api/                    # HTTP handlers and routes
│   ├── config/                 # Configuration loading
│   ├── models/                 # Data structures
│   ├── scheduler/              # Scheduled task runner
│   ├── services/               # Business logic (aggregator, cache)
│   └── utils/                  # Utility functions
├── pkg/client/                 # Weather API clients
├── .env.example               # Example environment variables
├── go.mod                     # Go module definition
├── Dockerfile                 # Docker configuration
├── docker-compose.yml         # Docker Compose setup
└── README.md                  # This file
```

## Design Decisions

### 1. Concurrent API Calls
- Weather data is fetched from multiple sources concurrently
- Uses goroutines and wait groups for parallel execution
- Results are aggregated for higher accuracy

### 2. Resilience Features
- **Exponential Backoff**: Retry failed API calls with increasing delays
- **Circuit Breaker**: Prevents cascading failures when APIs are down
- **Graceful Degradation**: Returns partial results if some sources fail

### 3. Caching Strategy
- Two-level caching: in-memory cache + aggregated results
- Configurable TTL and maximum size
- Automatic cleanup of expired entries

### 4. Data Aggregation
- Averages temperature, humidity, pressure, etc. from multiple sources
- Calculates confidence score based on data consistency
- Selects most common weather description

## Monitoring and Observability

- Structured logging with Zap
- Health check endpoint with service metrics
- Request ID tracing for debugging
- Error handling middleware

## Development

### Running Tests
```bash
go test ./... -v
```

### Code Formatting
```bash
go fmt ./...
```

### Building for Production
```bash
# Build with optimizations
go build -ldflags="-s -w" -o weather-aggregator ./cmd/server

# Compress binary (optional)
upx --best weather-aggregator
```

## Deployment

### Docker Compose
```yaml
version: '3.8'
services:
  weather-aggregator:
    build: .
    ports:
      - "8080:8080"
    environment:
      - FIBER_PORT=8080
      - OPENWEATHER_API_KEY=${OPENWEATHER_API_KEY}
      - FETCH_INTERVAL=15m
      - DEFAULT_CITIES=Prague,London,NewYork,Tokyo,Sydney
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/api/v1/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```

### Kubernetes Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: weather-aggregator
spec:
  replicas: 3
  selector:
    matchLabels:
      app: weather-aggregator
  template:
    metadata:
      labels:
        app: weather-aggregator
    spec:
      containers:
      - name: weather-aggregator
        image: weather-aggregator:latest
        ports:
        - containerPort: 8080
        env:
        - name: OPENWEATHER_API_KEY
          valueFrom:
            secretKeyRef:
              name: weather-secrets
              key: openweather-api-key
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /api/v1/health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /api/v1/health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: weather-aggregator
spec:
  selector:
    app: weather-aggregator
  ports:
  - port: 80
    targetPort: 8080
  type: LoadBalancer
```

## Production Considerations

1. **Rate Limiting**: Implement rate limiting for external API calls
2. **Persistent Storage**: Add database support for historical data
3. **Monitoring**: Integrate with Prometheus and Grafana
4. **Load Balancing**: Deploy multiple instances behind a load balancer
5. **Secrets Management**: Use Kubernetes secrets or AWS Secrets Manager

## Troubleshooting

### Common Issues

1. **No API Keys**: Service works with Open-Meteo without API keys, but OpenWeatherMap requires one
2. **City Not Found**: Ensure city names match API expectations (e.g., "NewYork" not "New York")
3. **Rate Limiting**: Check API provider limits and adjust `FETCH_INTERVAL`

### Logs
Check application logs for detailed error messages:
```bash
# View structured logs
docker logs weather-aggregator

# Follow logs
docker logs -f weather-aggregator
```

### Debug Mode
Run with debug logging:
```bash
LOG_LEVEL=debug ./weather-aggregator
```

## Performance

- Average response time: < 100ms (cached)
- Memory usage: ~50MB per instance
- Concurrent connections: 1000+ (Fiber optimized)
- Cache hit rate: > 90% (with proper TTL)

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

MIT License

## Support

For issues and feature requests, please use the [GitHub Issues](https://github.com/bobby-s-dev/weather-aggregator/issues) page.

## Acknowledgments

- [Fiber](https://gofiber.io/) - Fast HTTP framework for Go
- [OpenWeatherMap](https://openweathermap.org/) - Weather API
- [Open-Meteo](https://open-meteo.com/) - Free weather API
- [WeatherAPI.com](https://www.weatherapi.com/) - Weather API

---

**Note**: This is a demonstration project. For production use, consider:
- Adding authentication/authorization
- Implementing rate limiting
- Adding database persistence
- Setting up monitoring and alerting
- Implementing proper secret management

## Download

You can download this README.md file directly:

```bash
# Using curl
curl -O https://raw.githubusercontent.com/bobby-s-dev/weather-aggregator/main/README.md

# Or using wget
wget https://raw.githubusercontent.com/bobby-s-dev/weather-aggregator/main/README.md
```

For the complete project code, clone the repository:
```bash
git clone https://github.com/bobby-s-dev/weather-aggregator.git
```

---

*Last Updated: December 2025*