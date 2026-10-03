package model

import "time"

// Product represents scraped product details from an e-commerce platform.
type Product struct {
	ID             int64          `json:"id,omitempty"`
	Platform       string         `json:"platform"`
	URL            string         `json:"url"`
	ASIN           string         `json:"asin,omitempty"`
	Title          string         `json:"title"`
	Currency       string         `json:"currency"`
	CurrentPrice   float64        `json:"current_price"`
	OriginalPrice  float64        `json:"original_price,omitempty"`
	Discount       string         `json:"discount,omitempty"`
	FormattedPrice string         `json:"formatted_price"`
	FormattedMRP   string         `json:"formatted_mrp,omitempty"`
	InStock        bool           `json:"in_stock"`
	Availability   string         `json:"availability"`
	Rating         string         `json:"rating,omitempty"`
	ReviewCount    string         `json:"review_count,omitempty"`
	ImageURL       string         `json:"image_url,omitempty"`
	ScrapedAt      time.Time      `json:"scraped_at"`
	History        []PriceHistory `json:"history,omitempty"`
	TrackedJob     *TrackedJob    `json:"tracked_job,omitempty"`
	IsUserTracked  bool           `json:"is_user_tracked,omitempty"`
	UserInterval   string         `json:"user_interval,omitempty"`
	EffectiveCron  string         `json:"effective_cron,omitempty"`
	TrackedUsers   int            `json:"tracked_users,omitempty"`
}

// UserTrackedProduct represents an item in a specific user's wishlist / tracked list.
type UserTrackedProduct struct {
	ProductID              int64      `json:"product_id"`
	URL                    string     `json:"url"`
	ASIN                   string     `json:"asin,omitempty"`
	Platform               string     `json:"platform"`
	Title                  string     `json:"title"`
	Currency               string     `json:"currency"`
	CurrentPrice           float64    `json:"current_price"`
	FormattedPrice         string     `json:"formatted_price"`
	Discount               string     `json:"discount,omitempty"`
	ImageURL               string     `json:"image_url,omitempty"`
	InStock                bool       `json:"in_stock"`
	UserDesiredInterval    string     `json:"user_desired_interval"`
	EffectiveCronInterval  string     `json:"effective_cron_interval"`
	EffectiveIntervalLabel string     `json:"effective_interval_label"`
	CronStatus             string     `json:"cron_status"`
	NextRunAt              time.Time  `json:"next_run_at"`
	LastRunAt              *time.Time `json:"last_run_at,omitempty"`
	AddedAt                time.Time  `json:"added_at"`
}

// PriceHistory represents a single price point recorded in SQLite.
type PriceHistory struct {
	ID             int64     `json:"id"`
	ProductID      int64     `json:"product_id"`
	CurrentPrice   float64   `json:"current_price"`
	OriginalPrice  float64   `json:"original_price,omitempty"`
	Discount       string    `json:"discount,omitempty"`
	FormattedPrice string    `json:"formatted_price"`
	FormattedMRP   string    `json:"formatted_mrp,omitempty"`
	InStock        bool      `json:"in_stock"`
	Availability   string    `json:"availability"`
	Rating         string    `json:"rating,omitempty"`
	ReviewCount    string    `json:"review_count,omitempty"`
	RecordedAt     time.Time `json:"recorded_at"`
}

// TrackedJob represents a recurring price-check cron job.
type TrackedJob struct {
	ID            int64      `json:"id"`
	ProductID     int64      `json:"product_id"`
	Product       *Product   `json:"product,omitempty"`
	Interval      string     `json:"interval"`       // "6h", "12h", "24h", "2d", "1w", "1m"
	IntervalLabel string     `json:"interval_label"` // "Every 6 hrs", etc.
	Status        string     `json:"status"`         // "active", "paused"
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	NextRunAt     time.Time  `json:"next_run_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// TrackRequest is payload to create or update a recurring price-check job.
type TrackRequest struct {
	URL      string `json:"url"`
	Interval string `json:"interval"` // e.g. "6h", "12h", "24h", "2d", "1w", "1m"
}

// ScrapeRequest represents the payload to scrape a product.
type ScrapeRequest struct {
	URL string `json:"url"`
}

// ScrapeResponse represents the standardized JSON response from the API.
type ScrapeResponse struct {
	Success bool     `json:"success"`
	Data    *Product `json:"data,omitempty"`
	Error   string   `json:"error,omitempty"`
	TookMs  int64    `json:"took_ms"`
}

// TrackResponse represents JSON response for tracking operations.
type TrackResponse struct {
	Success bool        `json:"success"`
	Data    *TrackedJob `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// TrackedListResponse represents the list of all tracked jobs.
type TrackedListResponse struct {
	Success bool         `json:"success"`
	Count   int          `json:"count"`
	Jobs    []TrackedJob `json:"jobs"`
	Error   string       `json:"error,omitempty"`
}

// HistoryResponse represents price history response for graphing.
type HistoryResponse struct {
	Success bool           `json:"success"`
	Product *Product       `json:"product,omitempty"`
	History []PriceHistory `json:"history"`
	Stats   *PriceStats    `json:"stats,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// PriceStats contains analytical summary metrics for price history.
type PriceStats struct {
	LowestPrice  float64 `json:"lowest_price"`
	HighestPrice float64 `json:"highest_price"`
	AveragePrice float64 `json:"average_price"`
	TotalChecks  int     `json:"total_checks"`
	PriceChange  float64 `json:"price_change"` // difference from first check to current
}

// PlatformInfo describes a supported platform.
type PlatformInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Domains     []string `json:"domains"`
	Description string   `json:"description"`
}
