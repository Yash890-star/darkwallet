# 🛍️ PriceLens - E-Commerce Price Scraper & Scheduled Tracker (Go + SQLite)

A fast, concurrent, and extensible web scraping and price-tracking backend built with **Go**, **modernc.org/sqlite** (pure Go SQLite), and **GoQuery**. Given a product URL (specialized for **Amazon India** and extensible to any platform), it scrapes real-time pricing, maintains a historical audit log in SQLite, runs automated cron jobs at custom intervals, and plots interactive **Price vs. Time** graphs.

---

## ✨ Features

- **🚀 Real-Time Price Extraction (Amazon India)**:
  - Extracts **Current Selling Price**, **Original Price / MRP**, **Discount %**, **Availability**, **Ratings & Reviews**, and **Image**.
  - **ASIN Canonicalization**: Strips tracking query parameters to clean canonical URLs.
  - Multi-tier fallback extraction (JSON-LD Schema, Amazon Core Price DOM selectors, Off-screen accessibility spans).
- **📦 SQLite Persistent Storage**:
  - Automatically records historical price snapshots on every scrape (manual or cron).
  - Calculates analytics: **Lowest Recorded Price**, **Highest Recorded Price**, **Average Price**, and **Total Checks**.
- **⏱️ Automated Background Cron Scheduler**:
  - Built-in lightweight scheduler with zero external cron daemon dependencies.
  - Supported recurring intervals:
    - `every 6hrs` (6 hours)
    - `every 12 hrs` (12 hours)
    - `every 24 hrs` (24 hours - Default)
    - `every 2 days` (48 hours)
    - `every 1 week` (7 days)
    - `every 1 month` (30 days)
  - **🚀 Server Startup Auto-Scan**: On server startup, automatically evaluates all tracked products. If `(now - latest scan) >= configured cron interval` (or if no scan exists yet), triggers an immediate fresh scan so no intervals are missed during server downtime.
  - Features: Pause/Resume, Run Now on-demand, and Delete.
- **🔐 User Authentication & Scoped Tracking**:
  - Secure username/password authentication with `bcrypt` password hashing and session tokens.
  - User-scoped wishlist/tracking: each user only sees products they have chosen to track.
- **🔄 Smart Product Deduplication & Lowest-Interval Cron**:
  - When multiple users track the same product (e.g., Casio watch), the product is deduplicated in SQLite (`UNIQUE` canonical URL).
  - Background cron runs at the **lowest interval** selected among all users tracking that product (e.g., User A wants 24h, User B wants 6h -> background cron runs every 6h).
  - Both users see the effective lowest cron interval on their dashboards.
  - When a user untracks a product, the shared cron automatically recalculates to the lowest interval of the remaining users (or deletes the cron if 0 users remain).
  - **Fresh Price vs DB Cache**: If a product has active cron jobs, queries serve from SQLite without redundant web requests. If no active crons are set, an immediate fresh live scrape is performed.
- **📈 Price vs. Time Interactive Graph**:
  - Rendered via **Chart.js** with dark mode styling, emerald gradient fills, and price tooltips.
  - Visual summary cards showing lowest, highest, and average prices.
- **🔌 Highly Extensible Architecture**:
  - Clean `Scraper` interface and `Registry` pattern to add Flipkart, Myntra, etc.

---

## 📁 Project Structure

```
.
├── cmd/
│   └── server/
│       └── main.go                 # Server entrypoint & graceful shutdown
├── internal/
│   ├── api/
│   │   ├── handler.go              # Scrape, Track, History & Cron endpoints
│   │   └── handler_test.go         # API integration tests
│   ├── model/
│   │   └── product.go              # Product, PriceHistory & TrackedJob models
│   ├── scheduler/
│   │   ├── scheduler.go            # Background cron runner & interval parser
│   │   └── scheduler_test.go       # Interval parsing unit tests
│   ├── storage/
│   │   ├── sqlite.go               # SQLite database queries & migrations
│   │   └── sqlite_test.go          # Database unit tests
│   └── scraper/
│       ├── scraper.go              # Common Scraper interface definition
│       ├── registry.go             # Platform registry router
│       ├── client.go               # Browser header emulation & price parsing
│       ├── amazon.go               # Specialized Amazon India scraper
│       └── generic.go              # Generic OpenGraph / Schema.org fallback
├── web/
│   ├── index.html                  # Dashboard with Chart.js & Cron controls
│   └── static/
│       ├── style.css               # Cyber dark theme styles
│       └── app.js                  # Frontend logic, chart & cron managers
├── tracker.db                      # SQLite database file (auto-created)
├── go.mod
├── go.sum
└── README.md
```

---

## 🚀 Running the Server

### 1. Prerequisites
- **Go 1.21+** installed (`go version`).
- **No CGO or GCC required**: Built using `modernc.org/sqlite`, a pure-Go SQLite driver that compiles and runs natively across Windows, macOS, and Linux without any external C dependencies.

### 2. Start the Server

#### Option A: Run directly with Go
```powershell
go run ./cmd/server
```

#### Option B: Build and run the compiled binary
```powershell
# Build executable
go build -o ecom-server.exe ./cmd/server

# Run executable
./ecom-server.exe
```

### 3. Optional Environment Configuration
You can customize the port and SQLite database location:
```powershell
$env:PORT = "8080"
$env:DB_PATH = "tracker.db"
./ecom-server.exe
```

### 4. Access the Application
Once running, the server is available at:
- **Web UI & Dashboard**: [http://localhost:8080](http://localhost:8080)
- **Health Check**: [http://localhost:8080/health](http://localhost:8080/health)
- **Auth Endpoints**:
  - `POST /api/auth/register` — Create account & get token
  - `POST /api/auth/login` — Sign in & get token
  - `POST /api/auth/logout` — Terminate session
  - `GET  /api/auth/me` — Authenticated profile
- **Scrape API Endpoint**: `POST /api/scrape`
- **Schedule Tracking**: `POST /api/track`
- **Untrack Product**: `POST /api/track/untrack?product_id={id}`
- **User Wishlist / Tracked Products**: `GET /api/tracked`
- **Price History API**: `GET /api/history?url=...`

---

## 📡 API Reference

### 1. Scrape Product Price
- **Method**: `POST`
- **URL**: `/api/scrape`
- **Body**:
  ```json
  { "url": "https://www.amazon.in/dp/B07YCTCMFK" }
  ```

### 2. Schedule a Recurring Cron Job
- **Method**: `POST`
- **URL**: `/api/track`
- **Body**:
  ```json
  {
    "url": "https://www.amazon.in/dp/B07YCTCMFK",
    "interval": "every 6hrs"
  }
  ```
  *(Supported intervals: `"every 6hrs"`, `"every 12 hrs"`, `"every 24 hrs"`, `"every 2 days"`, `"every 1 week"`, `"every 1 month"`)*

### 3. List All Tracked Products
- **Method**: `GET`
- **URL**: `/api/tracked`

### 4. Trigger Immediate Check for a Tracked Job
- **Method**: `POST`
- **URL**: `/api/tracked/run?id=1`

### 5. Pause or Resume a Tracked Job
- **Method**: `POST`
- **URL**: `/api/tracked/toggle?id=1`

### 6. Delete a Tracked Job
- **Method**: `DELETE`
- **URL**: `/api/tracked?id=1`

### 7. Fetch Price vs. Time History for Graphing
- **Method**: `GET`
- **URL**: `/api/history?url=https://www.amazon.in/dp/B07YCTCMFK`
- **Response**:
  ```json
  {
    "success": true,
    "product": { ... },
    "history": [
      {
        "id": 1,
        "current_price": 9095,
        "original_price": 9195,
        "discount": "-1%",
        "recorded_at": "2026-10-03T08:20:41Z"
      }
    ],
    "stats": {
      "lowest_price": 9095,
      "highest_price": 9195,
      "average_price": 9145,
      "total_checks": 2,
      "price_change": -100
    }
  }
  ```

---

## 🧪 Running Tests
```powershell
go test -v ./...
```
All unit and integration tests run in pure Go with zero CGO dependencies.
