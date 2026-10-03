package scraper

import (
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"

	"ecom-scraper/internal/model"
)

const sampleAmazonHTML = `
<!DOCTYPE html>
<html>
<head>
  <title>Echo Dot (5th Gen) | Smart speaker with bigger sound : Amazon.in</title>
  <meta property="og:image" content="https://m.media-amazon.com/images/I/71C8O0m+T-L._SL1500_.jpg">
</head>
<body>
  <div id="titleSection">
    <h1 id="title">
      <span id="productTitle">Echo Dot (5th Gen) | Smart speaker with bigger sound, motion detection (Black)</span>
    </h1>
  </div>

  <div id="corePriceDisplay_desktop_feature_div">
    <span class="a-price aok-align-center reinventPricePriceToPayMargin priceToPay">
      <span class="a-offscreen">₹4,499.00</span>
      <span aria-hidden="true"><span class="a-price-symbol">₹</span><span class="a-price-whole">4,499<span class="a-price-decimal">.</span></span><span class="a-price-fraction">00</span></span>
    </span>
    <span class="a-price a-text-price">
      <span class="a-offscreen">₹5,499.00</span>
    </span>
    <span class="savingsPercentage">-18%</span>
  </div>

  <div id="availability">
    <span class="a-size-medium a-color-success">In stock</span>
  </div>

  <div id="imgTagWrapperId">
    <img id="landingImage" src="https://m.media-amazon.com/images/I/71C8O0m+T-L._SL1500_.jpg" data-old-hires="https://m.media-amazon.com/images/I/71C8O0m+T-L._SL1500_.jpg">
  </div>

  <div id="averageCustomerReviews">
    <span id="acrPopover" title="4.4 out of 5 stars">
      <span class="a-size-base">4.4</span>
    </span>
    <span id="acrCustomerReviewText">8,321 ratings</span>
  </div>
</body>
</html>
`

func TestAmazonHTMLParsing(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(sampleAmazonHTML))
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	scraper := NewAmazonScraper(nil)
	product := &model.Product{
		Platform:  scraper.PlatformName(),
		Currency:  "₹",
		InStock:   true,
		ScrapedAt: time.Now(),
	}

	scraper.extractTitle(doc, product)
	scraper.extractPrice(doc, product)
	scraper.extractOriginalPrice(doc, product)
	scraper.extractAvailability(doc, product)
	scraper.extractImage(doc, product)
	scraper.extractRating(doc, product)
	scraper.finalizeProductData(product)

	if !strings.Contains(product.Title, "Echo Dot (5th Gen)") {
		t.Errorf("expected title to contain Echo Dot, got %q", product.Title)
	}

	if product.CurrentPrice != 4499.00 {
		t.Errorf("expected current price 4499.00, got %v", product.CurrentPrice)
	}

	if product.OriginalPrice != 5499.00 {
		t.Errorf("expected original price 5499.00, got %v", product.OriginalPrice)
	}

	if product.Discount != "-18%" {
		t.Errorf("expected discount -18%%, got %q", product.Discount)
	}

	if !product.InStock {
		t.Errorf("expected in stock to be true")
	}

	if product.Rating != "4.4 out of 5 stars" {
		t.Errorf("expected rating '4.4 out of 5 stars', got %q", product.Rating)
	}

	if product.ReviewCount != "8,321 ratings" {
		t.Errorf("expected review count '8,321 ratings', got %q", product.ReviewCount)
	}

	if product.FormattedPrice != "₹4,499" {
		t.Errorf("expected formatted price ₹4,499, got %q", product.FormattedPrice)
	}
}
