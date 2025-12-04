package scheduler

import (
	"context"
	"sync"
	"time"

	"weather-aggregator/internal/services"
	"go.uber.org/zap"
)

type Scheduler struct {
	aggregator     *services.Aggregator
	logger         *zap.Logger
	cities         []string
	interval       time.Duration
	ticker         *time.Ticker
	stop           chan bool
	running        bool
	mu             sync.Mutex
	lastRun        time.Time
	nextRun        time.Time
	skipIfRunning  bool
}

func NewScheduler(aggregator *services.Aggregator, cities []string, interval time.Duration, logger *zap.Logger) *Scheduler {
	return &Scheduler{
		aggregator:    aggregator,
		logger:        logger,
		cities:        cities,
		interval:      interval,
		stop:          make(chan bool),
		skipIfRunning: true,
	}
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	
	s.ticker = time.NewTicker(s.interval)
	s.nextRun = time.Now().Add(s.interval)
	
	s.logger.Info("Scheduler started",
		zap.Duration("interval", s.interval),
		zap.Time("next_run", s.nextRun))
	
	// Run immediately on start
	go s.runFetch()
	
	// Start the scheduler loop
	go s.run()
}

func (s *Scheduler) run() {
	for {
		select {
		case <-s.ticker.C:
			s.nextRun = time.Now().Add(s.interval)
			s.logger.Debug("Scheduler tick", zap.Time("next_run", s.nextRun))
			go s.runFetch()
		case <-s.stop:
			s.ticker.Stop()
			return
		}
	}
}

func (s *Scheduler) runFetch() {
	s.mu.Lock()
	if s.skipIfRunning {
		// Check if already running
		if !s.lastRun.IsZero() && time.Since(s.lastRun) < s.interval {
			s.mu.Unlock()
			s.logger.Debug("Skipping fetch, previous run still within interval")
			return
		}
	}
	s.lastRun = time.Now()
	s.mu.Unlock()
	
	startTime := time.Now()
	s.logger.Info("Starting scheduled weather fetch",
		zap.Time("start_time", startTime),
		zap.Strings("cities", s.cities))
	
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	
	if err := s.aggregator.FetchWeatherData(ctx, s.cities); err != nil {
		s.logger.Error("Scheduled weather fetch failed",
			zap.Error(err),
			zap.Duration("duration", time.Since(startTime)))
	} else {
		s.logger.Info("Scheduled weather fetch completed",
			zap.Duration("duration", time.Since(startTime)))
	}
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if !s.running {
		return
	}
	
	s.logger.Info("Stopping scheduler")
	s.stop <- true
	s.running = false
}

func (s *Scheduler) ForceRun() {
	s.logger.Info("Manually triggering weather fetch")
	go s.runFetch()
}

func (s *Scheduler) GetStatus() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	return map[string]interface{}{
		"running":        s.running,
		"interval":       s.interval.String(),
		"last_run":       s.lastRun,
		"next_run":       s.nextRun,
		"cities":         s.cities,
		"skip_if_running": s.skipIfRunning,
	}
}

func (s *Scheduler) UpdateCities(cities []string) {
	s.mu.Lock()
	s.cities = cities
	s.mu.Unlock()
	
	s.logger.Info("Scheduler cities updated", zap.Strings("cities", cities))
}