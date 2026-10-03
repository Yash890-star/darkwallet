document.addEventListener('DOMContentLoaded', () => {
  // DOM Elements - Scraper
  const urlInput = document.getElementById('url-input');
  const scrapeBtn = document.getElementById('scrape-btn');
  const btnText = scrapeBtn.querySelector('.btn-text');
  const btnLoader = scrapeBtn.querySelector('.btn-loader');
  const alertBox = document.getElementById('alert-box');
  const alertText = document.getElementById('alert-text');
  const resultSection = document.getElementById('result-section');
  const latencyTag = document.getElementById('latency-tag');
  const copyJsonBtn = document.getElementById('copy-json-btn');
  const jsonCode = document.getElementById('json-code');
  const historyBadge = document.getElementById('history-badge');

  // DOM Elements - Auth & Header
  const openAuthBtn = document.getElementById('open-auth-btn');
  const userProfileBadge = document.getElementById('user-profile-badge');
  const userAvatarInitial = document.getElementById('user-avatar-initial');
  const userNameDisplay = document.getElementById('user-name-display');
  const logoutBtn = document.getElementById('logout-btn');

  // DOM Elements - Auth Modal
  const authModal = document.getElementById('auth-modal');
  const closeAuthModal = document.getElementById('close-auth-modal');
  const tabLogin = document.getElementById('tab-login');
  const tabRegister = document.getElementById('tab-register');
  const authForm = document.getElementById('auth-form');
  const authUsername = document.getElementById('auth-username');
  const authPassword = document.getElementById('auth-password');
  const authErrorMsg = document.getElementById('auth-error-msg');
  const authSubmitBtn = document.getElementById('auth-submit-btn');
  const authBtnText = authSubmitBtn.querySelector('.auth-btn-text');
  const authBtnLoader = authSubmitBtn.querySelector('.auth-btn-loader');

  // DOM Elements - Product Preview
  const productImg = document.getElementById('product-img');
  const productPlatform = document.getElementById('product-platform');
  const productStock = document.getElementById('product-stock');
  const productAsin = document.getElementById('product-asin');
  const productOriginPill = document.getElementById('product-origin-pill');
  const productSubscribersPill = document.getElementById('product-subscribers-pill');
  const productTitle = document.getElementById('product-title');
  const ratingRow = document.getElementById('rating-row');
  const productRating = document.getElementById('product-rating');
  const productReviews = document.getElementById('product-reviews');
  const productPrice = document.getElementById('product-price');
  const productMrp = document.getElementById('product-mrp');
  const productDiscount = document.getElementById('product-discount');
  const productLink = document.getElementById('product-link');
  const scrapedTime = document.getElementById('scraped-time');

  // DOM Elements - User Tracking Status & Lowest Cron Banner
  const userTrackStatusBox = document.getElementById('user-track-status-box');
  const userTrackIntervalText = document.getElementById('user-track-interval-text');
  const untrackCardBtn = document.getElementById('untrack-card-btn');
  const lowestCronBanner = document.getElementById('lowest-cron-banner');
  const lowestCronText = document.getElementById('lowest-cron-text');

  // DOM Elements - Cron Scheduler Controls
  const intervalSelect = document.getElementById('interval-select');
  const trackBtn = document.getElementById('track-btn');
  const trackBtnText = trackBtn.querySelector('.track-btn-text');
  const jobStatusPill = document.getElementById('job-status-pill');
  const trackFeedback = document.getElementById('track-feedback');

  // DOM Elements - Graph & Stats
  const statLowest = document.getElementById('stat-lowest');
  const statHighest = document.getElementById('stat-highest');
  const statAvg = document.getElementById('stat-avg');
  const statCount = document.getElementById('stat-count');
  const chartCanvas = document.getElementById('priceChart');
  let priceChartInstance = null;

  // DOM Elements - Tracked Wishlist Section
  const trackedSection = document.querySelector('.tracked-section');
  const trackedList = document.getElementById('tracked-list');
  const trackedBadge = document.getElementById('tracked-badge');
  const refreshTrackedBtn = document.getElementById('refresh-tracked-btn');
  const authRequiredBanner = document.getElementById('auth-required-banner');
  const bannerLoginBtn = document.getElementById('banner-login-btn');

  // Application State
  let currentUser = null;
  let authToken = localStorage.getItem('pricelens_token') || '';
  let authMode = 'login'; // 'login' or 'register'
  let currentProduct = null;
  let currentRawData = null;

  // Helper for Auth Headers
  function getAuthHeaders() {
    const headers = { 'Content-Type': 'application/json' };
    if (authToken) {
      headers['Authorization'] = 'Bearer ' + authToken;
    }
    return headers;
  }

  // ==============================================================
  // Authentication Management
  // ==============================================================

  async function checkAuthSession() {
    if (!authToken) {
      updateAuthUI(null);
      fetchUserTrackedProducts();
      return;
    }

    try {
      const res = await fetch('/api/auth/me', {
        headers: getAuthHeaders(),
      });
      const data = await res.json();
      if (res.ok && data.success && data.user) {
        currentUser = data.user;
        updateAuthUI(data.user);
      } else {
        // Token invalid or expired
        authToken = '';
        localStorage.removeItem('pricelens_token');
        currentUser = null;
        updateAuthUI(null);
      }
    } catch (err) {
      console.error('Session verification error:', err);
    } finally {
      fetchUserTrackedProducts();
    }
  }

  function updateAuthUI(user) {
    if (user) {
      openAuthBtn.style.display = 'none';
      userProfileBadge.style.display = 'flex';
      userAvatarInitial.textContent = (user.username || 'U')[0].toUpperCase();
      userNameDisplay.textContent = user.username;
      authRequiredBanner.style.display = 'none';
    } else {
      openAuthBtn.style.display = 'inline-flex';
      userProfileBadge.style.display = 'none';
      authRequiredBanner.style.display = 'block';
    }
  }

  function openAuth(mode = 'login', customMsg = '') {
    authMode = mode;
    authUsername.value = '';
    authPassword.value = '';
    authErrorMsg.style.display = 'none';

    if (mode === 'login') {
      tabLogin.classList.add('active');
      tabRegister.classList.remove('active');
      authBtnText.textContent = 'Sign In';
      authPassword.placeholder = 'Enter password';
    } else {
      tabRegister.classList.add('active');
      tabLogin.classList.remove('active');
      authBtnText.textContent = 'Create Account';
      authPassword.placeholder = 'Choose password (min 6 chars)';
    }

    if (customMsg) {
      authErrorMsg.textContent = customMsg;
      authErrorMsg.style.display = 'block';
      authErrorMsg.style.color = '#38bdf8';
      authErrorMsg.style.background = 'rgba(6, 182, 212, 0.1)';
      authErrorMsg.style.borderColor = 'rgba(6, 182, 212, 0.3)';
    }

    authModal.style.display = 'flex';
    authUsername.focus();
  }

  function closeAuth() {
    authModal.style.display = 'none';
  }

  // Auth Event Listeners
  openAuthBtn.addEventListener('click', () => openAuth('login'));
  if (bannerLoginBtn) {
    bannerLoginBtn.addEventListener('click', () => openAuth('login'));
  }
  closeAuthModal.addEventListener('click', closeAuth);
  authModal.addEventListener('click', (e) => {
    if (e.target === authModal) closeAuth();
  });

  tabLogin.addEventListener('click', () => openAuth('login'));
  tabRegister.addEventListener('click', () => openAuth('register'));

  authForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const username = authUsername.value.trim();
    const password = authPassword.value;

    if (!username || !password) {
      showAuthError('Please fill in both username and password.');
      return;
    }

    authSubmitBtn.disabled = true;
    authBtnText.style.display = 'none';
    authBtnLoader.style.display = 'inline-flex';
    authErrorMsg.style.display = 'none';

    const endpoint = authMode === 'register' ? '/api/auth/register' : '/api/auth/login';

    try {
      const res = await fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      });

      const data = await res.json();
      if (!res.ok || !data.success) {
        showAuthError(data.error || 'Authentication failed. Please check credentials.');
        return;
      }

      // Success
      authToken = data.token;
      localStorage.setItem('pricelens_token', authToken);
      currentUser = data.user;
      updateAuthUI(data.user);
      closeAuth();

      // Refresh product tracking status if product is displayed
      if (currentProduct) {
        triggerScrape(currentProduct.url, false);
      }
      fetchUserTrackedProducts();
    } catch (err) {
      showAuthError('Connection error: ' + err.message);
    } finally {
      authSubmitBtn.disabled = false;
      authBtnText.style.display = 'inline';
      authBtnLoader.style.display = 'none';
    }
  });

  logoutBtn.addEventListener('click', async () => {
    try {
      await fetch('/api/auth/logout', {
        method: 'POST',
        headers: getAuthHeaders(),
      });
    } catch (err) {
      console.warn('Logout error:', err);
    }
    authToken = '';
    currentUser = null;
    localStorage.removeItem('pricelens_token');
    updateAuthUI(null);
    if (currentProduct) {
      triggerScrape(currentProduct.url, false);
    }
    fetchUserTrackedProducts();
  });

  function showAuthError(msg) {
    authErrorMsg.textContent = msg;
    authErrorMsg.style.display = 'block';
    authErrorMsg.style.color = '#fb7185';
    authErrorMsg.style.background = 'rgba(244, 63, 94, 0.1)';
    authErrorMsg.style.borderColor = 'rgba(244, 63, 94, 0.3)';
  }

  // ==============================================================
  // Scraper & Deduplication Handling
  // ==============================================================

  // Handle Quick Test Chips
  document.querySelectorAll('.sample-chip').forEach(chip => {
    chip.addEventListener('click', () => {
      urlInput.value = chip.dataset.url;
      triggerScrape();
    });
  });

  // Handle Enter key in input
  urlInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      triggerScrape();
    }
  });

  // Handle Click on Scrape button
  scrapeBtn.addEventListener('click', () => {
    triggerScrape();
  });

  async function triggerScrape(overrideUrl = '', scrollToResult = true) {
    const rawUrl = overrideUrl || urlInput.value.trim();
    if (!rawUrl) {
      showError('Please enter or paste a valid Amazon product URL.');
      return;
    }

    hideError();
    setLoading(true);

    try {
      const res = await fetch('/api/scrape', {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify({ url: rawUrl }),
      });

      const data = await res.json();
      currentRawData = data;

      if (!res.ok || !data.success) {
        showError(data.error || 'Failed to scrape the product. Please try again.');
        resultSection.style.display = 'none';
        return;
      }

      currentProduct = data.data;
      renderProduct(data.data, data.took_ms, scrollToResult);
      loadPriceHistory(data.data.url);
      fetchUserTrackedProducts();
    } catch (err) {
      showError('Network error connecting to scraper server: ' + err.message);
      resultSection.style.display = 'none';
    } finally {
      setLoading(false);
    }
  }

  function renderProduct(p, tookMs, scrollTo = true) {
    resultSection.style.display = 'block';
    latencyTag.textContent = `⚡ Responded in ${tookMs}ms`;

    // Title & Image
    productTitle.textContent = p.title || 'Untitled Product';
    if (p.image_url) {
      productImg.src = p.image_url;
      productImg.style.display = 'block';
    } else {
      productImg.src = 'https://via.placeholder.com/200x200?text=No+Image';
    }

    // Platform & ASIN
    productPlatform.textContent = p.platform || 'Amazon India';
    if (p.asin) {
      productAsin.textContent = `ASIN: ${p.asin}`;
      productAsin.style.display = 'inline';
    } else {
      productAsin.style.display = 'none';
    }

    // Stock Status
    if (p.in_stock) {
      productStock.textContent = p.availability || 'In Stock';
      productStock.className = 'stock-badge in-stock';
    } else {
      productStock.textContent = p.availability || 'Currently Unavailable';
      productStock.className = 'stock-badge out-of-stock';
    }

    // Rating
    if (p.rating) {
      ratingRow.style.display = 'flex';
      productRating.textContent = p.rating;
      productReviews.textContent = p.review_count ? `(${p.review_count})` : '';
    } else {
      ratingRow.style.display = 'none';
    }

    // Pricing
    productPrice.textContent = p.formatted_price || `${p.currency}${p.current_price}`;
    if (p.original_price && p.original_price > p.current_price) {
      productMrp.textContent = p.formatted_mrp || `${p.currency}${p.original_price}`;
      productMrp.style.display = 'inline';
    } else {
      productMrp.style.display = 'none';
    }

    if (p.discount) {
      productDiscount.textContent = p.discount;
      productDiscount.style.display = 'inline-block';
    } else {
      productDiscount.style.display = 'none';
    }

    // Links & Timestamps
    productLink.href = p.url;
    const date = new Date(p.scraped_at);
    scrapedTime.textContent = `Updated: ${date.toLocaleTimeString()}`;

    // Origin / Deduplication Badges
    const hasCron = !!p.tracked_job || !!p.effective_cron;
    if (hasCron) {
      productOriginPill.className = 'origin-pill cached';
      productOriginPill.innerHTML = '⚡ Instant from DB (Shared Cron Active)';
    } else {
      productOriginPill.className = 'origin-pill live';
      productOriginPill.innerHTML = '🌐 Fresh Scraped Price (No Crons)';
    }

    if (p.tracked_users && p.tracked_users > 0) {
      productSubscribersPill.style.display = 'inline-flex';
      productSubscribersPill.textContent = `👥 ${p.tracked_users} ${p.tracked_users === 1 ? 'Subscriber' : 'Subscribers'}`;
    } else {
      productSubscribersPill.style.display = 'none';
    }

    // User-specific tracking status
    if (p.is_user_tracked) {
      userTrackStatusBox.style.display = 'flex';
      const userIntervalLabel = getIntervalLabel(p.user_interval);
      userTrackIntervalText.textContent = `Your Interval Preference: ${userIntervalLabel}`;
      trackBtnText.textContent = 'Update My Cron Interval';
    } else {
      userTrackStatusBox.style.display = 'none';
      trackBtnText.textContent = 'Add to Wishlist & Track';
    }

    // Lowest Cron Notice
    if (p.effective_cron) {
      lowestCronBanner.style.display = 'flex';
      if (p.tracked_users > 1) {
        lowestCronText.textContent = `Shared Lowest Cron: Multiple users tracking this item. Shared background check is running at lowest frequency: ${p.effective_cron}.`;
      } else {
        lowestCronText.textContent = `Active Shared Cron: Background check scheduled to run ${p.effective_cron}.`;
      }
    } else {
      lowestCronBanner.style.display = 'none';
    }

    // Synchronize select dropdown
    const effectiveInterval = (p.user_interval || (p.tracked_job && p.tracked_job.interval) || '').toLowerCase();
    for (let i = 0; i < intervalSelect.options.length; i++) {
      if (intervalSelect.options[i].value.toLowerCase() === effectiveInterval || intervalSelect.options[i].text.toLowerCase() === effectiveInterval) {
        intervalSelect.selectedIndex = i;
        break;
      }
    }

    // Cron tracking status pill
    updateTrackedStatusPill(p.tracked_job, p.effective_cron);

    // Raw JSON tab
    jsonCode.textContent = JSON.stringify(currentRawData, null, 2);

    if (scrollTo) {
      resultSection.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }
  }

  function updateTrackedStatusPill(job, effectiveLabel) {
    if (!job && !effectiveLabel) {
      jobStatusPill.style.display = 'none';
      return;
    }

    jobStatusPill.style.display = 'inline-block';
    const label = effectiveLabel || (job && job.interval_label) || 'Active';
    const status = (job && job.status) || 'active';

    if (status === 'active') {
      jobStatusPill.className = 'job-status-pill active';
      jobStatusPill.textContent = `Active (${label})`;
    } else {
      jobStatusPill.className = 'job-status-pill paused';
      jobStatusPill.textContent = `Paused (${label})`;
    }
  }

  // ==============================================================
  // Tracking & Wishlist Actions
  // ==============================================================

  // Handle Track / Wishlist Button
  trackBtn.addEventListener('click', async () => {
    if (!currentUser) {
      openAuth('login', 'Please sign in or register to add this product to your personal watchlist.');
      return;
    }

    if (!currentProduct || !currentProduct.url) {
      showError('Please scrape a product first before scheduling tracking.');
      return;
    }

    const interval = intervalSelect.value;
    trackBtn.disabled = true;
    trackBtnText.textContent = 'Saving...';

    try {
      const res = await fetch('/api/track', {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify({
          url: currentProduct.url,
          interval: interval,
        }),
      });

      const data = await res.json();
      if (!res.ok || !data.success) {
        showTrackFeedback(data.error || 'Failed to track product', true);
        return;
      }

      currentProduct.tracked_job = data.data;
      currentProduct.is_user_tracked = true;
      currentProduct.user_interval = interval;
      currentProduct.effective_cron = data.data.interval_label;

      showTrackFeedback(`✓ ${data.message || 'Product tracked successfully!'}`, false);
      renderProduct(currentProduct, 0, false);
      fetchUserTrackedProducts();
    } catch (err) {
      showTrackFeedback('Error: ' + err.message, true);
    } finally {
      trackBtn.disabled = false;
      trackBtnText.textContent = 'Update My Cron Interval';
    }
  });

  // Handle Untrack from Product Card
  untrackCardBtn.addEventListener('click', async () => {
    if (!currentProduct || !currentProduct.id) return;
    if (!confirm('Remove this product from your tracked wishlist?')) return;

    untrackCardBtn.disabled = true;
    untrackCardBtn.textContent = 'Removing...';

    try {
      const res = await fetch(`/api/track/untrack?product_id=${currentProduct.id}`, {
        method: 'POST',
        headers: getAuthHeaders(),
      });
      const data = await res.json();
      if (res.ok && data.success) {
        currentProduct.is_user_tracked = false;
        triggerScrape(currentProduct.url, false);
        fetchUserTrackedProducts();
      } else {
        alert(data.error || 'Failed to untrack product');
      }
    } catch (err) {
      console.error('Untrack error:', err);
    } finally {
      untrackCardBtn.disabled = false;
      untrackCardBtn.textContent = 'Remove from Wishlist';
    }
  });

  // Fetch Personalized Wishlist / Tracked List
  async function fetchUserTrackedProducts() {
    if (!currentUser) {
      trackedBadge.textContent = '0';
      trackedList.innerHTML = `
        <div class="empty-tracked">
          <p>Please <strong>Sign In</strong> to view and manage your personal price-tracking wishlist.</p>
        </div>
      `;
      authRequiredBanner.style.display = 'block';
      return;
    }

    authRequiredBanner.style.display = 'none';

    try {
      const res = await fetch('/api/tracked', {
        headers: getAuthHeaders(),
      });
      const data = await res.json();
      if (!res.ok || !data.success) {
        if (res.status === 401) {
          currentUser = null;
          updateAuthUI(null);
        }
        return;
      }

      const items = data.items || [];
      trackedBadge.textContent = items.length;

      if (items.length === 0) {
        trackedList.innerHTML = `
          <div class="empty-tracked">
            <p>Your wishlist is empty. Paste an Amazon India link above and click <strong>Add to Wishlist & Track</strong> to start tracking!</p>
          </div>
        `;
        return;
      }

      trackedList.innerHTML = '';
      items.forEach(item => {
        const card = document.createElement('div');
        card.className = 'tracked-card';

        const thumb = item.image_url || 'https://via.placeholder.com/56x56?text=No+Img';
        const priceStr = item.formatted_price || (item.current_price ? `${item.currency || '₹'}${item.current_price}` : 'Checking...');
        const isPaused = item.cron_status === 'paused';
        const statusBadge = isPaused
          ? `<span class="job-status-pill paused">Paused</span>`
          : `<span class="job-status-pill active">Active</span>`;

        const userPrefLabel = getIntervalLabel(item.user_desired_interval);
        const effectiveLabel = item.effective_interval_label || 'Every 24 hrs';

        card.innerHTML = `
          <img src="${thumb}" alt="thumb" class="tracked-thumb">
          <div class="tracked-info">
            <div class="tracked-title" title="${escapeHtml(item.title || item.url)}">${escapeHtml(item.title || item.url)}</div>
            <div class="tracked-meta">
              <span class="user-pref-tag" title="Your chosen frequency">My preference: ${escapeHtml(userPrefLabel)}</span>
              <span class="lowest-cron-tag" title="Active background cron running at lowest subscriber frequency">⚡ Shared Cron: ${escapeHtml(effectiveLabel)}</span>
              <span class="price-tag">${priceStr}</span>
              ${statusBadge}
              <span class="next-check">Next check: ${formatDate(item.next_run_at)}</span>
            </div>
          </div>
          <div class="tracked-actions">
            <button class="action-btn btn-graph" data-url="${escapeHtml(item.url)}">📈 View Graph</button>
            <button class="action-btn btn-delete" data-id="${item.product_id}">✕ Untrack</button>
          </div>
        `;

        // Graph Button
        card.querySelector('.btn-graph').addEventListener('click', () => {
          urlInput.value = item.url;
          triggerScrape(item.url, true);
          const graphTabBtn = document.getElementById('graph-tab-btn');
          if (graphTabBtn) graphTabBtn.click();
        });

        // Untrack Button
        card.querySelector('.btn-delete').addEventListener('click', async (e) => {
          if (!confirm(`Remove "${item.title || 'this product'}" from your tracked wishlist?`)) return;
          const btn = e.currentTarget;
          btn.disabled = true;
          try {
            const resp = await fetch(`/api/track/untrack?product_id=${item.product_id}`, {
              method: 'POST',
              headers: getAuthHeaders(),
            });
            const d = await resp.json();
            if (d.success) {
              fetchUserTrackedProducts();
              if (currentProduct && currentProduct.id === item.product_id) {
                triggerScrape(currentProduct.url, false);
              }
            } else {
              alert(d.error || 'Failed to remove tracker');
            }
          } catch (err) {
            console.error('Untrack error:', err);
          }
        });

        trackedList.appendChild(card);
      });
    } catch (err) {
      console.error('Failed to fetch user tracked products:', err);
    }
  }

  refreshTrackedBtn.addEventListener('click', () => {
    fetchUserTrackedProducts();
  });

  // ==============================================================
  // Price History & Chart.js Visualization
  // ==============================================================

  async function loadPriceHistory(url) {
    try {
      const res = await fetch('/api/history?url=' + encodeURIComponent(url));
      const data = await res.json();
      if (!res.ok || !data.success) return;

      const history = data.history || [];
      const stats = data.stats || {};
      historyBadge.textContent = history.length;

      // Update stat cards
      const curr = currentProduct ? currentProduct.currency : '₹';
      statLowest.textContent = stats.lowest_price ? `${curr}${formatNumber(stats.lowest_price)}` : '--';
      statHighest.textContent = stats.highest_price ? `${curr}${formatNumber(stats.highest_price)}` : '--';
      statAvg.textContent = stats.average_price ? `${curr}${formatNumber(stats.average_price)}` : '--';
      statCount.textContent = stats.total_checks || history.length;

      renderChart(history, curr);
    } catch (err) {
      console.error('Failed to load price history:', err);
    }
  }

  function renderChart(history, currency) {
    if (!chartCanvas) return;

    if (priceChartInstance) {
      priceChartInstance.destroy();
    }

    if (!history || history.length === 0) {
      return;
    }

    const labels = history.map(h => {
      const d = new Date(h.recorded_at);
      return d.toLocaleDateString([], { month: 'short', day: 'numeric' }) + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    });

    const prices = history.map(h => h.current_price);

    const ctx = chartCanvas.getContext('2d');
    const gradient = ctx.createLinearGradient(0, 0, 0, 260);
    gradient.addColorStop(0, 'rgba(16, 185, 129, 0.35)');
    gradient.addColorStop(1, 'rgba(16, 185, 129, 0.0)');

    priceChartInstance = new Chart(ctx, {
      type: 'line',
      data: {
        labels: labels,
        datasets: [{
          label: 'Price',
          data: prices,
          borderColor: '#10b981',
          borderWidth: 2.5,
          backgroundColor: gradient,
          fill: true,
          tension: 0.3,
          pointBackgroundColor: '#10b981',
          pointBorderColor: '#ffffff',
          pointBorderWidth: 1.5,
          pointRadius: history.length === 1 ? 6 : 4,
          pointHoverRadius: 7,
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: {
          mode: 'index',
          intersect: false,
        },
        plugins: {
          legend: {
            display: false,
          },
          tooltip: {
            backgroundColor: '#0f172a',
            titleColor: '#94a3b8',
            bodyColor: '#34d399',
            bodyFont: {
              family: 'JetBrains Mono',
              size: 13,
              weight: 'bold',
            },
            borderColor: 'rgba(255, 255, 255, 0.1)',
            borderWidth: 1,
            padding: 10,
            displayColors: false,
            callbacks: {
              label: function(context) {
                return `Price: ${currency}${formatNumber(context.parsed.y)}`;
              }
            }
          }
        },
        scales: {
          x: {
            grid: {
              color: 'rgba(255, 255, 255, 0.04)',
            },
            ticks: {
              color: '#64748b',
              font: {
                size: 10,
              },
              maxRotation: 45,
            }
          },
          y: {
            grid: {
              color: 'rgba(255, 255, 255, 0.05)',
            },
            ticks: {
              color: '#94a3b8',
              font: {
                family: 'JetBrains Mono',
                size: 11,
              },
              callback: function(value) {
                return currency + formatNumber(value);
              }
            }
          }
        }
      }
    });
  }

  // ==============================================================
  // UI Helpers & Tab Switching
  // ==============================================================

  // Tabs
  document.querySelectorAll('.tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
      document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));

      btn.classList.add('active');
      const tabTarget = document.getElementById(btn.dataset.tab);
      if (tabTarget) {
        tabTarget.classList.add('active');
        if (btn.dataset.tab === 'graph-view' && priceChartInstance) {
          priceChartInstance.resize();
        }
      }
    });
  });

  // Copy JSON
  copyJsonBtn.addEventListener('click', () => {
    if (!currentRawData) return;
    navigator.clipboard.writeText(JSON.stringify(currentRawData, null, 2)).then(() => {
      const orig = copyJsonBtn.innerHTML;
      copyJsonBtn.innerHTML = '✓ Copied!';
      setTimeout(() => {
        copyJsonBtn.innerHTML = orig;
      }, 2000);
    });
  });

  // Code Snippet Copies
  document.querySelectorAll('.snippet-copy-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const type = btn.dataset.snippet;
      const codeEl = document.getElementById(`snippet-${type}`);
      if (!codeEl) return;

      navigator.clipboard.writeText(codeEl.innerText).then(() => {
        const span = btn.querySelector('span');
        const origText = span.textContent;
        span.textContent = 'Copied!';
        btn.style.color = '#34d399';
        btn.style.borderColor = 'rgba(52, 211, 153, 0.4)';
        setTimeout(() => {
          span.textContent = origText;
          btn.style.color = '';
          btn.style.borderColor = '';
        }, 1800);
      });
    });
  });

  function showTrackFeedback(msg, isError) {
    trackFeedback.style.display = 'block';
    trackFeedback.style.color = isError ? '#fb7185' : '#34d399';
    trackFeedback.textContent = msg;
    setTimeout(() => {
      trackFeedback.style.display = 'none';
    }, 4500);
  }

  function setLoading(isLoading) {
    if (isLoading) {
      scrapeBtn.disabled = true;
      btnText.style.display = 'none';
      btnLoader.style.display = 'inline-flex';
    } else {
      scrapeBtn.disabled = false;
      btnText.style.display = 'inline';
      btnLoader.style.display = 'none';
    }
  }

  function showError(msg) {
    alertText.textContent = msg;
    alertBox.style.display = 'flex';
  }

  function hideError() {
    alertBox.style.display = 'none';
  }

  function formatNumber(num) {
    return Number(num).toLocaleString('en-IN');
  }

  function formatDate(dateStr) {
    if (!dateStr) return 'Pending';
    const d = new Date(dateStr);
    const now = new Date();
    const diffMs = d - now;
    if (diffMs > 0 && diffMs < 24 * 60 * 60 * 1000) {
      const hours = Math.floor(diffMs / (60 * 60 * 1000));
      const mins = Math.floor((diffMs % (60 * 60 * 1000)) / (60 * 1000));
      return `in ${hours}h ${mins}m`;
    }
    return d.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  }

  function getIntervalLabel(key) {
    const map = {
      '6h': 'Every 6 hrs',
      '12h': 'Every 12 hrs',
      '24h': 'Every 24 hrs',
      '2d': 'Every 2 days',
      '1w': 'Every 1 week',
      '1m': 'Every 1 month',
      'every 6hrs': 'Every 6 hrs',
      'every 12 hrs': 'Every 12 hrs',
      'every 24 hrs': 'Every 24 hrs',
      'every 2 days': 'Every 2 days',
      'every 1 week': 'Every 1 week',
      'every 1 month': 'Every 1 month',
    };
    return map[key] || key || 'Every 24 hrs';
  }

  function escapeHtml(str) {
    if (!str) return '';
    return str.replace(/&/g, '&amp;')
              .replace(/</g, '&lt;')
              .replace(/>/g, '&gt;')
              .replace(/"/g, '&quot;');
  }

  // Start by verifying auth session
  checkAuthSession();
});
