package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"ecom-scraper/internal/model"
)

var (
	asinRegex = regexp.MustCompile(`/(?:dp|gp/product|ASIN)/([A-Z0-9]{10})`)
)

// AmazonScraper implements Scraper specifically for Amazon (specialized for Amazon India).
type AmazonScraper struct {
	client *http.Client
}

// NewAmazonScraper constructs an Amazon scraper with a dedicated HTTP client.
func NewAmazonScraper(client *http.Client) *AmazonScraper {
	if client == nil {
		client = NewHTTPClient(15 * time.Second)
	}
	return &AmazonScraper{
		client: client,
	}
}

func (s *AmazonScraper) ID() string {
	return "amazon_in"
}

func (s *AmazonScraper) PlatformName() string {
	return "Amazon India"
}

func (s *AmazonScraper) Domains() []string {
	return []string{
		"amazon.in",
		"www.amazon.in",
		"amzn.in",
		"amzn.to",
		"amazon.com",
	}
}

func (s *AmazonScraper) CanHandle(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	for _, d := range s.Domains() {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// Scrape extracts product price and metadata from an Amazon URL.
func (s *AmazonScraper) Scrape(ctx context.Context, targetURL string) (*model.Product, error) {
	canonicalURL, asin := s.canonicalizeURL(targetURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, canonicalURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	SetBrowserHeaders(req, canonicalURL)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network request to Amazon failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("amazon returned HTTP %d: access restricted (anti-bot triggered)", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("amazon returned unexpected status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	htmlContent := string(bodyBytes)

	// Check for Amazon Captcha / Bot Challenge
	if s.isCaptchaPage(htmlContent) {
		return nil, errors.New("amazon anti-bot captcha challenge encountered. Please retry in a few moments")
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	product := &model.Product{
		Platform:  s.PlatformName(),
		URL:       canonicalURL,
		ASIN:      asin,
		Currency:  "₹",
		InStock:   true,
		ScrapedAt: time.Now().UTC(),
	}

	// 1. Try extracting structured JSON-LD Schema (very reliable when present)
	s.extractFromJSONLD(doc, product)

	// 2. Extract Title (DOM Fallback)
	if product.Title == "" {
		s.extractTitle(doc, product)
	}

	// 3. Extract Current Price (DOM Fallback)
	if product.CurrentPrice == 0 {
		s.extractPrice(doc, product)
	}

	// 4. Extract Original Price / MRP
	s.extractOriginalPrice(doc, product)

	// 5. Extract Availability
	if product.Availability == "" {
		s.extractAvailability(doc, product)
	}

	// 6. Extract Image
	if product.ImageURL == "" {
		s.extractImage(doc, product)
	}

	// 7. Extract Rating & Review Count
	s.extractRating(doc, product)

	// 8. Compute or format discount and formatted prices
	s.finalizeProductData(product)

	if product.CurrentPrice == 0 && product.Title == "" {
		return nil, errors.New("unable to extract product information; the product page structure may have changed or the item is unavailable")
	}

	return product, nil
}

// canonicalizeURL strips tracking parameters and isolates the ASIN for a cleaner URL.
func (s *AmazonScraper) canonicalizeURL(rawURL string) (string, string) {
	matches := asinRegex.FindStringSubmatch(rawURL)
	if len(matches) > 1 {
		asin := matches[1]
		if strings.Contains(rawURL, "amazon.in") || strings.Contains(rawURL, "amzn.in") {
			return fmt.Sprintf("https://www.amazon.in/dp/%s", asin), asin
		}
		return fmt.Sprintf("https://www.amazon.com/dp/%s", asin), asin
	}
	return rawURL, ""
}

func (s *AmazonScraper) isCaptchaPage(html string) bool {
	lower := strings.ToLower(html)
	return strings.Contains(lower, "validatecaptcha") ||
		strings.Contains(lower, "enter the characters you see below") ||
		strings.Contains(lower, "sorry, we just need to make sure you're not a robot") ||
		strings.Contains(lower, "<title>robot check</title>")
}

func (s *AmazonScraper) extractFromJSONLD(doc *goquery.Document, product *model.Product) {
	doc.Find(`script[type="application/ld+json"]`).Each(func(_ int, sel *goquery.Selection) {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(sel.Text()), &raw); err != nil {
			return
		}

		schemaType, _ := raw["@type"].(string)
		if schemaType != "Product" {
			return
		}

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
				if curr == "INR" {
					product.Currency = "₹"
				} else {
					product.Currency = curr
				}
			}

			if avail, ok := offers["availability"].(string); ok {
				if strings.Contains(avail, "InStock") {
					product.InStock = true
					product.Availability = "In Stock"
				} else if strings.Contains(avail, "OutOfStock") {
					product.InStock = false
					product.Availability = "Currently unavailable"
				}
			}
		}
	})
}

func (s *AmazonScraper) extractTitle(doc *goquery.Document, product *model.Product) {
	selectors := []string{
		"#productTitle",
		"#title",
		"h1.a-size-large",
		"span#productTitle",
		`meta[property="og:title"]`,
	}

	for _, sel := range selectors {
		var text string
		if strings.HasPrefix(sel, "meta") {
			text, _ = doc.Find(sel).Attr("content")
		} else {
			text = doc.Find(sel).First().Text()
		}

		text = CleanText(text)
		if text != "" {
			// Amazon titles often end with ": Amazon.in: ..."
			if idx := strings.Index(text, ": Amazon.in"); idx != -1 {
				text = strings.TrimSpace(text[:idx])
			}
			product.Title = text
			return
		}
	}
}

func (s *AmazonScraper) extractPrice(doc *goquery.Document, product *model.Product) {
	// 1. Check offscreen prices inside core price display
	priceSelectors := []string{
		"#corePriceDisplay_desktop_feature_div span.a-price:not(.a-text-price) span.a-offscreen",
		"#corePrice_desktop span.a-price:not(.a-text-price) span.a-offscreen",
		"#apex_desktop span.a-price:not(.a-text-price) span.a-offscreen",
		"#corePrice_feature_div span.a-price:not(.a-text-price) span.a-offscreen",
		"#priceblock_ourprice",
		"#priceblock_dealprice",
		"#priceblock_saleprice",
		"span.priceBlockDealPriceString",
		"#sns-base-price",
		"span.a-price:not(.a-text-price) span.a-offscreen",
	}

	for _, sel := range priceSelectors {
		el := doc.Find(sel).First()
		if el.Length() > 0 {
			priceText := el.Text()
			price, err := CleanPriceString(priceText)
			if err == nil && price > 0 {
				product.CurrentPrice = price
				return
			}
		}
	}

	// 2. Fallback: Combine .a-price-whole and .a-price-fraction
	whole := CleanText(doc.Find("#corePriceDisplay_desktop_feature_div .a-price-whole, span.a-price-whole").First().Text())
	fraction := CleanText(doc.Find("#corePriceDisplay_desktop_feature_div .a-price-fraction, span.a-price-fraction").First().Text())
	if whole != "" {
		raw := whole
		if fraction != "" {
			raw += "." + fraction
		}
		price, err := CleanPriceString(raw)
		if err == nil && price > 0 {
			product.CurrentPrice = price
		}
	}
}

func (s *AmazonScraper) extractOriginalPrice(doc *goquery.Document, product *model.Product) {
	mrpSelectors := []string{
		"#corePriceDisplay_desktop_feature_div span.a-price.a-text-price span.a-offscreen",
		"#corePrice_desktop span.a-price.a-text-price span.a-offscreen",
		"span.a-price.a-text-price span.a-offscreen",
		"span.basisPrice span.a-offscreen",
		"span.a-text-strike",
	}

	for _, sel := range mrpSelectors {
		el := doc.Find(sel).First()
		if el.Length() > 0 {
			priceText := el.Text()
			price, err := CleanPriceString(priceText)
			if err == nil && price > 0 && price >= product.CurrentPrice {
				product.OriginalPrice = price
				return
			}
		}
	}
}

func (s *AmazonScraper) extractAvailability(doc *goquery.Document, product *model.Product) {
	availText := CleanText(doc.Find("#availability span").First().Text())
	if availText == "" {
		availText = CleanText(doc.Find("#availability").First().Text())
	}

	if availText != "" {
		product.Availability = availText
		lower := strings.ToLower(availText)
		if strings.Contains(lower, "currently unavailable") || strings.Contains(lower, "out of stock") {
			product.InStock = false
		} else {
			product.InStock = true
		}
	} else {
		// Check out of stock box
		if doc.Find("#outOfStock").Length() > 0 {
			product.Availability = "Currently unavailable"
			product.InStock = false
		} else {
			product.Availability = "Available"
			product.InStock = true
		}
	}
}

func (s *AmazonScraper) extractImage(doc *goquery.Document, product *model.Product) {
	landingImg := doc.Find("#landingImage").First()
	if landingImg.Length() > 0 {
		if hiRes, exists := landingImg.Attr("data-old-hires"); exists && hiRes != "" {
			product.ImageURL = hiRes
			return
		}
		if src, exists := landingImg.Attr("src"); exists && src != "" {
			product.ImageURL = src
			return
		}
	}

	altImg := doc.Find("#imgBlkFront, #main-image").First()
	if altImg.Length() > 0 {
		if src, exists := altImg.Attr("src"); exists && src != "" {
			product.ImageURL = src
			return
		}
	}

	ogImg, exists := doc.Find(`meta[property="og:image"]`).Attr("content")
	if exists && ogImg != "" {
		product.ImageURL = ogImg
	}
}

func (s *AmazonScraper) extractRating(doc *goquery.Document, product *model.Product) {
	ratingSel := doc.Find("#acrPopover, span[data-hook='rating-out-of-text']").First()
	if ratingSel.Length() > 0 {
		ratingText := CleanText(ratingSel.AttrOr("title", ratingSel.Text()))
		if ratingText != "" {
			product.Rating = ratingText
		}
	}

	reviewSel := doc.Find("#acrCustomerReviewText").First()
	if reviewSel.Length() > 0 {
		product.ReviewCount = CleanText(reviewSel.Text())
	}

	// Discount badge
	discountSel := doc.Find(".savingsPercentage, .reinventPriceSavingsPercentageMargin").First()
	if discountSel.Length() > 0 {
		product.Discount = CleanText(discountSel.Text())
	}
}

func (s *AmazonScraper) finalizeProductData(product *model.Product) {
	if product.CurrentPrice > 0 {
		product.FormattedPrice = FormatPriceWithCommas(product.CurrentPrice, product.Currency)
	}

	if product.OriginalPrice > 0 {
		product.FormattedMRP = FormatPriceWithCommas(product.OriginalPrice, product.Currency)
		if product.Discount == "" && product.OriginalPrice > product.CurrentPrice {
			pct := math.Round((1.0 - (product.CurrentPrice / product.OriginalPrice)) * 100.0)
			if pct > 0 {
				product.Discount = fmt.Sprintf("-%d%%", int(pct))
			}
		}
	}
}
