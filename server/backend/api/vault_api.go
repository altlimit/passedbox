package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/altlimit/dsorm"
	rs "github.com/altlimit/restruct"
	"github.com/google/uuid"
	"passedbox.com/model"
	"passedbox.com/push"
)

type VaultAPI struct {
	DB   *dsorm.Client `route:"-"`
	Push *push.Sender  `route:"-"`
	Auth *AuthConfig   `route:"-"`
}

func (v *VaultAPI) Middlewares() []rs.Middleware {
	if v.Auth == nil {
		return nil
	}
	return []rs.Middleware{NewAuthMiddleware(v.Auth, v.DB, []AuthRoute{
		// Public endpoints (no auth)
		{Path: "login", Methods: []string{"POST"}, Level: AuthNone},
		{Path: "logout", Methods: []string{"POST"}, Level: AuthNone},
		{Path: "info", Methods: []string{"GET"}, Level: AuthNone},
		{Path: "vaults", Methods: []string{"POST"}, Level: AuthNone},
		{Path: "stripe/webhook", Methods: []string{"POST"}, Level: AuthNone},
		{Path: "vaults/{id}/checkin", Level: AuthNone},
		{Path: "vaults/{id}/buy", Level: AuthNone},
		{Path: "vaults/{id}/confirm-payment", Level: AuthNone},
		{Path: "vaults/{id}/share", Methods: []string{"GET"}, Level: AuthNone},
		{Path: "vaults/{id}/calendar.ics", Methods: []string{"GET"}, Level: AuthNone},
		{Path: "vaults/{id}/push/subscribe", Methods: []string{"POST"}, Level: AuthNone},
		{Path: "vaults/{id}/push/unsubscribe", Methods: []string{"POST"}, Level: AuthNone},
		{Path: "push/vapid-key", Methods: []string{"GET"}, Level: AuthNone},
		// Token-auth (vault token or admin JWT)
		{Path: "vaults/{id}", Methods: []string{"GET"}, Level: AuthToken},
		{Path: "vaults/{id}", Methods: []string{"PUT"}, Level: AuthToken},
		{Path: "vaults/{id}", Methods: []string{"DELETE"}, Level: AuthToken},
		{Path: "vaults/{id}/deactivate", Methods: []string{"POST"}, Level: AuthToken},
	})}
}

func (v *VaultAPI) Routes() []rs.Route {
	return []rs.Route{
		// Auth endpoints (public)
		{Handler: "Login", Path: "login", Methods: []string{"POST"}},
		{Handler: "Logout", Path: "logout", Methods: []string{"POST"}},
		// Public endpoints
		{Handler: "ServerInfo", Path: "info", Methods: []string{"GET"}},
		{Handler: "RegisterVault", Path: "vaults", Methods: []string{"POST"}},
		{Handler: "CheckIn", Path: "vaults/{id}/checkin", Methods: []string{"POST", "GET"}},
		{Handler: "GetShare", Path: "vaults/{id}/share", Methods: []string{"GET"}},
		{Handler: "CalendarICS", Path: "vaults/{id}/calendar.ics", Methods: []string{"GET"}},
		{Handler: "PushSubscribe", Path: "vaults/{id}/push/subscribe", Methods: []string{"POST"}},
		{Handler: "PushUnsubscribe", Path: "vaults/{id}/push/unsubscribe", Methods: []string{"POST"}},
		{Handler: "VAPIDPublicKey", Path: "push/vapid-key", Methods: []string{"GET"}},
		// Token-auth (vault token or admin JWT)
		{Handler: "GetVault", Path: "vaults/{id}", Methods: []string{"GET"}},
		{Handler: "UpdateVault", Path: "vaults/{id}", Methods: []string{"PUT"}},
		// Admin-only (JWT — default middleware)
		{Handler: "GetSettings", Path: "settings", Methods: []string{"GET"}},
		{Handler: "UpdateSettings", Path: "settings", Methods: []string{"PUT"}},
		{Handler: "DeleteVault", Path: "vaults/{id}", Methods: []string{"DELETE"}},
		{Handler: "ReleaseVault", Path: "vaults/{id}/release", Methods: []string{"POST"}},
		{Handler: "AddCredits", Path: "vaults/{id}/credits", Methods: []string{"POST"}},
		{Handler: "ListVaults", Path: "vaults", Methods: []string{"GET"}},
	}
}

// Login authenticates with the admin password and returns a JWT session cookie.
// POST /api/v1/login
func (v *VaultAPI) Login(r *http.Request, req struct {
	Password string `json:"password"`
}, w http.ResponseWriter) any {

	if err := RateLimitByIP(r, "login", 10, time.Minute); err != nil {
		return err
	}

	if v.Auth == nil || !v.Auth.CheckPassword(req.Password) {
		return rs.Error{Message: "Invalid password"}
	}

	token, err := v.Auth.CreateJWT()
	if err != nil {
		return rs.Error{Message: "Failed to create session"}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400, // 24 hours
	})

	return map[string]any{"ok": true}
}

// Logout clears the session cookie.
// POST /api/v1/logout
func (v *VaultAPI) Logout(w http.ResponseWriter) any {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	return map[string]any{"ok": true}
}

// RegisterVault handles vault registration. A single exists check is performed:
//   - If the vault exists and the request token matches, the vault is re-assumed
//     (credits, status, and token are preserved). Released vaults are reset to pending.
//   - If the vault does not exist, a new pending vault is created.
//   - If PaymentEnabled and the vault has no credits, a Stripe checkout session is created.
//
// POST /api/v1/vaults
func (v *VaultAPI) RegisterVault(r *http.Request, req struct {
	ID              string `json:"id"`
	Token           string `json:"token"`
	Share3Enc       []byte `json:"share3Enc"`
	ReleaseOnExpiry bool   `json:"releaseOnExpiry"`
	EnableKeepAlive bool   `json:"enableKeepAlive"`
	KeepAliveDays   int64  `json:"keepAliveDays"`
	Years           int64  `json:"years"`
}) (any, error) {
	if err := RateLimitByIP(r, "register", 5, time.Minute); err != nil {
		return nil, err
	}
	ctx := r.Context()

	if req.ID == "" {
		return nil, fmt.Errorf("vault id is required")
	}
	if len(req.Share3Enc) == 0 {
		return nil, fmt.Errorf("share3Enc is required")
	}

	keepAliveDays := req.KeepAliveDays
	if keepAliveDays <= 0 {
		keepAliveDays = 30 // default 30 days
	}

	years := req.Years
	if years <= 0 {
		years = 1
	}

	// Load server config to check payment mode
	config, err := model.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load config")
	}

	// Single exists check
	vault := &model.Vault{ID: req.ID}
	existsErr := v.DB.Get(ctx, vault)
	exists := existsErr == nil

	if exists {
		// Vault exists — require matching token to re-assume
		if req.Token == "" || vault.Token != req.Token {
			return nil, rs.Error{
				Message: fmt.Sprintf("vault %s exists", req.ID),
				Status:  http.StatusConflict,
			}
		}

		// Re-assume: update share data and DMS settings, keep token and credits
		vault.Share3Enc = req.Share3Enc
		vault.ReleaseOnExpiry = req.ReleaseOnExpiry
		vault.EnableKeepAlive = req.EnableKeepAlive
		vault.KeepAliveDays = keepAliveDays
		vault.LastCheckIn = time.Now()

		// Reset released state if needed
		if vault.Status == "released" {
			vault.Released = false
			vault.ReleasedAt = time.Time{}
			vault.Status = "pending"
		} else if vault.Status == "inactive" {
			vault.Status = "active"
		}
		// Keep existing status (active stays active, pending stays pending)
	} else {
		// New vault
		vault = &model.Vault{
			ID:              req.ID,
			Status:          "pending",
			Share3Enc:       req.Share3Enc,
			ReleaseOnExpiry: req.ReleaseOnExpiry,
			EnableKeepAlive: req.EnableKeepAlive,
			KeepAliveDays:   keepAliveDays,
			LastCheckIn:     time.Now(),
			Token:           uuid.New().String(),
		}
	}

	if err := v.DB.Put(ctx, vault); err != nil {
		return nil, fmt.Errorf("failed to register vault: %w", err)
	}

	// Payment mode: create checkout if no credits
	var checkoutURL string
	if config.PaymentEnabled && vault.Credits <= 0 {
		sessionID, url, err := v.CreateCheckoutSession(ctx, vault.ID, years)
		if err != nil {
			if !exists {
				v.DB.Delete(ctx, vault)
			}
			return nil, fmt.Errorf("failed to create checkout: %w", err)
		}
		checkoutURL = url

		vault.AddPendingSession(sessionID)
		if err := v.DB.Put(ctx, vault); err != nil {
			return nil, fmt.Errorf("failed to create vault: %w", err)
		}
	}

	return &rs.Json{
		Status: http.StatusCreated,
		Content: map[string]any{
			"id":          vault.ID,
			"token":       vault.Token,
			"status":      vault.Status,
			"checkoutUrl": checkoutURL,
		},
	}, nil
}

// ListVaults returns paginated vaults ordered by updatedAt desc.
// GET /api/v1/vaults?limit=20&cursor=...
func (v *VaultAPI) ListVaults(ctx context.Context, r *http.Request) (any, error) {
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	cursor := r.URL.Query().Get("cursor")

	q := dsorm.NewQuery("Vault").Order("-modified").Limit(limit)
	if cursor != "" {
		q = q.Start(cursor)
	}

	vaults, nextCursor, err := dsorm.Query[*model.Vault](ctx, v.DB, q, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list vaults: %w", err)
	}

	// Strip sensitive data
	result := make([]map[string]any, 0, len(vaults))
	for _, vault := range vaults {
		result = append(result, map[string]any{
			"id":              vault.ID,
			"status":          vault.Status,
			"releaseOnExpiry": vault.ReleaseOnExpiry,
			"enableKeepAlive": vault.EnableKeepAlive,
			"keepAliveDays":   vault.KeepAliveDays,
			"lastCheckIn":     vault.LastCheckIn,
			"released":        vault.Released,
			"releasedAt":      vault.ReleasedAt,
			"createdAt":       vault.CreatedAt,
			"updatedAt":       vault.UpdatedAt,
		})
	}

	return map[string]any{
		"vaults": result,
		"cursor": nextCursor,
	}, nil
}

// GetVault returns vault status (without share3).
// GET /api/v1/vaults/{id}
func (v *VaultAPI) GetVault(ctx context.Context) (any, error) {
	id := rs.Vars(ctx)["id"]

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(ctx, vault); err != nil {
		return nil, fmt.Errorf("vault not found")
	}

	return map[string]any{
		"id":              vault.ID,
		"status":          vault.Status,
		"releaseOnExpiry": vault.ReleaseOnExpiry,
		"enableKeepAlive": vault.EnableKeepAlive,
		"keepAliveDays":   vault.KeepAliveDays,
		"lastCheckIn":     vault.LastCheckIn,
		"released":        vault.Released,
		"releasedAt":      vault.ReleasedAt,
		"credits":         vault.Credits,
		"releaseDate":     vault.ReleaseDate,
		"creditsActive":   vault.ReleaseDate.After(time.Now()),
		"createdAt":       vault.CreatedAt,
		"updatedAt":       vault.UpdatedAt,
	}, nil
}

// UpdateVault updates DMS settings for a vault.
// PUT /api/v1/vaults/{id}
func (v *VaultAPI) UpdateVault(ctx context.Context, req struct {
	ReleaseOnExpiry *bool  `json:"releaseOnExpiry"`
	EnableKeepAlive *bool  `json:"enableKeepAlive"`
	KeepAliveDays   *int64 `json:"keepAliveDays"`
}) (any, error) {
	id := rs.Vars(ctx)["id"]

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(ctx, vault); err != nil {
		return nil, fmt.Errorf("vault not found")
	}

	if vault.Status != "active" {
		return nil, rs.Error{
			Status:  http.StatusBadRequest,
			Message: "vault must be active to update settings",
		}
	}

	if req.ReleaseOnExpiry != nil {
		vault.ReleaseOnExpiry = *req.ReleaseOnExpiry
	}
	if req.EnableKeepAlive != nil {
		vault.EnableKeepAlive = *req.EnableKeepAlive
	}
	if req.KeepAliveDays != nil && *req.KeepAliveDays > 0 {
		vault.KeepAliveDays = *req.KeepAliveDays
	}

	if err := v.DB.Put(ctx, vault); err != nil {
		return nil, fmt.Errorf("failed to update vault: %w", err)
	}

	return map[string]any{"ok": true}, nil
}

// DeleteVault removes a vault and its associated data.
// DELETE /api/v1/vaults/{id}
func (v *VaultAPI) DeleteVault(ctx context.Context) (any, error) {
	id := rs.Vars(ctx)["id"]

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(ctx, vault); err != nil {
		return nil, fmt.Errorf("vault not found")
	}

	if err := v.DB.Delete(ctx, vault); err != nil {
		return nil, fmt.Errorf("failed to delete vault: %w", err)
	}

	return map[string]any{"ok": true}, nil
}

// DeactivateVault deactivates a vault from the client side.
// If the vault has credits, it sets status to "inactive" (preserving credits).
// If it has no credits, it deletes the vault record entirely.
// POST /api/v1/vaults/{id}/deactivate
func (v *VaultAPI) Vaults_0_Deactivate(ctx context.Context) (any, error) {
	id := rs.Vars(ctx)["0"]

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(ctx, vault); err != nil {
		return nil, fmt.Errorf("vault not found")
	}

	if vault.Credits > 0 {
		vault.Status = "inactive"
		vault.EnableKeepAlive = false
		vault.ReleaseOnExpiry = false
		if err := v.DB.Put(ctx, vault); err != nil {
			return nil, fmt.Errorf("failed to deactivate vault: %w", err)
		}
		return map[string]any{"ok": true, "action": "deactivated"}, nil
	}

	if err := v.DB.Delete(ctx, vault); err != nil {
		return nil, fmt.Errorf("failed to delete vault: %w", err)
	}
	return map[string]any{"ok": true, "action": "deleted"}, nil
}

// ReleaseVault marks a vault as released, making share3 available for recovery.
// POST /api/v1/vaults/{id}/release
func (v *VaultAPI) ReleaseVault(ctx context.Context) (any, error) {
	id := rs.Vars(ctx)["id"]

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(ctx, vault); err != nil {
		return nil, fmt.Errorf("vault not found")
	}

	if vault.Released {
		return nil, fmt.Errorf("vault has already been released")
	}

	config, err := model.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load config")
	}

	if config.PaymentEnabled && vault.Credits <= 0 {
		return nil, fmt.Errorf("no active credits — purchase credits to release vault")
	}

	vault.Released = true
	vault.ReleasedAt = time.Now()
	vault.Status = "released"

	if err := v.DB.Put(ctx, vault); err != nil {
		return nil, fmt.Errorf("failed to release vault: %w", err)
	}

	return map[string]any{"ok": true, "releasedAt": vault.ReleasedAt}, nil
}

// CheckIn records a keep-alive check-in for a vault.
// POST/GET /api/v1/vaults/{id}/checkin
func (v *VaultAPI) CheckIn(r *http.Request) (any, error) {
	if err := RateLimitByIP(r, "checkin", 30, time.Minute); err != nil {
		return nil, err
	}
	ctx := r.Context()
	id := rs.Vars(ctx)["id"]
	token := r.URL.Query().Get("token")
	method := r.URL.Query().Get("method")
	if method == "" {
		method = "manual"
	}

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(ctx, vault); err != nil {
		return nil, fmt.Errorf("vault not found")
	}

	// Validate token if provided (for calendar/push links)
	if token != "" && vault.Token != token {
		return nil, fmt.Errorf("invalid token")
	}

	if vault.Released {
		return nil, fmt.Errorf("vault has already been released")
	}

	config, err := model.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load config")
	}

	if config.PaymentEnabled && vault.Credits <= 0 {
		return nil, fmt.Errorf("no active credits — purchase credits to check in")
	}

	// Update last check-in
	vault.LastCheckIn = time.Now()
	vault.LastCheckInMethod = method
	if err := v.DB.Put(ctx, vault); err != nil {
		return nil, fmt.Errorf("failed to update check-in: %w", err)
	}

	return map[string]any{
		"ok":          true,
		"lastCheckIn": vault.LastCheckIn,
	}, nil
}

// GetShare returns the released share3 (only if the vault has been released).
// GET /api/v1/vaults/{id}/share
func (v *VaultAPI) GetShare(r *http.Request) (any, error) {
	if err := RateLimitByIP(r, "share", 20, time.Minute); err != nil {
		return nil, err
	}
	ctx := r.Context()
	id := rs.Vars(ctx)["id"]

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(ctx, vault); err != nil {
		return nil, fmt.Errorf("vault not found")
	}

	if !vault.Released {
		return nil, fmt.Errorf("vault has not been released")
	}

	return map[string]any{
		"id":         vault.ID,
		"share3Enc":  vault.Share3Enc,
		"releasedAt": vault.ReleasedAt,
	}, nil
}

// AddCredits adds credit years to a vault.
// POST /api/v1/vaults/{id}/credits
func (v *VaultAPI) AddCredits(ctx context.Context, req struct {
	Years int64 `json:"years"`
}) (any, error) {
	id := rs.Vars(ctx)["id"]

	if req.Years < 1 || req.Years > 30 {
		return nil, fmt.Errorf("years must be between 1 and 30")
	}

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(ctx, vault); err != nil {
		return nil, fmt.Errorf("vault not found")
	}

	vault.AddCredits(req.Years)
	if err := v.DB.Put(ctx, vault); err != nil {
		return nil, fmt.Errorf("failed to add credits: %w", err)
	}

	return &rs.Json{
		Status: http.StatusCreated,
		Content: map[string]any{
			"credits":     vault.Credits,
			"releaseDate": vault.ReleaseDate,
		},
	}, nil
}

// CalendarICS generates an ICS file for keep-alive reminders.
// GET /api/v1/vaults/{id}/calendar.ics
func (v *VaultAPI) CalendarICS(r *http.Request, w http.ResponseWriter) error {
	id := rs.Vars(r.Context())["id"]

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(r.Context(), vault); err != nil {
		return fmt.Errorf("vault not found")
	}

	// Build the check-in URL
	config, err := model.Config(r.Context())
	if err != nil {
		return fmt.Errorf("failed to load config")
	}
	checkinURL := fmt.Sprintf("%s/checkin?id=%s&token=%s", config.BaseURL, id, vault.Token)

	interval := vault.KeepAliveDays
	if interval <= 0 {
		interval = 30
	}
	// Use half the keep-alive days for reminder frequency (remind before deadline)
	reminderDays := interval / 2
	if reminderDays < 1 {
		reminderDays = 1
	}

	now := time.Now().UTC()
	dtStamp := now.Format("20060102T150405Z")
	dtStart := now.Format("20060102T090000Z")

	ics := fmt.Sprintf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//PassedBox//Dead Man Switch//EN
CALSCALE:GREGORIAN
METHOD:PUBLISH
BEGIN:VEVENT
UID:%s@passedbox
DTSTAMP:%s
DTSTART:%s
RRULE:FREQ=DAILY;INTERVAL=%d
SUMMARY:PassedBox Keep-Alive Check-In
DESCRIPTION:Click the link to confirm you are still active:\n%s
URL:%s
DURATION:PT15M
BEGIN:VALARM
TRIGGER:-PT10M
ACTION:DISPLAY
DESCRIPTION:PassedBox Keep-Alive Reminder
END:VALARM
END:VEVENT
END:VCALENDAR`, id, dtStamp, dtStart, reminderDays, checkinURL, checkinURL)

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="passedbox-%s.ics"`, id[:8]))
	w.Header().Set("Content-Length", strconv.Itoa(len(ics)))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(ics))
	return nil
}

// TestPush sends a test push notification to all subscribers for a vault.
// POST /api/v1/vaults/{id}/push/test
func (v *VaultAPI) Vaults_0_Push_Test(ctx context.Context) (any, error) {
	id := rs.Vars(ctx)["0"]

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(ctx, vault); err != nil {
		return nil, fmt.Errorf("vault not found")
	}

	if v.Push == nil {
		return nil, fmt.Errorf("push notifications not configured")
	}

	err := v.Push.SendToVault(ctx, id, push.NotifyPayload{
		Title: "PassedBox Keep-Alive",
		Body:  "This is a test notification. Your push setup is working!",
		URL:   fmt.Sprintf("/checkin?id=%s&token=%s", id, vault.Token),
		Tag:   "passedbox-test",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send test push: %w", err)
	}

	return map[string]any{"ok": true}, nil
}

// PushSubscribe registers a web push subscription for a vault.
// POST /api/v1/vaults/{id}/push/subscribe
func (v *VaultAPI) PushSubscribe(r *http.Request, req struct {
	Endpoint string `json:"endpoint"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
}) (*rs.Json, error) {
	if err := RateLimitByIP(r, "push-subscribe", 10, time.Minute); err != nil {
		return nil, err
	}
	ctx := r.Context()
	id := rs.Vars(ctx)["id"]

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(ctx, vault); err != nil {
		return nil, fmt.Errorf("vault not found")
	}

	if req.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}

	sub := &model.PushSubscription{
		VaultID:  id,
		Endpoint: req.Endpoint,
		P256dh:   req.P256dh,
		Auth:     req.Auth,
	}

	if err := v.DB.Put(ctx, sub); err != nil {
		return nil, fmt.Errorf("failed to save subscription: %w", err)
	}

	return &rs.Json{
		Status:  http.StatusCreated,
		Content: map[string]any{"id": sub.ID},
	}, nil
}

// PushUnsubscribe removes a push subscription by endpoint.
// POST /api/v1/vaults/{id}/push/unsubscribe
func (v *VaultAPI) PushUnsubscribe(r *http.Request, req struct {
	Endpoint string `json:"endpoint"`
}) (any, error) {
	if err := RateLimitByIP(r, "push-unsubscribe", 10, time.Minute); err != nil {
		return nil, err
	}
	ctx := r.Context()
	id := rs.Vars(ctx)["id"]

	q := dsorm.NewQuery("PushSubscription").
		FilterField("vaultId", "=", id).
		FilterField("endpoint", "=", req.Endpoint)
	subs, _, err := dsorm.Query[*model.PushSubscription](ctx, v.DB, q, "")
	if err != nil || len(subs) == 0 {
		return map[string]any{"ok": true}, nil
	}

	for _, sub := range subs {
		v.DB.Delete(ctx, sub)
	}

	return map[string]any{"ok": true}, nil
}

// VAPIDPublicKey returns the server's VAPID public key for push subscription.
// GET /api/v1/push/vapid-key
func (v *VaultAPI) VAPIDPublicKey(ctx context.Context) (any, error) {
	if v.Push == nil {
		return nil, fmt.Errorf("push notifications not configured")
	}

	pubKey, _, err := v.Push.GetOrCreateVAPIDKeys(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]any{"publicKey": pubKey}, nil
}

// Stats returns the current vault stats.
// GET /api/v1/stats
func (v *VaultAPI) Stats(ctx context.Context) (any, error) {
	stats, err := model.GetOrCreateStats(ctx)
	if err != nil {
		// Fall back to recalculating
		stats, err = model.RecalculateStats(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get stats: %w", err)
		}
	}
	return stats, nil
}

// ApproveVault changes a vault from pending to active status.
// POST /api/v1/vaults/{id}/approve
func (v *VaultAPI) Vaults_0_Approve(ctx context.Context) (any, error) {
	id := rs.Vars(ctx)["0"]

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(ctx, vault); err != nil {
		return nil, fmt.Errorf("vault not found")
	}

	if vault.Status != "pending" {
		return nil, fmt.Errorf("vault is already %s", vault.Status)
	}

	vault.Status = "active"
	if err := v.DB.Put(ctx, vault); err != nil {
		return nil, fmt.Errorf("failed to approve vault: %w", err)
	}

	return map[string]any{"ok": true}, nil
}

// CreatePendingVault creates a new vault in pending status (admin pre-approval).
// POST /api/v1/vaults/pending
func (v *VaultAPI) Vaults_Pending(ctx context.Context, req struct {
	ID string `json:"id"`
}) (any, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("vault id is required")
	}

	// Check if vault already exists
	existing := &model.Vault{ID: req.ID}
	if err := v.DB.Get(ctx, existing); err == nil {
		return nil, rs.Error{
			Message: fmt.Sprintf("vault %s exists", req.ID),
			Status:  http.StatusConflict,
		}
	}

	vault := &model.Vault{
		ID:     req.ID,
		Status: "pending",
	}

	if err := v.DB.Put(ctx, vault); err != nil {
		return nil, fmt.Errorf("failed to create vault: %w", err)
	}

	return &rs.Json{
		Status:  http.StatusCreated,
		Content: map[string]any{"id": vault.ID, "status": "pending"},
	}, nil
}
