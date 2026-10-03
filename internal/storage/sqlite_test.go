package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ecom-scraper/internal/model"
)

func createTempDB(t *testing.T) (*SQLiteStorage, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite storage: %v", err)
	}

	cleanup := func() {
		_ = store.Close()
		_ = os.Remove(dbPath)
	}
	return store, cleanup
}

func TestSQLiteProductAndHistory(t *testing.T) {
	store, cleanup := createTempDB(t)
	defer cleanup()

	product := &model.Product{
		Platform:       "Amazon India",
		URL:            "https://www.amazon.in/dp/B07YCTCMFK",
		ASIN:           "B07YCTCMFK",
		Title:          "Casio G-Shock GA-2100",
		Currency:       "₹",
		CurrentPrice:   9195.0,
		OriginalPrice:  9195.0,
		FormattedPrice: "₹9,195",
		FormattedMRP:   "₹9,195",
		InStock:        true,
		Availability:   "In stock",
		Rating:         "4.4 out of 5 stars",
		ReviewCount:    "(2,400)",
		ImageURL:       "https://example.com/img.jpg",
		ScrapedAt:      time.Now().Add(-2 * time.Hour),
	}

	// 1. Upsert product
	id, err := store.UpsertProduct(product)
	if err != nil {
		t.Fatalf("UpsertProduct failed: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive product ID, got %d", id)
	}

	// 2. Record first snapshot
	err = store.RecordPriceSnapshot(id, product)
	if err != nil {
		t.Fatalf("RecordPriceSnapshot failed: %v", err)
	}

	// 3. Record price drop snapshot
	product.CurrentPrice = 8499.0
	product.Discount = "-8%"
	product.FormattedPrice = "₹8,499"
	product.ScrapedAt = time.Now()
	err = store.RecordPriceSnapshot(id, product)
	if err != nil {
		t.Fatalf("RecordPriceSnapshot 2 failed: %v", err)
	}

	// 4. Fetch history and stats
	history, stats, err := store.GetPriceHistory(id)
	if err != nil {
		t.Fatalf("GetPriceHistory failed: %v", err)
	}

	if len(history) != 2 {
		t.Fatalf("expected 2 history records, got %d", len(history))
	}

	if stats == nil {
		t.Fatalf("expected non-nil stats")
	}

	if stats.LowestPrice != 8499.0 {
		t.Errorf("expected lowest price 8499, got %v", stats.LowestPrice)
	}
	if stats.HighestPrice != 9195.0 {
		t.Errorf("expected highest price 9195, got %v", stats.HighestPrice)
	}
	if stats.TotalChecks != 2 {
		t.Errorf("expected 2 total checks, got %d", stats.TotalChecks)
	}
	if stats.PriceChange != -696.0 {
		t.Errorf("expected price change -696, got %v", stats.PriceChange)
	}
}

func TestSQLiteTrackedJobs(t *testing.T) {
	store, cleanup := createTempDB(t)
	defer cleanup()

	product := &model.Product{
		Platform:     "Amazon India",
		URL:          "https://www.amazon.in/dp/B07YCTCMFK",
		Title:        "Casio G-Shock",
		Currency:     "₹",
		CurrentPrice: 9095,
	}

	id, err := store.UpsertProduct(product)
	if err != nil {
		t.Fatalf("failed to upsert product: %v", err)
	}

	// Create job scheduled for the past (so it's due)
	dueTime := time.Now().UTC().Add(-10 * time.Minute)
	job, err := store.UpsertTrackedJob(id, "6h", "Every 6 hrs", dueTime)
	if err != nil {
		t.Fatalf("UpsertTrackedJob failed: %v", err)
	}

	if job.Status != "active" {
		t.Errorf("expected active status, got %s", job.Status)
	}

	// Fetch due jobs
	dueJobs, err := store.GetDueTrackedJobs(time.Now().UTC())
	if err != nil {
		t.Fatalf("GetDueTrackedJobs failed: %v", err)
	}
	if len(dueJobs) != 1 {
		t.Fatalf("expected 1 due job, got %d", len(dueJobs))
	}

	// Toggle status to paused
	newStatus, err := store.ToggleTrackedJobStatus(job.ID)
	if err != nil {
		t.Fatalf("ToggleTrackedJobStatus failed: %v", err)
	}
	if newStatus != "paused" {
		t.Errorf("expected paused status, got %s", newStatus)
	}

	// Due jobs should now be 0 because it's paused
	dueJobs, _ = store.GetDueTrackedJobs(time.Now().UTC())
	if len(dueJobs) != 0 {
		t.Errorf("expected 0 due jobs when paused, got %d", len(dueJobs))
	}

	// Delete job
	err = store.DeleteTrackedJob(job.ID)
	if err != nil {
		t.Fatalf("DeleteTrackedJob failed: %v", err)
	}

	list, _ := store.ListTrackedJobs()
	if len(list) != 0 {
		t.Errorf("expected 0 jobs after deletion, got %d", len(list))
	}
}

func TestGetLatestScanTime(t *testing.T) {
	store, cleanup := createTempDB(t)
	defer cleanup()

	product := &model.Product{
		Platform:     "Amazon India",
		URL:          "https://www.amazon.in/dp/B07YCTCMFK",
		Title:        "Casio G-Shock",
		Currency:     "₹",
		CurrentPrice: 9095,
	}

	id, err := store.UpsertProduct(product)
	if err != nil {
		t.Fatalf("failed to upsert product: %v", err)
	}

	// Case 1: No history yet, returns updated_at
	latest, err := store.GetLatestScanTime(id)
	if err != nil {
		t.Fatalf("GetLatestScanTime failed: %v", err)
	}
	if latest.IsZero() {
		t.Fatalf("expected non-zero fallback time")
	}

	// Case 2: After snapshot, returns snapshot recorded_at
	scanTime := time.Now().UTC().Add(-3 * time.Hour)
	product.ScrapedAt = scanTime
	_ = store.RecordPriceSnapshot(id, product)

	latest, err = store.GetLatestScanTime(id)
	if err != nil {
		t.Fatalf("GetLatestScanTime failed: %v", err)
	}
	if latest.Sub(scanTime).Abs() > time.Second {
		t.Errorf("expected %v, got %v", scanTime, latest)
	}
}

func TestAuthAndMultiUserLowestInterval(t *testing.T) {
	store, cleanup := createTempDB(t)
	defer cleanup()

	// 1. Create Users
	u1, err := store.CreateUser("alice", "password123")
	if err != nil {
		t.Fatalf("CreateUser alice failed: %v", err)
	}
	u2, err := store.CreateUser("bob", "secret456")
	if err != nil {
		t.Fatalf("CreateUser bob failed: %v", err)
	}

	// Test authentication
	authU1, err := store.AuthenticateUser("alice", "password123")
	if err != nil || authU1.ID != u1.ID {
		t.Fatalf("AuthenticateUser failed for alice: %v", err)
	}
	_, err = store.AuthenticateUser("alice", "wrongpass")
	if err == nil {
		t.Fatalf("expected error on wrong password, got nil")
	}

	// Test session
	token, err := store.CreateSession(u1.ID, 24*time.Hour)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	sessionUser, err := store.GetUserBySessionToken(token)
	if err != nil || sessionUser.ID != u1.ID {
		t.Fatalf("GetUserBySessionToken failed: %v", err)
	}

	// 2. Insert Casio Watch (Deduplicated Product)
	casio := &model.Product{
		Platform:     "Amazon India",
		URL:          "https://www.amazon.in/dp/B07YCTCMFK",
		Title:        "Casio G-Shock GA-2100",
		CurrentPrice: 9195,
	}
	prodID, err := store.UpsertProduct(casio)
	if err != nil {
		t.Fatalf("UpsertProduct failed: %v", err)
	}

	// 3. User 1 (Alice) tracks Casio with 24h cron
	job1, err := store.AddOrUpdateUserTracker(u1.ID, prodID, "24h")
	if err != nil {
		t.Fatalf("AddOrUpdateUserTracker u1 failed: %v", err)
	}
	if job1.Interval != "24h" || job1.IntervalLabel != "Every 24 hrs" {
		t.Errorf("expected 24h interval, got %s / %s", job1.Interval, job1.IntervalLabel)
	}

	hasCron, err := store.HasActiveCron(prodID)
	if err != nil || !hasCron {
		t.Errorf("expected HasActiveCron to be true, got %v", hasCron)
	}

	// 4. User 2 (Bob) tracks the SAME Casio with 6h cron
	job2, err := store.AddOrUpdateUserTracker(u2.ID, prodID, "6h")
	if err != nil {
		t.Fatalf("AddOrUpdateUserTracker u2 failed: %v", err)
	}
	// The shared job should now be the LOWEST interval: 6h
	if job2.Interval != "6h" || job2.IntervalLabel != "Every 6 hrs" {
		t.Errorf("expected shared lowest 6h interval, got %s / %s", job2.Interval, job2.IntervalLabel)
	}

	// 5. Verify User 1 sees the product with shared lowest cron (6h)
	u1Prods, err := store.GetUserTrackedProducts(u1.ID)
	if err != nil {
		t.Fatalf("GetUserTrackedProducts u1 failed: %v", err)
	}
	if len(u1Prods) != 1 {
		t.Fatalf("expected 1 tracked product for u1, got %d", len(u1Prods))
	}
	if u1Prods[0].EffectiveIntervalLabel != "Every 6 hrs" {
		t.Errorf("expected effective cron 'Every 6 hrs' for u1, got %s", u1Prods[0].EffectiveIntervalLabel)
	}
	if u1Prods[0].UserDesiredInterval != "24h" {
		t.Errorf("expected user interval '24h' for u1, got %s", u1Prods[0].UserDesiredInterval)
	}

	// 6. Verify User 2 sees the product with shared lowest cron (6h)
	u2Prods, err := store.GetUserTrackedProducts(u2.ID)
	if err != nil {
		t.Fatalf("GetUserTrackedProducts u2 failed: %v", err)
	}
	if len(u2Prods) != 1 {
		t.Fatalf("expected 1 tracked product for u2, got %d", len(u2Prods))
	}
	if u2Prods[0].EffectiveIntervalLabel != "Every 6 hrs" {
		t.Errorf("expected effective cron 'Every 6 hrs' for u2, got %s", u2Prods[0].EffectiveIntervalLabel)
	}
	if u2Prods[0].UserDesiredInterval != "6h" {
		t.Errorf("expected user interval '6h' for u2, got %s", u2Prods[0].UserDesiredInterval)
	}

	// 7. User 3 (Charlie) registers and has 0 products
	u3, err := store.CreateUser("charlie", "password789")
	if err != nil {
		t.Fatalf("CreateUser charlie failed: %v", err)
	}
	u3Prods, err := store.GetUserTrackedProducts(u3.ID)
	if err != nil {
		t.Fatalf("GetUserTrackedProducts u3 failed: %v", err)
	}
	if len(u3Prods) != 0 {
		t.Fatalf("expected 0 tracked products for u3, got %d", len(u3Prods))
	}

	// 8. Bob (6h) untracks the product. Shared cron should revert to Alice's 24h!
	err = store.RemoveUserTracker(u2.ID, prodID)
	if err != nil {
		t.Fatalf("RemoveUserTracker u2 failed: %v", err)
	}

	u1ProdsAfter, _ := store.GetUserTrackedProducts(u1.ID)
	if len(u1ProdsAfter) != 1 {
		t.Fatalf("expected 1 product for u1 after Bob untracks, got %d", len(u1ProdsAfter))
	}
	if u1ProdsAfter[0].EffectiveIntervalLabel != "Every 24 hrs" {
		t.Errorf("expected effective cron to revert to 'Every 24 hrs', got %s", u1ProdsAfter[0].EffectiveIntervalLabel)
	}

	// 9. Alice untracks the product. Cron should now be deleted.
	err = store.RemoveUserTracker(u1.ID, prodID)
	if err != nil {
		t.Fatalf("RemoveUserTracker u1 failed: %v", err)
	}
	hasCronAfter, err := store.HasActiveCron(prodID)
	if err != nil || hasCronAfter {
		t.Errorf("expected HasActiveCron to be false after all users untracked, got %v", hasCronAfter)
	}
}
