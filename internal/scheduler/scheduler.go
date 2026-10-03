package scheduler

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"ecom-scraper/internal/model"
	"ecom-scraper/internal/scraper"
	"ecom-scraper/internal/storage"
)

type SupportedInterval = model.SupportedInterval

var (
	Intervals     = model.Intervals
	ParseInterval = model.ParseInterval
)

type Scheduler struct {
	store    storage.Storage
	registry *scraper.Registry
	stopChan chan struct{}
	wg       sync.WaitGroup
	interval time.Duration
}

func NewScheduler(store storage.Storage, registry *scraper.Registry) *Scheduler {
	return &Scheduler{
		store:    store,
		registry: registry,
		stopChan: make(chan struct{}),
		interval: 30 * time.Second, // check for due jobs every 30s
	}
}

// Start launches the background scheduler loop.
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.runLoop()
	log.Printf("⏱️ Price Tracker Scheduler started (polling every %s)", s.interval)
}

// Stop gracefully shuts down the background scheduler.
func (s *Scheduler) Stop() {
	close(s.stopChan)
	s.wg.Wait()
	log.Printf("⏱️ Price Tracker Scheduler stopped")
}

func (s *Scheduler) runLoop() {
	defer s.wg.Done()

	// On server start, evaluate every tracked product:
	// If (now - latest scan) >= configured cron interval, run fresh scan!
	s.CheckStartupScans()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkDueJobs()
		case <-s.stopChan:
			return
		}
	}
}

// CheckStartupScans inspects every product with a configured cron interval.
// If the difference between now and the latest scan >= configured cron interval
// (or if no scan exists yet), it triggers a fresh scan immediately.
func (s *Scheduler) CheckStartupScans() {
	now := time.Now().UTC()
	jobs, err := s.store.ListTrackedJobs()
	if err != nil {
		log.Printf("scheduler [startup]: error listing tracked jobs: %v", err)
		return
	}

	if len(jobs) == 0 {
		log.Printf("scheduler [startup]: no tracked products found in database")
		return
	}

	log.Printf("scheduler [startup]: checking %d tracked product(s) for overdue scans...", len(jobs))

	for _, job := range jobs {
		if job.Status != "active" {
			log.Printf("scheduler [startup]: product ID %d (%q) is paused. Skipping startup scan.", job.ProductID, job.Product.Title)
			continue
		}

		dur, label, _, err := ParseInterval(job.Interval)
		if err != nil {
			dur = 24 * time.Hour
			label = "Every 24 hrs"
		}

		latestScan, err := s.store.GetLatestScanTime(job.ProductID)
		if err != nil || latestScan.IsZero() {
			if job.LastRunAt != nil {
				latestScan = *job.LastRunAt
			} else if !job.Product.ScrapedAt.IsZero() {
				latestScan = job.Product.ScrapedAt
			}
		}

		var shouldScan bool
		var diff time.Duration

		if latestScan.IsZero() {
			shouldScan = true
			log.Printf("scheduler [startup]: product ID %d (%q) has no recorded scan history. Triggering fresh scan.", job.ProductID, job.Product.Title)
		} else {
			diff = now.Sub(latestScan)
			if diff >= dur {
				shouldScan = true
				log.Printf("scheduler [startup]: product ID %d (%q) is overdue (last scanned %s ago >= interval: %s [%s]). Triggering fresh scan...",
					job.ProductID, job.Product.Title, diff.Round(time.Second), dur, label)
			} else {
				nextIn := dur - diff
				log.Printf("scheduler [startup]: product ID %d (%q) is fresh (last scanned %s ago < interval: %s [%s]. Next scan in %s).",
					job.ProductID, job.Product.Title, diff.Round(time.Second), dur, label, nextIn.Round(time.Second))
			}
		}

		if shouldScan {
			s.executeJob(job)
		}
	}
}

func (s *Scheduler) checkDueJobs() {
	now := time.Now().UTC()
	jobs, err := s.store.GetDueTrackedJobs(now)
	if err != nil {
		log.Printf("scheduler: error fetching due jobs: %v", err)
		return
	}

	if len(jobs) == 0 {
		return
	}

	log.Printf("scheduler: found %d due tracking jobs to execute", len(jobs))
	for _, job := range jobs {
		s.executeJob(job)
	}
}

func (s *Scheduler) executeJob(job model.TrackedJob) {
	dur, _, _, err := ParseInterval(job.Interval)
	if err != nil {
		dur = 24 * time.Hour
	}

	targetURL := job.Product.URL
	sc, _, err := s.registry.FindScraper(targetURL)
	if err != nil {
		log.Printf("scheduler [job #%d]: failed to find scraper for URL %s: %v", job.ID, targetURL, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	log.Printf("scheduler [job #%d]: scraping %s...", job.ID, targetURL)
	prod, err := sc.Scrape(ctx, targetURL)
	now := time.Now().UTC()
	nextRun := now.Add(dur)

	if err != nil {
		log.Printf("scheduler [job #%d]: scrape failed: %v", job.ID, err)
		// Still advance next run so failing jobs don't thrash in an endless retry loop
		_ = s.store.UpdateJobRun(job.ID, now, nextRun)
		return
	}

	// Update product and record price snapshot
	prod.ID = job.ProductID
	_ = s.store.RecordPriceSnapshot(job.ProductID, prod)
	_ = s.store.UpdateJobRun(job.ID, now, nextRun)

	log.Printf("scheduler [job #%d]: successfully recorded price %s%v for %q. Next run: %s",
		job.ID, prod.Currency, prod.CurrentPrice, prod.Title, nextRun.Format("2006-01-02 15:04:05 UTC"))
}

// RunJobNow manually triggers an immediate execution of a tracked job.
func (s *Scheduler) RunJobNow(jobID int64) (*model.Product, error) {
	jobs, err := s.store.ListTrackedJobs()
	if err != nil {
		return nil, err
	}

	var targetJob *model.TrackedJob
	for _, j := range jobs {
		if j.ID == jobID {
			targetJob = &j
			break
		}
	}

	if targetJob == nil || targetJob.Product == nil {
		return nil, errors.New("tracked job not found")
	}

	sc, _, err := s.registry.FindScraper(targetJob.Product.URL)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	prod, err := sc.Scrape(ctx, targetJob.Product.URL)
	if err != nil {
		return nil, err
	}

	dur, _, _, err := ParseInterval(targetJob.Interval)
	if err != nil {
		dur = 24 * time.Hour
	}

	now := time.Now().UTC()
	nextRun := now.Add(dur)

	_ = s.store.RecordPriceSnapshot(targetJob.ProductID, prod)
	_ = s.store.UpdateJobRun(targetJob.ID, now, nextRun)

	return prod, nil
}
