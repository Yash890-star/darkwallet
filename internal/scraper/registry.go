package scraper

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"ecom-scraper/internal/model"
)

var (
	ErrNoScraperFound = errors.New("no compatible scraper found for the provided URL")
	ErrInvalidURL     = errors.New("the provided string is not a valid URL")
)

// Registry manages all registered platform scrapers.
type Registry struct {
	scrapers []Scraper
	fallback Scraper
}

// NewRegistry initializes an empty scraper registry.
func NewRegistry() *Registry {
	return &Registry{
		scrapers: make([]Scraper, 0),
	}
}

// Register adds a platform scraper to the registry.
func (r *Registry) Register(s Scraper) {
	r.scrapers = append(r.scrapers, s)
}

// SetFallback sets an optional fallback scraper (e.g. generic OpenGraph/Schema.org).
func (r *Registry) SetFallback(s Scraper) {
	r.fallback = s
}

// FindScraper returns the appropriate scraper and parsed URL for a given target URL string.
func (r *Registry) FindScraper(rawURL string) (Scraper, *url.URL, error) {
	cleanURL := strings.TrimSpace(rawURL)
	if cleanURL == "" {
		return nil, nil, errors.New("URL cannot be empty")
	}

	if !strings.HasPrefix(cleanURL, "http://") && !strings.HasPrefix(cleanURL, "https://") {
		cleanURL = "https://" + cleanURL
	}

	parsed, err := url.Parse(cleanURL)
	if err != nil || parsed.Host == "" {
		return nil, nil, fmt.Errorf("%w: %s", ErrInvalidURL, rawURL)
	}

	for _, s := range r.scrapers {
		if s.CanHandle(parsed) {
			return s, parsed, nil
		}
	}

	if r.fallback != nil {
		return r.fallback, parsed, nil
	}

	return nil, parsed, fmt.Errorf("%w: %s", ErrNoScraperFound, parsed.Host)
}

// ListPlatforms returns information about all registered scrapers.
func (r *Registry) ListPlatforms() []model.PlatformInfo {
	res := make([]model.PlatformInfo, 0, len(r.scrapers))
	for _, s := range r.scrapers {
		res = append(res, model.PlatformInfo{
			ID:          s.ID(),
			Name:        s.PlatformName(),
			Domains:     s.Domains(),
			Description: fmt.Sprintf("Scraper for %s", s.PlatformName()),
		})
	}
	return res
}
