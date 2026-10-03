package scraper

import (
	"context"
	"net/url"
	"testing"

	"ecom-scraper/internal/model"
)

// MockScraper for testing registry mechanics
type MockScraper struct {
	id       string
	name     string
	domain   string
	called   bool
}

func (m *MockScraper) ID() string           { return m.id }
func (m *MockScraper) PlatformName() string { return m.name }
func (m *MockScraper) Domains() []string    { return []string{m.domain} }
func (m *MockScraper) CanHandle(u *url.URL) bool {
	return u.Hostname() == m.domain
}
func (m *MockScraper) Scrape(ctx context.Context, targetURL string) (*model.Product, error) {
	m.called = true
	return &model.Product{
		Platform: m.name,
		URL:      targetURL,
	}, nil
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	amazonScraper := NewAmazonScraper(nil)
	registry.Register(amazonScraper)

	mockFlipkart := &MockScraper{id: "flipkart", name: "Flipkart", domain: "www.flipkart.com"}
	registry.Register(mockFlipkart)

	t.Run("Find Amazon India Scraper", func(t *testing.T) {
		s, parsed, err := registry.FindScraper("https://www.amazon.in/dp/B0CXF335N4")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.ID() != "amazon_in" {
			t.Errorf("expected amazon_in, got %s", s.ID())
		}
		if parsed.Host != "www.amazon.in" {
			t.Errorf("expected host www.amazon.in, got %s", parsed.Host)
		}
	})

	t.Run("Find Mock Flipkart Scraper", func(t *testing.T) {
		s, _, err := registry.FindScraper("https://www.flipkart.com/item/123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.ID() != "flipkart" {
			t.Errorf("expected flipkart, got %s", s.ID())
		}
	})

	t.Run("Fallback Scraper on Unknown Domain", func(t *testing.T) {
		fallback := &MockScraper{id: "fallback", name: "Fallback", domain: "*"}
		registry.SetFallback(fallback)

		s, _, err := registry.FindScraper("https://example.com/product/xyz")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.ID() != "fallback" {
			t.Errorf("expected fallback, got %s", s.ID())
		}
	})

	t.Run("List registered platforms", func(t *testing.T) {
		platforms := registry.ListPlatforms()
		if len(platforms) != 2 {
			t.Errorf("expected 2 platforms, got %d", len(platforms))
		}
	})
}

func TestAmazonCanonicalizeURL(t *testing.T) {
	s := NewAmazonScraper(nil)

	tests := []struct {
		input       string
		expectedURL string
		expectedASIN string
	}{
		{
			input:       "https://www.amazon.in/Echo-Dot-5th-Gen-Smart-Speaker/dp/B0CXF335N4/ref=sr_1_1?keywords=echo",
			expectedURL: "https://www.amazon.in/dp/B0CXF335N4",
			expectedASIN: "B0CXF335N4",
		},
		{
			input:       "https://www.amazon.in/gp/product/B0B4F2TTTS",
			expectedURL: "https://www.amazon.in/dp/B0B4F2TTTS",
			expectedASIN: "B0B4F2TTTS",
		},
	}

	for _, tt := range tests {
		u, asin := s.canonicalizeURL(tt.input)
		if u != tt.expectedURL {
			t.Errorf("canonicalizeURL(%q) url = %q; want %q", tt.input, u, tt.expectedURL)
		}
		if asin != tt.expectedASIN {
			t.Errorf("canonicalizeURL(%q) asin = %q; want %q", tt.input, asin, tt.expectedASIN)
		}
	}
}
