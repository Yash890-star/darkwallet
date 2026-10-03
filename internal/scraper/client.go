package scraper

import (
	"crypto/tls"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:124.0) Gecko/20100101 Firefox/124.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36 Edg/122.0.0.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.3 Safari/605.1.15",
}

// NewHTTPClient creates an optimized HTTP client with cookie jar and realistic browser characteristics.
func NewHTTPClient(timeout time.Duration) *http.Client {
	jar, _ := cookiejar.New(nil)

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		DisableKeepAlives: false,
		MaxIdleConns:      100,
		IdleConnTimeout:   90 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   timeout,
	}
}

// GetRandomUserAgent returns a random modern desktop browser User-Agent.
func GetRandomUserAgent() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return userAgents[r.Intn(len(userAgents))]
}

// SetBrowserHeaders decorates an HTTP request to closely mimic a real desktop browser.
func SetBrowserHeaders(req *http.Request, targetURL string) {
	req.Header.Set("User-Agent", GetRandomUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-IN,en-GB;q=0.9,en-US;q=0.8,en;q=0.7,hi;q=0.6")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Sec-Ch-Ua", `"Google Chrome";v="123", "Not:A-Brand";v="8", "Chromium";v="123"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
}

var (
	priceRegex = regexp.MustCompile(`(\d+(?:,\d+)*(?:\.\d+)?)`)
)

// CleanPriceString extracts a float price from raw scraped text like "₹ 1,499.00", "Rs. 799", "Rs. 450.50".
func CleanPriceString(raw string) (float64, error) {
	match := priceRegex.FindString(raw)
	if match == "" {
		return 0, strconv.ErrSyntax
	}

	clean := strings.ReplaceAll(match, ",", "")
	return strconv.ParseFloat(clean, 64)
}

// CleanText trims and compresses whitespace.
func CleanText(raw string) string {
	fields := strings.Fields(raw)
	return strings.Join(fields, " ")
}

// FormatPriceWithCommas formats a numeric price with commas and currency symbol.
func FormatPriceWithCommas(amount float64, currency string) string {
	parts := strings.Split(strconv.FormatFloat(amount, 'f', 2, 64), ".")
	intPart := parts[0]
	decPart := parts[1]

	var result []byte
	n := len(intPart)
	for i, c := range intPart {
		if i > 0 && (n-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}

	if decPart == "00" {
		return fmt.Sprintf("%s%s", currency, string(result))
	}
	return fmt.Sprintf("%s%s.%s", currency, string(result), decPart)
}
