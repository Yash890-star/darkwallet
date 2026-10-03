package scheduler

import (
	"context"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"ecom-scraper/internal/model"
	"ecom-scraper/internal/scraper"
	"ecom-scraper/internal/storage"
)

type mockCronScraper struct {
	scrapedCount int32
}

func (m *mockCronScraper) ID() string           { return "mock_cron" }
func (m *mockCronScraper) PlatformName() string { return "Mock Platform" }
func (m *mockCronScraper) Domains() []string    { return []string{"mock.cron"} }
func (m *mockCronScraper) CanHandle(u *url.URL) bool {
	return u.Hostname() == "mock.cron"
}
func (m *mockCronScraper) Scrape(ctx context.Context, targetURL string) (*model.Product, error) {
	atomic.AddInt32(&m.scrapedCount, 1)
	return &model.Product{
		Platform:       "Mock Platform",
		URL:            targetURL,
		Title:          "Mock Product",
		Currency:       "₹",
		CurrentPrice:   1000,
		FormattedPrice: "₹1,000",
		InStock:        true,
		ScrapedAt:      time.Now(),
	}, nil
}

func TestParseInterval(t *testing.T) {
	tests := []struct {
		input            string
		expectedDuration time.Duration
		expectedLabel    string
		hasError         bool
	}{
		{
			input:            "every 6hrs",
			expectedDuration: 6 * time.Hour,
			expectedLabel:    "Every 6 hrs",
		},
		{
			input:            "every 12 hrs",
			expectedDuration: 12 * time.Hour,
			expectedLabel:    "Every 12 hrs",
		},
		{
			input:            "every 24 hrs",
			expectedDuration: 24 * time.Hour,
			expectedLabel:    "Every 24 hrs",
		},
		{
			input:            "every 2 days",
			expectedDuration: 48 * time.Hour,
			expectedLabel:    "Every 2 days",
		},
		{
			input:            "every 1 week",
			expectedDuration: 7 * 24 * time.Hour,
			expectedLabel:    "Every 1 week",
		},
		{
			input:            "every 1 month",
			expectedDuration: 30 * 24 * time.Hour,
			expectedLabel:    "Every 1 month",
		},
		{
			input:            "6h",
			expectedDuration: 6 * time.Hour,
			expectedLabel:    "Every 6 hrs",
		},
		{
			input:            "1w",
			expectedDuration: 7 * 24 * time.Hour,
			expectedLabel:    "Every 1 week",
		},
		{
			input:    "invalid_interval",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			dur, label, _, err := ParseInterval(tt.input)
			if (err != nil) != tt.hasError {
				t.Fatalf("ParseInterval(%q) error = %v, want error %v", tt.input, err, tt.hasError)
			}
			if !tt.hasError {
				if dur != tt.expectedDuration {
					t.Errorf("ParseInterval(%q) duration = %v, want %v", tt.input, dur, tt.expectedDuration)
				}
				if label != tt.expectedLabel {
					t.Errorf("ParseInterval(%q) label = %q, want %q", tt.input, label, tt.expectedLabel)
				}
			}
		})
	}
}

func TestCheckStartupScans(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "startup_test.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	mockScraper := &mockCronScraper{}
	registry := scraper.NewRegistry()
	registry.Register(mockScraper)

	// Product A: Last scanned 8 hours ago with "every 6hrs" -> diff (8h) > interval (6h) -> MUST scan
	prodA := &model.Product{
		Platform:     "Mock Platform",
		URL:          "https://mock.cron/product/A",
		Title:        "Product A",
		Currency:     "₹",
		CurrentPrice: 500,
		ScrapedAt:    time.Now().UTC().Add(-8 * time.Hour),
	}
	idA, err := store.UpsertProduct(prodA)
	if err != nil {
		t.Fatalf("UpsertProduct A failed: %v", err)
	}
	_ = store.RecordPriceSnapshot(idA, prodA)
	_, _ = store.UpsertTrackedJob(idA, "every 6hrs", "Every 6 hrs", time.Now().UTC().Add(-2*time.Hour))

	// Product B: Last scanned 1 hour ago with "every 6hrs" -> diff (1h) < interval (6h) -> MUST NOT scan
	prodB := &model.Product{
		Platform:     "Mock Platform",
		URL:          "https://mock.cron/product/B",
		Title:        "Product B",
		Currency:     "₹",
		CurrentPrice: 700,
		ScrapedAt:    time.Now().UTC().Add(-1 * time.Hour),
	}
	idB, err := store.UpsertProduct(prodB)
	if err != nil {
		t.Fatalf("UpsertProduct B failed: %v", err)
	}
	_ = store.RecordPriceSnapshot(idB, prodB)
	_, _ = store.UpsertTrackedJob(idB, "every 6hrs", "Every 6 hrs", time.Now().UTC().Add(5*time.Hour))

	sched := NewScheduler(store, registry)

	// Run startup check
	sched.CheckStartupScans()

	// Only Product A should have been scraped!
	count := atomic.LoadInt32(&mockScraper.scrapedCount)
	if count != 1 {
		t.Errorf("expected exactly 1 product to be rescanned on startup, got %d", count)
	}
}
