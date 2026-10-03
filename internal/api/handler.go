package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ecom-scraper/internal/model"
	"ecom-scraper/internal/scheduler"
	"ecom-scraper/internal/scraper"
	"ecom-scraper/internal/storage"
)

type Handler struct {
	registry  *scraper.Registry
	store     storage.Storage
	sched     *scheduler.Scheduler
	startTime time.Time
}

func NewHandler(registry *scraper.Registry, store storage.Storage, sched *scheduler.Scheduler) *Handler {
	return &Handler{
		registry:  registry,
		store:     store,
		sched:     sched,
		startTime: time.Now(),
	}
}

// EnableCORS adds standard CORS headers and handles preflight OPTIONS requests.
func (h *Handler) EnableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Helper to extract authenticated user from Bearer header or cookie.
func (h *Handler) getUserFromRequest(r *http.Request) *model.User {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		user, err := h.store.GetUserBySessionToken(token)
		if err == nil && user != nil {
			return user
		}
	}

	cookie, err := r.Cookie("session_token")
	if err == nil && cookie.Value != "" {
		user, err := h.store.GetUserBySessionToken(cookie.Value)
		if err == nil && user != nil {
			return user
		}
	}

	return nil
}

// -------------------------------------------------------------
// Authentication Endpoints
// -------------------------------------------------------------

// HandleRegister creates a new user account.
func (h *Handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, model.AuthResponse{Success: false, Error: "only POST allowed"})
		return
	}

	var req model.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.AuthResponse{Success: false, Error: "invalid JSON"})
		return
	}

	user, err := h.store.CreateUser(req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.AuthResponse{Success: false, Error: err.Error()})
		return
	}

	token, err := h.store.CreateSession(user.ID, 30*24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.AuthResponse{Success: false, Error: "failed to create session"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, model.AuthResponse{
		Success: true,
		Token:   token,
		User:    user,
	})
}

// HandleLogin authenticates an existing user.
func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, model.AuthResponse{Success: false, Error: "only POST allowed"})
		return
	}

	var req model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.AuthResponse{Success: false, Error: "invalid JSON"})
		return
	}

	user, err := h.store.AuthenticateUser(req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, model.AuthResponse{Success: false, Error: err.Error()})
		return
	}

	token, err := h.store.CreateSession(user.ID, 30*24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.AuthResponse{Success: false, Error: "failed to create session"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, model.AuthResponse{
		Success: true,
		Token:   token,
		User:    user,
	})
}

// HandleLogout terminates the active session.
func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false})
		return
	}

	authHeader := r.Header.Get("Authorization")
	var token string
	if strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	} else if cookie, err := r.Cookie("session_token"); err == nil {
		token = cookie.Value
	}

	if token != "" {
		_ = h.store.DeleteSession(token)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: false,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "logged out successfully"})
}

// HandleMe returns the currently authenticated user.
func (h *Handler) HandleMe(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromRequest(r)
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, model.AuthResponse{Success: false, Error: "unauthenticated"})
		return
	}

	writeJSON(w, http.StatusOK, model.AuthResponse{
		Success: true,
		User:    user,
	})
}

// -------------------------------------------------------------
// Product Scrape & Deduplication Endpoints
// -------------------------------------------------------------

// HealthCheck returns server status and uptime.
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "healthy",
		"uptime": time.Since(h.startTime).String(),
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// GetPlatforms returns the list of supported platforms.
func (h *Handler) GetPlatforms(w http.ResponseWriter, r *http.Request) {
	platforms := h.registry.ListPlatforms()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"count":     len(platforms),
		"platforms": platforms,
	})
}

// GetIntervals returns supported cron tracking intervals.
func (h *Handler) GetIntervals(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"intervals": model.Intervals,
	})
}

// HandleScrape handles product scraping with deduplication & cron status awareness:
// 1. If product is in DB AND has an active cron job: returns cached deduplicated product with latest price & history.
// 2. If NO active cron jobs are set: fetches the fresh new price from the target website and updates SQLite.
func (h *Handler) HandleScrape(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var targetURL string

	switch r.Method {
	case http.MethodGet:
		targetURL = strings.TrimSpace(r.URL.Query().Get("url"))
	case http.MethodPost:
		var req model.ScrapeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.ScrapeResponse{
				Success: false,
				Error:   "invalid JSON payload in request body",
				TookMs:  time.Since(start).Milliseconds(),
			})
			return
		}
		targetURL = strings.TrimSpace(req.URL)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, model.ScrapeResponse{
			Success: false,
			Error:   "only GET and POST methods are allowed",
			TookMs:  time.Since(start).Milliseconds(),
		})
		return
	}

	if targetURL == "" {
		writeJSON(w, http.StatusBadRequest, model.ScrapeResponse{
			Success: false,
			Error:   "url parameter is required",
			TookMs:  time.Since(start).Milliseconds(),
		})
		return
	}

	sc, parsedURL, err := h.registry.FindScraper(targetURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.ScrapeResponse{
			Success: false,
			Error:   err.Error(),
			TookMs:  time.Since(start).Milliseconds(),
		})
		return
	}

	// Canonicalize Amazon URL if applicable to guarantee product deduplication
	canonicalURL := targetURL
	if strings.Contains(parsedURL.Host, "amazon") {
		// Use scraper canonicalize logic or standard ASIN path
		if strings.Contains(targetURL, "/dp/") {
			idx := strings.Index(targetURL, "/dp/")
			if idx != -1 && len(targetURL) >= idx+14 {
				asin := targetURL[idx+4 : idx+14]
				canonicalURL = "https://www.amazon.in/dp/" + asin
			}
		}
	}

	currentUser := h.getUserFromRequest(r)

	// Check if product already exists in SQLite
	existingProd, history, _, err := h.store.GetPriceHistoryByURL(canonicalURL)
	if existingProd == nil && canonicalURL != targetURL {
		existingProd, history, _, err = h.store.GetPriceHistoryByURL(targetURL)
	}

	if existingProd != nil {
		hasCron, _ := h.store.HasActiveCron(existingProd.ID)
		if hasCron {
			// Product has an active background cron tracking it!
			// Return deduplicated product from DB without unnecessary external scraping
			existingProd.History = history
			h.attachUserTrackingContext(existingProd, currentUser)

			writeJSON(w, http.StatusOK, model.ScrapeResponse{
				Success: true,
				Data:    existingProd,
				TookMs:  time.Since(start).Milliseconds(),
			})
			return
		}
	}

	// If no active crons are set, or product not yet in DB: fetch fresh price!
	freshProduct, err := sc.Scrape(r.Context(), targetURL)
	tookMs := time.Since(start).Milliseconds()

	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, model.ScrapeResponse{
			Success: false,
			Error:   err.Error(),
			TookMs:  tookMs,
		})
		return
	}

	// Persist to SQLite
	productID, err := h.store.UpsertProduct(freshProduct)
	if err == nil {
		freshProduct.ID = productID
		_ = h.store.RecordPriceSnapshot(productID, freshProduct)

		// Load updated history
		updatedHistory, _, err := h.store.GetPriceHistory(productID)
		if err == nil {
			freshProduct.History = updatedHistory
		}

		h.attachUserTrackingContext(freshProduct, currentUser)
	}

	writeJSON(w, http.StatusOK, model.ScrapeResponse{
		Success: true,
		Data:    freshProduct,
		TookMs:  tookMs,
	})
}

func (h *Handler) attachUserTrackingContext(p *model.Product, u *model.User) {
	job, _ := h.store.GetTrackedJobByProductID(p.ID)
	p.TrackedJob = job
	if job != nil {
		p.EffectiveCron = job.IntervalLabel
	}

	if u != nil {
		isTracked, desiredInterval, _ := h.store.IsProductTrackedByUser(u.ID, p.ID)
		p.IsUserTracked = isTracked
		p.UserInterval = desiredInterval
	}
}

// -------------------------------------------------------------
// User-Scoped Tracking & Lowest Cron Interval Endpoints
// -------------------------------------------------------------

// HandleTrack handles adding/updating a product in the logged-in user's tracker list.
// Automatically calculates the lowest interval across all users tracking this product,
// and sets that lowest interval on the shared cron job.
func (h *Handler) HandleTrack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, model.TrackResponse{
			Success: false,
			Error:   "only POST method is allowed",
		})
		return
	}

	user := h.getUserFromRequest(r)
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, model.TrackResponse{
			Success: false,
			Error:   "please log in to track products",
		})
		return
	}

	var req model.TrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.TrackResponse{
			Success: false,
			Error:   "invalid JSON request",
		})
		return
	}

	cleanURL := strings.TrimSpace(req.URL)
	if cleanURL == "" {
		writeJSON(w, http.StatusBadRequest, model.TrackResponse{
			Success: false,
			Error:   "url is required",
		})
		return
	}

	_, label, key, err := model.ParseInterval(req.Interval)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, model.TrackResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// 1. Ensure product exists in DB (deduplicated)
	prod, err := h.store.GetProductByURL(cleanURL)
	if err != nil || prod == nil {
		sc, _, err := h.registry.FindScraper(cleanURL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, model.TrackResponse{
				Success: false,
				Error:   "unsupported platform: " + err.Error(),
			})
			return
		}

		prod, err = sc.Scrape(r.Context(), cleanURL)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, model.TrackResponse{
				Success: false,
				Error:   "initial product scrape failed: " + err.Error(),
			})
			return
		}

		prodID, err := h.store.UpsertProduct(prod)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, model.TrackResponse{
				Success: false,
				Error:   "failed to save product: " + err.Error(),
			})
			return
		}
		prod.ID = prodID
		_ = h.store.RecordPriceSnapshot(prodID, prod)
	}

	// 2. Add or update user's tracker and recalculate lowest cron interval
	sharedJob, err := h.store.AddOrUpdateUserTracker(user.ID, prod.ID, key)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.TrackResponse{
			Success: false,
			Error:   "failed to schedule tracking: " + err.Error(),
		})
		return
	}
	sharedJob.Product = prod

	writeJSON(w, http.StatusOK, model.TrackResponse{
		Success: true,
		Data:    sharedJob,
		Message: "Tracked at " + label + " (Active shared cron: " + sharedJob.IntervalLabel + ")",
	})
}

// HandleListTracked returns ONLY the products tracked by the currently logged-in user.
// Each item displays the shared lowest interval active for that product.
func (h *Handler) HandleListTracked(w http.ResponseWriter, r *http.Request) {
	user := h.getUserFromRequest(r)
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"success": false,
			"error":   "please log in to view your tracked products",
			"jobs":    []interface{}{},
		})
		return
	}

	items, err := h.store.GetUserTrackedProducts(user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"count":   len(items),
		"items":   items,
	})
}

// HandleUntrack removes a product from the logged-in user's tracker list,
// and adjusts the shared cron job to the lowest interval of remaining users (or deletes if 0 users).
func (h *Handler) HandleUntrack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "only DELETE or POST allowed"})
		return
	}

	user := h.getUserFromRequest(r)
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"success": false, "error": "unauthenticated"})
		return
	}

	idStr := r.URL.Query().Get("product_id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	productID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || productID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "valid product_id is required"})
		return
	}

	err = h.store.RemoveUserTracker(user.ID, productID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Product removed from your tracked list",
	})
}

// HandleTrackedAction handles run now for a tracked job.
func (h *Handler) HandleTrackedAction(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	jobID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || jobID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "valid id parameter is required",
		})
		return
	}

	path := r.URL.Path

	if strings.HasSuffix(path, "/run") && r.Method == http.MethodPost {
		prod, err := h.sched.RunJobNow(jobID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "Job executed successfully",
			"product": prod,
		})
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "error": "unknown action"})
}

// HandleHistory returns price vs time history and analytics.
func (h *Handler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	idStr := strings.TrimSpace(r.URL.Query().Get("id"))

	var prod *model.Product
	var history []model.PriceHistory
	var stats *model.PriceStats
	var err error

	if idStr != "" {
		id, parseErr := strconv.ParseInt(idStr, 10, 64)
		if parseErr == nil {
			prod, err = h.store.GetProductByID(id)
			if prod != nil {
				history, stats, err = h.store.GetPriceHistory(id)
			}
		}
	} else if rawURL != "" {
		prod, history, stats, err = h.store.GetPriceHistoryByURL(rawURL)
	} else {
		writeJSON(w, http.StatusBadRequest, model.HistoryResponse{
			Success: false,
			Error:   "either 'url' or 'id' query parameter is required",
		})
		return
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.HistoryResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if prod == nil {
		writeJSON(w, http.StatusNotFound, model.HistoryResponse{
			Success: false,
			Error:   "no product found with pricing history",
		})
		return
	}

	currentUser := h.getUserFromRequest(r)
	h.attachUserTrackingContext(prod, currentUser)

	writeJSON(w, http.StatusOK, model.HistoryResponse{
		Success: true,
		Product: prod,
		History: history,
		Stats:   stats,
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
