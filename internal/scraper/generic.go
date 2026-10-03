package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"ecom-scraper/internal/model"
)

// GenericScraper attempts to scrape generic e-commerce platforms using OpenGraph and JSON-LD metadata.
type GenericScraper struct {
	client *http.Client
}

func NewGenericScraper(client *http.Client) *GenericScraper {
	if client == nil {
		client = NewHTTPClient(15 * time.Second)
	}
	return &GenericScraper{
		client: client,
	}
}

func (s *GenericScraper) ID() string {
	return "generic"
}

func (s *GenericScraper) PlatformName() string {
	return "Generic Web / OpenGraph"
}

func (s *GenericScraper) Domains() []string {
	return []string{"*"}
}

func (s *GenericScraper) CanHandle(u *url.URL) bool {
	return true
}

func (s *GenericScraper) Scrape(ctx context.Context, targetURL string) (*model.Product, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	SetBrowserHeaders(req, targetURL)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("target site returned HTTP status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	product := &model.Product{
		Platform:  resp.Request.URL.Host,
		URL:       targetURL,
		Currency:  "₹",
		InStock:   true,
		ScrapedAt: time.Now().UTC(),
	}

	// 1. JSON-LD Check
	doc.Find(`script[type="application/ld+json"]`).Each(func(_ int, sel *goquery.Selection) {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(sel.Text()), &raw); err != nil {
			return
		}

		schemaType, _ := raw["@type"].(string)
		if schemaType == "Product" {
			if name, ok := raw["name"].(string); ok && product.Title == "" {
				product.Title = CleanText(name)
			}
			if img, ok := raw["image"].(string); ok && product.ImageURL == "" {
				product.ImageURL = img
			}
			if offers, ok := raw["offers"].(map[string]interface{}); ok {
				if priceStr, ok := offers["price"].(string); ok {
					if price, err := CleanPriceString(priceStr); err == nil && price > 0 {
						product.CurrentPrice = price
					}
				} else if priceNum, ok := offers["price"].(float64); ok && priceNum > 0 {
					product.CurrentPrice = priceNum
				}
				if curr, ok := offers["priceCurrency"].(string); ok && curr != "" {
					product.Currency = curr
				}
			}
		}
	})

	// 2. OpenGraph Fallbacks
	if product.Title == "" {
		ogTitle, exists := doc.Find(`meta[property="og:title"]`).Attr("content")
		if exists && ogTitle != "" {
			product.Title = CleanText(ogTitle)
		} else {
			product.Title = CleanText(doc.Find("title").Text())
		}
	}

	if product.ImageURL == "" {
		ogImage, exists := doc.Find(`meta[property="og:image"]`).Attr("content")
		if exists && ogImage != "" {
			product.ImageURL = ogImage
		}
	}

	if product.CurrentPrice == 0 {
		ogPrice, exists := doc.Find(`meta[property="product:price:amount"], meta[property="og:price:amount"]`).Attr("content")
		if exists && ogPrice != "" {
			if price, err := CleanPriceString(ogPrice); err == nil {
				product.CurrentPrice = price
			}
		}
	}

	if product.CurrentPrice > 0 {
		product.FormattedPrice = fmt.Sprintf("%s%.2f", product.Currency, product.CurrentPrice)
	}

	if product.Title == "" && product.CurrentPrice == 0 {
		return nil, errors.New("unable to extract product or price from URL")
	}

	return product, nil
}
