package storage

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"ecom-scraper/internal/model"
)

var (
	ErrUserAlreadyExists = errors.New("username is already taken")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrSessionExpired    = errors.New("session has expired or is invalid")
)

// Storage defines the interface for database persistence.
type Storage interface {
	UpsertProduct(p *model.Product) (int64, error)
	RecordPriceSnapshot(productID int64, p *model.Product) error
	GetProductByID(id int64) (*model.Product, error)
	GetProductByURL(url string) (*model.Product, error)
	GetPriceHistory(productID int64) ([]model.PriceHistory, *model.PriceStats, error)
	GetPriceHistoryByURL(rawURL string) (*model.Product, []model.PriceHistory, *model.PriceStats, error)
	UpsertTrackedJob(productID int64, interval string, intervalLabel string, nextRunAt time.Time) (*model.TrackedJob, error)
	GetTrackedJobByProductID(productID int64) (*model.TrackedJob, error)
	GetDueTrackedJobs(now time.Time) ([]model.TrackedJob, error)
	GetLatestScanTime(productID int64) (time.Time, error)
	UpdateJobRun(jobID int64, lastRunAt time.Time, nextRunAt time.Time) error
	ListTrackedJobs() ([]model.TrackedJob, error)
	DeleteTrackedJob(jobID int64) error
	ToggleTrackedJobStatus(jobID int64) (string, error)

	// Auth
	CreateUser(username, password string) (*model.User, error)
	AuthenticateUser(username, password string) (*model.User, error)
	CreateSession(userID int64, duration time.Duration) (string, error)
	GetUserBySessionToken(token string) (*model.User, error)
	DeleteSession(token string) error

	// Multi-User Tracking & Deduplication
	AddOrUpdateUserTracker(userID int64, productID int64, desiredInterval string) (*model.TrackedJob, error)
	RemoveUserTracker(userID int64, productID int64) error
	GetUserTrackedProducts(userID int64) ([]model.UserTrackedProduct, error)
	IsProductTrackedByUser(userID int64, productID int64) (bool, string, error)
	HasActiveCron(productID int64) (bool, error)

	Close() error
}

type SQLiteStorage struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteStorage initializes SQLite database and runs migrations.
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)

	s := &SQLiteStorage{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to migrate sqlite database: %w", err)
	}

	return s, nil
}

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

func (s *SQLiteStorage) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL COLLATE NOCASE,
		password_hash TEXT NOT NULL,
		created_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS sessions (
		token TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

	CREATE TABLE IF NOT EXISTS products (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT UNIQUE NOT NULL,
		asin TEXT,
		platform TEXT NOT NULL,
		title TEXT NOT NULL,
		currency TEXT NOT NULL,
		image_url TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS price_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		product_id INTEGER NOT NULL REFERENCES products(id) ON DELETE CASCADE,
		current_price REAL NOT NULL,
		original_price REAL,
		discount TEXT,
		formatted_price TEXT,
		formatted_mrp TEXT,
		in_stock BOOLEAN,
		availability TEXT,
		rating TEXT,
		review_count TEXT,
		recorded_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_history_product_date ON price_history(product_id, recorded_at ASC);

	CREATE TABLE IF NOT EXISTS tracked_jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		product_id INTEGER NOT NULL UNIQUE REFERENCES products(id) ON DELETE CASCADE,
		interval TEXT NOT NULL,
		interval_label TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		last_run_at DATETIME,
		next_run_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_jobs_status_next ON tracked_jobs(status, next_run_at);

	CREATE TABLE IF NOT EXISTS user_trackers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		product_id INTEGER NOT NULL REFERENCES products(id) ON DELETE CASCADE,
		desired_interval TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		UNIQUE(user_id, product_id)
	);
	CREATE INDEX IF NOT EXISTS idx_user_trackers_user ON user_trackers(user_id);
	CREATE INDEX IF NOT EXISTS idx_user_trackers_product ON user_trackers(product_id);
	`
	_, err := s.db.Exec(schema)
	return err
}

// -------------------------------------------------------------
// Auth Implementations
// -------------------------------------------------------------

func (s *SQLiteStorage) CreateUser(username, password string) (*model.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	username = strings.TrimSpace(username)
	if len(username) < 3 {
		return nil, errors.New("username must be at least 3 characters")
	}
	if len(password) < 4 {
		return nil, errors.New("password must be at least 4 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now().UTC()
	query := `INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?);`
	res, err := s.db.Exec(query, username, string(hash), now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrUserAlreadyExists
		}
		return nil, err
	}

	id, _ := res.LastInsertId()
	return &model.User{
		ID:        id,
		Username:  username,
		CreatedAt: now,
	}, nil
}

func (s *SQLiteStorage) AuthenticateUser(username, password string) (*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	username = strings.TrimSpace(username)
	var u model.User
	var hash string
	err := s.db.QueryRow(`SELECT id, username, password_hash, created_at FROM users WHERE username = ?;`, username).Scan(&u.ID, &u.Username, &hash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return &u, nil
}

func (s *SQLiteStorage) CreateSession(userID int64, duration time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	expiresAt := time.Now().UTC().Add(duration)

	query := `INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?);`
	_, err := s.db.Exec(query, token, userID, expiresAt)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s *SQLiteStorage) GetUserBySessionToken(token string) (*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT u.id, u.username, u.created_at, s.expires_at
	FROM sessions s
	JOIN users u ON s.user_id = u.id
	WHERE s.token = ?;
	`
	var u model.User
	var expiresAt time.Time
	err := s.db.QueryRow(query, token).Scan(&u.ID, &u.Username, &u.CreatedAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionExpired
	}
	if err != nil {
		return nil, err
	}

	if time.Now().UTC().After(expiresAt) {
		return nil, ErrSessionExpired
	}

	return &u, nil
}

func (s *SQLiteStorage) DeleteSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?;`, token)
	return err
}

// -------------------------------------------------------------
// Product & Price Snapshots
// -------------------------------------------------------------

// UpsertProduct inserts or updates a product by its canonical URL.
func (s *SQLiteStorage) UpsertProduct(p *model.Product) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	query := `
	INSERT INTO products (url, asin, platform, title, currency, image_url, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(url) DO UPDATE SET
		asin = CASE WHEN excluded.asin != '' THEN excluded.asin ELSE products.asin END,
		platform = excluded.platform,
		title = excluded.title,
		currency = excluded.currency,
		image_url = CASE WHEN excluded.image_url != '' THEN excluded.image_url ELSE products.image_url END,
		updated_at = excluded.updated_at
	RETURNING id;
	`
	var id int64
	err := s.db.QueryRow(query, p.URL, p.ASIN, p.Platform, p.Title, p.Currency, p.ImageURL, now, now).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to upsert product: %w", err)
	}
	p.ID = id
	return id, nil
}

// RecordPriceSnapshot appends a new price point to price_history.
func (s *SQLiteStorage) RecordPriceSnapshot(productID int64, p *model.Product) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	recordedAt := p.ScrapedAt
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}

	query := `
	INSERT INTO price_history (
		product_id, current_price, original_price, discount, formatted_price,
		formatted_mrp, in_stock, availability, rating, review_count, recorded_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	_, err := s.db.Exec(
		query,
		productID,
		p.CurrentPrice,
		p.OriginalPrice,
		p.Discount,
		p.FormattedPrice,
		p.FormattedMRP,
		p.InStock,
		p.Availability,
		p.Rating,
		p.ReviewCount,
		recordedAt,
	)
	return err
}

func (s *SQLiteStorage) GetProductByID(id int64) (*model.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, url, asin, platform, title, currency, image_url, updated_at FROM products WHERE id = ?;`
	row := s.db.QueryRow(query, id)

	var p model.Product
	var updatedAt time.Time
	err := row.Scan(&p.ID, &p.URL, &p.ASIN, &p.Platform, &p.Title, &p.Currency, &p.ImageURL, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.ScrapedAt = updatedAt
	return &p, nil
}

func (s *SQLiteStorage) GetProductByURL(url string) (*model.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, url, asin, platform, title, currency, image_url, updated_at FROM products WHERE url = ?;`
	row := s.db.QueryRow(query, url)

	var p model.Product
	var updatedAt time.Time
	err := row.Scan(&p.ID, &p.URL, &p.ASIN, &p.Platform, &p.Title, &p.Currency, &p.ImageURL, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.ScrapedAt = updatedAt
	return &p, nil
}

func (s *SQLiteStorage) GetPriceHistory(productID int64) ([]model.PriceHistory, *model.PriceStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, product_id, current_price, original_price, discount, formatted_price,
	       formatted_mrp, in_stock, availability, rating, review_count, recorded_at
	FROM price_history
	WHERE product_id = ?
	ORDER BY recorded_at ASC;
	`
	rows, err := s.db.Query(query, productID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var history []model.PriceHistory
	var minPrice = math.MaxFloat64
	var maxPrice = -1.0
	var sumPrice = 0.0

	for rows.Next() {
		var h model.PriceHistory
		var origPrice sql.NullFloat64
		var origFmt sql.NullString
		err := rows.Scan(
			&h.ID, &h.ProductID, &h.CurrentPrice, &origPrice, &h.Discount,
			&h.FormattedPrice, &origFmt, &h.InStock, &h.Availability,
			&h.Rating, &h.ReviewCount, &h.RecordedAt,
		)
		if err != nil {
			return nil, nil, err
		}
		if origPrice.Valid {
			h.OriginalPrice = origPrice.Float64
		}
		if origFmt.Valid {
			h.FormattedMRP = origFmt.String
		}

		if h.CurrentPrice > 0 {
			if h.CurrentPrice < minPrice {
				minPrice = h.CurrentPrice
			}
			if h.CurrentPrice > maxPrice {
				maxPrice = h.CurrentPrice
			}
			sumPrice += h.CurrentPrice
		}

		history = append(history, h)
	}

	if len(history) == 0 {
		return []model.PriceHistory{}, nil, nil
	}

	avgPrice := sumPrice / float64(len(history))
	firstPrice := history[0].CurrentPrice
	lastPrice := history[len(history)-1].CurrentPrice

	stats := &model.PriceStats{
		LowestPrice:  minPrice,
		HighestPrice: maxPrice,
		AveragePrice: math.Round(avgPrice*100) / 100,
		TotalChecks:  len(history),
		PriceChange:  math.Round((lastPrice-firstPrice)*100) / 100,
	}

	return history, stats, nil
}

func (s *SQLiteStorage) GetPriceHistoryByURL(rawURL string) (*model.Product, []model.PriceHistory, *model.PriceStats, error) {
	prod, err := s.GetProductByURL(rawURL)
	if err != nil || prod == nil {
		return nil, nil, nil, err
	}

	history, stats, err := s.GetPriceHistory(prod.ID)
	if err != nil {
		return nil, nil, nil, err
	}

	if len(history) > 0 {
		latest := history[len(history)-1]
		prod.CurrentPrice = latest.CurrentPrice
		prod.OriginalPrice = latest.OriginalPrice
		prod.Discount = latest.Discount
		prod.FormattedPrice = latest.FormattedPrice
		prod.FormattedMRP = latest.FormattedMRP
		prod.InStock = latest.InStock
		prod.Availability = latest.Availability
		prod.Rating = latest.Rating
		prod.ReviewCount = latest.ReviewCount
		prod.ScrapedAt = latest.RecordedAt
	}

	job, _ := s.GetTrackedJobByProductID(prod.ID)
	prod.TrackedJob = job

	return prod, history, stats, nil
}

// HasActiveCron returns true if the product currently has an active scheduled cron job.
func (s *SQLiteStorage) HasActiveCron(productID int64) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM tracked_jobs WHERE product_id = ? AND status = 'active';`, productID).Scan(&count)
	return count > 0, err
}

// -------------------------------------------------------------
// Multi-User Tracking & Lowest Interval Calculation
// -------------------------------------------------------------

// AddOrUpdateUserTracker registers product tracking for a specific user,
// and recalculates the lowest interval cron job to run for this product.
func (s *SQLiteStorage) AddOrUpdateUserTracker(userID int64, productID int64, desiredInterval string) (*model.TrackedJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	query := `
	INSERT INTO user_trackers (user_id, product_id, desired_interval, created_at)
	VALUES (?, ?, ?, ?)
	ON CONFLICT(user_id, product_id) DO UPDATE SET
		desired_interval = excluded.desired_interval;
	`
	_, err := s.db.Exec(query, userID, productID, desiredInterval, now)
	if err != nil {
		return nil, fmt.Errorf("failed to save user tracker: %w", err)
	}

	// Recalculate shared cron job at lowest interval
	return s.recalculateSharedCronLocked(productID)
}

// RemoveUserTracker deletes tracking for a user and updates the shared cron job.
func (s *SQLiteStorage) RemoveUserTracker(userID int64, productID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM user_trackers WHERE user_id = ? AND product_id = ?;`, userID, productID)
	if err != nil {
		return err
	}

	_, err = s.recalculateSharedCronLocked(productID)
	return err
}

// recalculateSharedCronLocked determines the lowest interval across all users tracking this product.
// Must be called with s.mu held.
func (s *SQLiteStorage) recalculateSharedCronLocked(productID int64) (*model.TrackedJob, error) {
	rows, err := s.db.Query(`SELECT desired_interval FROM user_trackers WHERE product_id = ?;`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var minDuration time.Duration = math.MaxInt64
	var minKey string
	var minLabel string
	count := 0

	for rows.Next() {
		var desired string
		if err := rows.Scan(&desired); err != nil {
			continue
		}
		dur, label, key, err := model.ParseInterval(desired)
		if err != nil {
			dur = 24 * time.Hour
			key = "24h"
			label = "Every 24 hrs"
		}
		count++
		if dur < minDuration {
			minDuration = dur
			minKey = key
			minLabel = label
		}
	}

	if count == 0 {
		// No users tracking this product: remove shared cron job
		_, _ = s.db.Exec(`DELETE FROM tracked_jobs WHERE product_id = ?;`, productID)
		return nil, nil
	}

	now := time.Now().UTC()
	nextRun := now.Add(minDuration)

	// Check if an existing cron job exists
	var existingNext time.Time
	var existingID int64
	row := s.db.QueryRow(`SELECT id, next_run_at FROM tracked_jobs WHERE product_id = ?;`, productID)
	if err := row.Scan(&existingID, &existingNext); err == nil {
		if existingNext.Before(nextRun) && existingNext.After(now) {
			nextRun = existingNext
		}
	}

	query := `
	INSERT INTO tracked_jobs (product_id, interval, interval_label, status, next_run_at, created_at)
	VALUES (?, ?, ?, 'active', ?, ?)
	ON CONFLICT(product_id) DO UPDATE SET
		interval = excluded.interval,
		interval_label = excluded.interval_label,
		status = 'active',
		next_run_at = excluded.next_run_at
	RETURNING id, product_id, interval, interval_label, status, last_run_at, next_run_at, created_at;
	`
	row = s.db.QueryRow(query, productID, minKey, minLabel, nextRun, now)
	var job model.TrackedJob
	var lastRun sql.NullTime
	err = row.Scan(&job.ID, &job.ProductID, &job.Interval, &job.IntervalLabel, &job.Status, &lastRun, &job.NextRunAt, &job.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert shared tracked job: %w", err)
	}
	if lastRun.Valid {
		job.LastRunAt = &lastRun.Time
	}
	return &job, nil
}

// parseAnyTime safely parses time from time.Time, string, []byte, or int64.
func parseAnyTime(val interface{}) time.Time {
	if val == nil {
		return time.Time{}
	}
	switch v := val.(type) {
	case time.Time:
		return v
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return time.Time{}
		}
		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02 15:04:05.999999999",
			"2006-01-02 15:04:05-07:00",
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, v); err == nil {
				return t
			}
		}
	case []byte:
		return parseAnyTime(string(v))
	case int64:
		return time.Unix(v, 0)
	}
	return time.Time{}
}

// GetUserTrackedProducts returns only the products tracked by a specific user.
func (s *SQLiteStorage) GetUserTrackedProducts(userID int64) ([]model.UserTrackedProduct, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT ut.product_id, ut.desired_interval, ut.created_at,
	       p.url, p.asin, p.platform, p.title, p.currency, p.image_url,
	       COALESCE(j.interval, ut.desired_interval) as eff_interval,
	       COALESCE(j.interval_label, ut.desired_interval) as eff_label,
	       COALESCE(j.status, 'inactive') as cron_status,
	       j.next_run_at,
	       j.last_run_at,
	       h.current_price, h.formatted_price, h.discount, h.in_stock
	FROM user_trackers ut
	JOIN products p ON ut.product_id = p.id
	LEFT JOIN tracked_jobs j ON j.product_id = p.id
	LEFT JOIN (
		SELECT product_id, current_price, formatted_price, discount, in_stock, MAX(recorded_at)
		FROM price_history
		GROUP BY product_id
	) h ON h.product_id = p.id
	WHERE ut.user_id = ?
	ORDER BY ut.created_at DESC;
	`
	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.UserTrackedProduct
	for rows.Next() {
		var item model.UserTrackedProduct
		var rawAddedAt interface{}
		var rawNextRun interface{}
		var rawLastRun interface{}
		var curPrice sql.NullFloat64
		var fmtPrice sql.NullString
		var discount sql.NullString
		var inStock sql.NullBool

		err := rows.Scan(
			&item.ProductID, &item.UserDesiredInterval, &rawAddedAt,
			&item.URL, &item.ASIN, &item.Platform, &item.Title, &item.Currency, &item.ImageURL,
			&item.EffectiveCronInterval, &item.EffectiveIntervalLabel, &item.CronStatus,
			&rawNextRun, &rawLastRun,
			&curPrice, &fmtPrice, &discount, &inStock,
		)
		if err != nil {
			return nil, err
		}

		item.AddedAt = parseAnyTime(rawAddedAt)
		nextTime := parseAnyTime(rawNextRun)
		if !nextTime.IsZero() {
			item.NextRunAt = nextTime
		} else {
			item.NextRunAt = item.AddedAt
		}

		lastTime := parseAnyTime(rawLastRun)
		if !lastTime.IsZero() {
			item.LastRunAt = &lastTime
		}

		if curPrice.Valid {
			item.CurrentPrice = curPrice.Float64
		}
		if fmtPrice.Valid {
			item.FormattedPrice = fmtPrice.String
		}
		if discount.Valid {
			item.Discount = discount.String
		}
		if inStock.Valid {
			item.InStock = inStock.Bool
		}

		list = append(list, item)
	}

	return list, nil
}

// IsProductTrackedByUser checks if a user is already tracking a given product.
func (s *SQLiteStorage) IsProductTrackedByUser(userID int64, productID int64) (bool, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var interval string
	err := s.db.QueryRow(`SELECT desired_interval FROM user_trackers WHERE user_id = ? AND product_id = ?;`, userID, productID).Scan(&interval)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, interval, nil
}

// -------------------------------------------------------------
// Shared Cron Jobs
// -------------------------------------------------------------

func (s *SQLiteStorage) UpsertTrackedJob(productID int64, interval string, intervalLabel string, nextRunAt time.Time) (*model.TrackedJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	query := `
	INSERT INTO tracked_jobs (product_id, interval, interval_label, status, next_run_at, created_at)
	VALUES (?, ?, ?, 'active', ?, ?)
	ON CONFLICT(product_id) DO UPDATE SET
		interval = excluded.interval,
		interval_label = excluded.interval_label,
		status = 'active',
		next_run_at = excluded.next_run_at
	RETURNING id, product_id, interval, interval_label, status, last_run_at, next_run_at, created_at;
	`
	row := s.db.QueryRow(query, productID, interval, intervalLabel, nextRunAt, now)
	var job model.TrackedJob
	var lastRun sql.NullTime
	err := row.Scan(&job.ID, &job.ProductID, &job.Interval, &job.IntervalLabel, &job.Status, &lastRun, &job.NextRunAt, &job.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert tracked job: %w", err)
	}
	if lastRun.Valid {
		job.LastRunAt = &lastRun.Time
	}
	return &job, nil
}

func (s *SQLiteStorage) GetTrackedJobByProductID(productID int64) (*model.TrackedJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, product_id, interval, interval_label, status, last_run_at, next_run_at, created_at FROM tracked_jobs WHERE product_id = ?;`
	row := s.db.QueryRow(query, productID)
	var job model.TrackedJob
	var lastRun sql.NullTime
	err := row.Scan(&job.ID, &job.ProductID, &job.Interval, &job.IntervalLabel, &job.Status, &lastRun, &job.NextRunAt, &job.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastRun.Valid {
		job.LastRunAt = &lastRun.Time
	}
	return &job, nil
}

func (s *SQLiteStorage) GetDueTrackedJobs(now time.Time) ([]model.TrackedJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT j.id, j.product_id, j.interval, j.interval_label, j.status, j.last_run_at, j.next_run_at, j.created_at,
	       p.url, p.asin, p.platform, p.title, p.currency, p.image_url
	FROM tracked_jobs j
	JOIN products p ON j.product_id = p.id
	WHERE j.status = 'active' AND j.next_run_at <= ?
	ORDER BY j.next_run_at ASC;
	`
	rows, err := s.db.Query(query, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []model.TrackedJob
	for rows.Next() {
		var j model.TrackedJob
		var p model.Product
		var lastRun sql.NullTime
		err := rows.Scan(
			&j.ID, &j.ProductID, &j.Interval, &j.IntervalLabel, &j.Status, &lastRun, &j.NextRunAt, &j.CreatedAt,
			&p.URL, &p.ASIN, &p.Platform, &p.Title, &p.Currency, &p.ImageURL,
		)
		if err != nil {
			return nil, err
		}
		if lastRun.Valid {
			j.LastRunAt = &lastRun.Time
		}
		p.ID = j.ProductID
		j.Product = &p
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (s *SQLiteStorage) GetLatestScanTime(productID int64) (time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT recorded_at FROM price_history
	WHERE product_id = ?
	ORDER BY recorded_at DESC
	LIMIT 1;
	`
	var recordedAt time.Time
	err := s.db.QueryRow(query, productID).Scan(&recordedAt)
	if err == nil {
		return recordedAt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, err
	}

	var updatedAt time.Time
	err = s.db.QueryRow(`SELECT updated_at FROM products WHERE id = ?;`, productID).Scan(&updatedAt)
	if err != nil {
		return time.Time{}, err
	}
	return updatedAt, nil
}

func (s *SQLiteStorage) UpdateJobRun(jobID int64, lastRunAt time.Time, nextRunAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `UPDATE tracked_jobs SET last_run_at = ?, next_run_at = ? WHERE id = ?;`
	_, err := s.db.Exec(query, lastRunAt, nextRunAt, jobID)
	return err
}

func (s *SQLiteStorage) ListTrackedJobs() ([]model.TrackedJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT j.id, j.product_id, j.interval, j.interval_label, j.status, j.last_run_at, j.next_run_at, j.created_at,
	       p.url, p.asin, p.platform, p.title, p.currency, p.image_url,
	       h.current_price, h.formatted_price, h.discount
	FROM tracked_jobs j
	JOIN products p ON j.product_id = p.id
	LEFT JOIN (
		SELECT product_id, current_price, formatted_price, discount, MAX(recorded_at)
		FROM price_history
		GROUP BY product_id
	) h ON h.product_id = p.id
	ORDER BY j.created_at DESC;
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []model.TrackedJob
	for rows.Next() {
		var j model.TrackedJob
		var p model.Product
		var lastRun sql.NullTime
		var curPrice sql.NullFloat64
		var fmtPrice sql.NullString
		var discount sql.NullString

		err := rows.Scan(
			&j.ID, &j.ProductID, &j.Interval, &j.IntervalLabel, &j.Status, &lastRun, &j.NextRunAt, &j.CreatedAt,
			&p.URL, &p.ASIN, &p.Platform, &p.Title, &p.Currency, &p.ImageURL,
			&curPrice, &fmtPrice, &discount,
		)
		if err != nil {
			return nil, err
		}
		if lastRun.Valid {
			j.LastRunAt = &lastRun.Time
		}
		p.ID = j.ProductID
		if curPrice.Valid {
			p.CurrentPrice = curPrice.Float64
		}
		if fmtPrice.Valid {
			p.FormattedPrice = fmtPrice.String
		}
		if discount.Valid {
			p.Discount = discount.String
		}
		j.Product = &p
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (s *SQLiteStorage) DeleteTrackedJob(jobID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `DELETE FROM tracked_jobs WHERE id = ?;`
	_, err := s.db.Exec(query, jobID)
	return err
}

func (s *SQLiteStorage) ToggleTrackedJobStatus(jobID int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var status string
	err := s.db.QueryRow(`SELECT status FROM tracked_jobs WHERE id = ?;`, jobID).Scan(&status)
	if err != nil {
		return "", err
	}

	newStatus := "paused"
	if status == "paused" {
		newStatus = "active"
	}

	_, err = s.db.Exec(`UPDATE tracked_jobs SET status = ? WHERE id = ?;`, newStatus, jobID)
	return newStatus, err
}
