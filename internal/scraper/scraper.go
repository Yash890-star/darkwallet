package scraper

import (
	"context"
	"net/url"

	"ecom-scraper/internal/model"
)

// Scraper defines the behavior every e-commerce platform scraper must implement.
type Scraper interface {
	// ID returns the unique identifier for this platform scraper (e.g. "amazon_in").
	ID() string
	// PlatformName returns the human-friendly name (e.g. "Amazon India").
	PlatformName() string
	// CanHandle returns true if this scraper knows how to parse the given URL.
	CanHandle(u *url.URL) bool
	// Domains returns the supported domain names.
	Domains() []string
	// Scrape fetches and extracts the product information.
	Scrape(ctx context.Context, targetURL string) (*model.Product, error)
}
