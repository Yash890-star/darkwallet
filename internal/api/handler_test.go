package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"ecom-scraper/internal/model"
	"ecom-scraper/internal/scheduler"
	"ecom-scraper/internal/scraper"
	"ecom-scraper/internal/storage"
)

type mockTestScraper struct{}

func (m *mockTestScraper) ID() string           { return "test_mock" }
func (m *mockTestScraper) PlatformName() string { return "Test Mock Platform" }
func (m *mockTestScraper) Domains() []string    { return []string{"mock.test"} }
func (m *mockTestScraper) CanHandle(u *url.URL) bool {
	return u.Hostname() == "mock.test"
}
func (m *mockTestScraper) Scrape(ctx context.Context, targetURL string) (*model.Product, error) {
	return &model.Product{
		Platform:       "Test Mock Platform",
		URL:            targetURL,
		Title:          "Mock Smartphone 128GB",
		Currency:       "₹",
		CurrentPrice:   19999.00,
		FormattedPrice: "₹19,999",
		InStock:        true,
		ScrapedAt:      time.Now(),
	}, nil
}

func setupTestServer(t *testing.T) (*Handler, *scraper.Registry, storage.Storage) {
	registry := scraper.NewRegistry()
	registry.Register(&mockTestScraper{})
	dbPath := filepath.Join(t.TempDir(), "api_test.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	sched := scheduler.NewScheduler(store, registry)
	handler := NewHandler(registry, store, sched)
	return handler, registry, store
}

func TestHealthCheck(t *testing.T) {
	handler, _, _ := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	handler.HealthCheck(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res["status"] != "healthy" {
		t.Errorf("expected status healthy, got %v", res["status"])
	}
}

func TestGetPlatforms(t *testing.T) {
	handler, _, _ := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/platforms", nil)
	rr := httptest.NewRecorder()

	handler.GetPlatforms(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var res struct {
		Success   bool                 `json:"success"`
		Platforms []model.PlatformInfo `json:"platforms"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !res.Success || len(res.Platforms) != 1 {
		t.Errorf("expected success with 1 platform, got %v", res)
	}
}

func TestGetIntervals(t *testing.T) {
	handler, _, _ := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/intervals", nil)
	rr := httptest.NewRecorder()

	handler.GetIntervals(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var res struct {
		Success   bool                          `json:"success"`
		Intervals []scheduler.SupportedInterval `json:"intervals"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse intervals: %v", err)
	}
	if len(res.Intervals) != 6 {
		t.Errorf("expected 6 supported intervals, got %d", len(res.Intervals))
	}
}

func TestHandleScrape(t *testing.T) {
	handler, _, _ := setupTestServer(t)

	t.Run("Empty URL returns 400", func(t *testing.T) {
		body, _ := json.Marshal(model.ScrapeRequest{URL: ""})
		req := httptest.NewRequest(http.MethodPost, "/api/scrape", bytes.NewReader(body))
		rr := httptest.NewRecorder()

		handler.HandleScrape(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rr.Code)
		}
	})

	t.Run("Valid URL with Mock Scraper returns 200 with product", func(t *testing.T) {
		body, _ := json.Marshal(model.ScrapeRequest{URL: "https://mock.test/product/123"})
		req := httptest.NewRequest(http.MethodPost, "/api/scrape", bytes.NewReader(body))
		rr := httptest.NewRecorder()

		handler.HandleScrape(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d (body: %s)", rr.Code, rr.Body.String())
		}

		var res model.ScrapeResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if !res.Success || res.Data == nil {
			t.Fatalf("expected success with data")
		}

		if res.Data.CurrentPrice != 19999.00 {
			t.Errorf("expected price 19999.00, got %v", res.Data.CurrentPrice)
		}
	})
}

func TestHandleTrackAndHistory(t *testing.T) {
	handler, _, store := setupTestServer(t)

	// Create user & session for auth
	user, err := store.CreateUser("tester", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	token, err := store.CreateSession(user.ID, time.Hour)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// 1. Schedule a cron job for mock product with auth
	trackPayload, _ := json.Marshal(model.TrackRequest{
		URL:      "https://mock.test/product/123",
		Interval: "every 6hrs",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader(trackPayload))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.HandleTrack(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("HandleTrack failed with code %d: %s", rr.Code, rr.Body.String())
	}

	var trackRes model.TrackResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &trackRes); err != nil {
		t.Fatalf("failed to parse TrackResponse: %v", err)
	}

	if !trackRes.Success || trackRes.Data.IntervalLabel != "Every 6 hrs" {
		t.Errorf("expected success with 'Every 6 hrs', got %v", trackRes)
	}

	// 2. Fetch history for this product
	histReq := httptest.NewRequest(http.MethodGet, "/api/history?url=https://mock.test/product/123", nil)
	histRR := httptest.NewRecorder()
	handler.HandleHistory(histRR, histReq)

	if histRR.Code != http.StatusOK {
		t.Fatalf("HandleHistory failed with code %d: %s", histRR.Code, histRR.Body.String())
	}

	var histRes model.HistoryResponse
	if err := json.Unmarshal(histRR.Body.Bytes(), &histRes); err != nil {
		t.Fatalf("failed to parse HistoryResponse: %v", err)
	}

	if len(histRes.History) == 0 {
		t.Errorf("expected at least 1 history record, got 0")
	}
	if histRes.Stats == nil || histRes.Stats.LowestPrice != 19999.00 {
		t.Errorf("expected lowest price 19999.00 in stats, got %v", histRes.Stats)
	}
}

func TestAuthAndMultiUserAPIs(t *testing.T) {
	handler, _, _ := setupTestServer(t)

	// 1. Register User 1
	regPayload, _ := json.Marshal(model.RegisterRequest{
		Username: "alice",
		Password: "alicepassword",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(regPayload))
	regRR := httptest.NewRecorder()
	handler.HandleRegister(regRR, regReq)

	if regRR.Code != http.StatusOK {
		t.Fatalf("Register failed: %s", regRR.Body.String())
	}

	var authRes1 model.AuthResponse
	_ = json.Unmarshal(regRR.Body.Bytes(), &authRes1)
	if !authRes1.Success || authRes1.Token == "" {
		t.Fatalf("expected token in register response")
	}

	// 2. Register User 2
	regPayload2, _ := json.Marshal(model.RegisterRequest{
		Username: "bob",
		Password: "bobpassword",
	})
	regReq2 := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(regPayload2))
	regRR2 := httptest.NewRecorder()
	handler.HandleRegister(regRR2, regReq2)
	var authRes2 model.AuthResponse
	_ = json.Unmarshal(regRR2.Body.Bytes(), &authRes2)

	// 3. User 1 tracks product with 24 hrs
	track1, _ := json.Marshal(model.TrackRequest{
		URL:      "https://mock.test/product/123",
		Interval: "every 24 hrs",
	})
	tReq1 := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader(track1))
	tReq1.Header.Set("Authorization", "Bearer "+authRes1.Token)
	tRR1 := httptest.NewRecorder()
	handler.HandleTrack(tRR1, tReq1)
	if tRR1.Code != http.StatusOK {
		t.Fatalf("User 1 track failed: %s", tRR1.Body.String())
	}

	// 4. User 2 tracks SAME product with 6 hrs (lowest)
	track2, _ := json.Marshal(model.TrackRequest{
		URL:      "https://mock.test/product/123",
		Interval: "every 6hrs",
	})
	tReq2 := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader(track2))
	tReq2.Header.Set("Authorization", "Bearer "+authRes2.Token)
	tRR2 := httptest.NewRecorder()
	handler.HandleTrack(tRR2, tReq2)
	if tRR2.Code != http.StatusOK {
		t.Fatalf("User 2 track failed: %s", tRR2.Body.String())
	}

	// 5. Query /api/tracked as User 1 -> should see 1 product with effective lowest cron "Every 6 hrs"
	listReq1 := httptest.NewRequest(http.MethodGet, "/api/tracked", nil)
	listReq1.Header.Set("Authorization", "Bearer "+authRes1.Token)
	listRR1 := httptest.NewRecorder()
	handler.HandleListTracked(listRR1, listReq1)

	var list1 struct {
		Success bool                       `json:"success"`
		Count   int                        `json:"count"`
		Items   []model.UserTrackedProduct `json:"items"`
	}
	_ = json.Unmarshal(listRR1.Body.Bytes(), &list1)
	if list1.Count != 1 || list1.Items[0].EffectiveIntervalLabel != "Every 6 hrs" {
		t.Errorf("User 1 expected 1 item with 'Every 6 hrs', got %+v", list1)
	}

	// 6. User 3 (unauthenticated) accessing /api/tracked -> 401
	unauthReq := httptest.NewRequest(http.MethodGet, "/api/tracked", nil)
	unauthRR := httptest.NewRecorder()
	handler.HandleListTracked(unauthRR, unauthReq)
	if unauthRR.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauth /api/tracked, got %d", unauthRR.Code)
	}

	// 7. Untrack product for User 2 -> shared cron should revert to User 1's 24 hrs
	untrackReq := httptest.NewRequest(http.MethodPost, "/api/track/untrack?product_id="+strconv.FormatInt(list1.Items[0].ProductID, 10), nil)
	untrackReq.Header.Set("Authorization", "Bearer "+authRes2.Token)
	untrackRR := httptest.NewRecorder()
	handler.HandleUntrack(untrackRR, untrackReq)
	if untrackRR.Code != http.StatusOK {
		t.Fatalf("Untrack failed: %s", untrackRR.Body.String())
	}

	// Check User 1 tracked products again
	listRR1After := httptest.NewRecorder()
	handler.HandleListTracked(listRR1After, listReq1)
	var list1After struct {
		Success bool                       `json:"success"`
		Count   int                        `json:"count"`
		Items   []model.UserTrackedProduct `json:"items"`
	}
	_ = json.Unmarshal(listRR1After.Body.Bytes(), &list1After)
	if list1After.Count != 1 || list1After.Items[0].EffectiveIntervalLabel != "Every 24 hrs" {
		t.Errorf("User 1 expected revert to 'Every 24 hrs', got %+v", list1After)
	}
}
