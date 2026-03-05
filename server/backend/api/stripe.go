package api

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/altlimit/dsorm"
	rs "github.com/altlimit/restruct"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"golang.org/x/crypto/bcrypt"
	"passedbox.com/model"
)

// CreateCheckoutSession creates a Stripe checkout session for vault credits.
// Returns the session ID and checkout URL.
func (v *VaultAPI) CreateCheckoutSession(ctx context.Context, vaultID string, years int64) (string, string, error) {
	config, err := model.Config(ctx)
	if err != nil {
		return "", "", err
	}

	if config.StripeSecretKey == "" || config.StripePriceID == "" {
		return "", "", fmt.Errorf("stripe is not configured")
	}

	stripe.Key = config.StripeSecretKey

	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(config.StripePriceID),
				Quantity: stripe.Int64(years),
				AdjustableQuantity: &stripe.CheckoutSessionLineItemAdjustableQuantityParams{
					Enabled: stripe.Bool(true),
					Minimum: stripe.Int64(1),
					Maximum: stripe.Int64(30),
				},
			},
		},
		SuccessURL: stripe.String(config.BaseURL + "/payment?status=success&id=" + vaultID + "&session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:  stripe.String(config.BaseURL + "/payment?status=cancelled&id=" + vaultID),
		Metadata: map[string]string{
			"vault_id": vaultID,
			"years":    fmt.Sprintf("%d", years),
		},
	}

	s, err := session.New(params)
	if err != nil {
		return "", "", fmt.Errorf("failed to create checkout session: %w", err)
	}

	return s.ID, s.URL, nil
}

// ConfirmPayment verifies a Stripe checkout session and adds credits if paid.
// POST /api/v1/vaults/{id}/confirm-payment
func (v *VaultAPI) Vaults_0_ConfirmPayment(ctx context.Context, req struct {
	SessionID string `json:"sessionId"`
}) (any, error) {
	id := rs.Vars(ctx)["0"]

	if req.SessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(ctx, vault); err != nil {
		return nil, fmt.Errorf("vault not found")
	}

	// Check that session is in the pending list
	found := false
	for _, s := range vault.PendingSessions {
		if s == req.SessionID {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("session not found in pending payments")
	}

	// Load Stripe key and query the session
	config, err := model.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load config")
	}
	stripe.Key = config.StripeSecretKey

	params := &stripe.CheckoutSessionParams{}
	params.AddExpand("line_items")
	cs, err := session.Get(req.SessionID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve checkout session: %w", err)
	}

	if cs.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
		return map[string]any{"status": "unpaid"}, nil
	}

	// Payment confirmed — read actual quantity from line items (user may have adjusted)
	years := 1
	if cs.LineItems != nil && len(cs.LineItems.Data) > 0 {
		years = int(cs.LineItems.Data[0].Quantity)
	} else if yearsStr, ok := cs.Metadata["years"]; ok {
		fmt.Sscanf(yearsStr, "%d", &years)
	}

	vault.AddCredits(int64(years))
	vault.RemovePendingSession(req.SessionID)
	if vault.Status == "pending" {
		vault.Status = "active"
	}

	if err := v.DB.Put(ctx, vault); err != nil {
		return nil, fmt.Errorf("failed to update vault: %w", err)
	}

	slog.Info("Payment confirmed", "vaultId", id, "sessionId", req.SessionID, "years", years)

	return map[string]any{
		"status":      "paid",
		"credits":     vault.Credits,
		"releaseDate": vault.ReleaseDate,
	}, nil
}

// BuyCredits creates a Stripe checkout session for an existing vault.
// POST /api/v1/vaults/{id}/buy
func (v *VaultAPI) Vaults_0_Buy(ctx context.Context, req struct {
	Years int64 `json:"years"`
}) (any, error) {
	id := rs.Vars(ctx)["0"]

	if req.Years < 1 || req.Years > 30 {
		return nil, fmt.Errorf("years must be between 1 and 30")
	}

	vault := &model.Vault{ID: id}
	if err := v.DB.Get(ctx, vault); err != nil {
		return nil, fmt.Errorf("vault not found")
	}

	sessionID, checkoutURL, err := v.CreateCheckoutSession(ctx, id, req.Years)
	if err != nil {
		return nil, fmt.Errorf("failed to create checkout: %w", err)
	}

	vault.AddPendingSession(sessionID)
	if err := v.DB.Put(ctx, vault); err != nil {
		return nil, fmt.Errorf("failed to save pending session: %w", err)
	}

	return map[string]any{
		"checkoutUrl": checkoutURL,
		"sessionId":   sessionID,
	}, nil
}

// ProcessPendingPayments checks all vaults with pending payments and confirms them via Stripe.
// Called by the background worker.
func (v *VaultAPI) ProcessPendingPayments(ctx context.Context) error {
	config, err := model.Config(ctx)
	if err != nil || config.StripeSecretKey == "" {
		return nil // Stripe not configured, skip
	}
	stripe.Key = config.StripeSecretKey

	q := dsorm.NewQuery("Vault").FilterField("pendingPayment", "=", true)
	vaults, _, err := dsorm.Query[*model.Vault](ctx, v.DB, q, "")
	if err != nil {
		return fmt.Errorf("failed to query pending payment vaults: %w", err)
	}

	for _, vault := range vaults {
		changed := false
		for _, sessionID := range vault.PendingSessions {
			p := &stripe.CheckoutSessionParams{}
			p.AddExpand("line_items")
			cs, err := session.Get(sessionID, p)
			if err != nil {
				slog.Error("Worker: failed to query Stripe session", "sessionId", sessionID, "error", err)
				continue
			}
			if cs.PaymentStatus == stripe.CheckoutSessionPaymentStatusPaid {
				years := 1
				if cs.LineItems != nil && len(cs.LineItems.Data) > 0 {
					years = int(cs.LineItems.Data[0].Quantity)
				} else if yearsStr, ok := cs.Metadata["years"]; ok {
					fmt.Sscanf(yearsStr, "%d", &years)
				}
				vault.AddCredits(int64(years))
				vault.RemovePendingSession(sessionID)
				if vault.Status == "pending" {
					vault.Status = "active"
				}
				changed = true
				slog.Info("Worker: confirmed payment", "vaultId", vault.ID, "sessionId", sessionID, "years", years)
			} else if cs.Status == stripe.CheckoutSessionStatusExpired {
				// Session expired (24h default) — stop polling it
				vault.RemovePendingSession(sessionID)
				changed = true
				slog.Info("Worker: removed expired session", "sessionId", sessionID, "vaultId", vault.ID)
			}
		}
		if changed {
			if err := v.DB.Put(ctx, vault); err != nil {
				slog.Error("Worker: failed to update vault", "vaultId", vault.ID, "error", err)
			}
		}
	}
	return nil
}

// ServerInfo returns public server configuration.
// GET /api/v1/info
func (v *VaultAPI) ServerInfo(ctx context.Context) (any, error) {
	config, err := model.Config(ctx)
	if err != nil {
		return map[string]any{"paymentEnabled": false}, nil
	}

	return map[string]any{
		"paymentEnabled": config.PaymentEnabled,
	}, nil
}

// GetSettings returns admin-visible server settings.
// GET /api/v1/settings
func (v *VaultAPI) GetSettings(ctx context.Context) (any, error) {
	config, err := model.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load settings")
	}

	// Mask sensitive keys — only show last 4 chars
	mask := func(s string) string {
		if len(s) <= 4 {
			return "••••"
		}
		return "••••" + s[len(s)-4:]
	}

	return map[string]any{
		"paymentEnabled":  config.PaymentEnabled,
		"stripeSecretKey": mask(config.StripeSecretKey),
		"stripePriceId":   config.StripePriceID,
		"baseUrl":         config.BaseURL,
		"hasStripeKey":    config.StripeSecretKey != "",
	}, nil
}

// UpdateSettings updates payment and Stripe configuration.
// PUT /api/v1/settings
func (v *VaultAPI) UpdateSettings(ctx context.Context, req struct {
	PaymentEnabled  *bool   `json:"paymentEnabled"`
	StripeSecretKey *string `json:"stripeSecretKey"`
	StripePriceID   *string `json:"stripePriceId"`
	BaseURL         *string `json:"baseUrl"`
}) (any, error) {
	config, err := model.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load settings")
	}

	if req.PaymentEnabled != nil {
		config.PaymentEnabled = *req.PaymentEnabled
	}
	if req.StripeSecretKey != nil && *req.StripeSecretKey != "" {
		config.StripeSecretKey = *req.StripeSecretKey
	}
	if req.StripePriceID != nil {
		config.StripePriceID = *req.StripePriceID
	}
	if req.BaseURL != nil {
		config.BaseURL = *req.BaseURL
	}

	if err := v.DB.Put(ctx, config); err != nil {
		return nil, fmt.Errorf("failed to save settings: %w", err)
	}

	return map[string]any{"ok": true}, nil
}

// Settings_Password changes the admin password.
// POST /api/v1/settings/password
func (v *VaultAPI) Settings_Password(ctx context.Context, req struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}) (any, error) {
	if req.CurrentPassword == "" || req.NewPassword == "" {
		return nil, fmt.Errorf("both current and new password are required")
	}

	if len(req.NewPassword) < 8 {
		return nil, fmt.Errorf("new password must be at least 8 characters")
	}

	if !v.Auth.CheckPassword(req.CurrentPassword) {
		return nil, fmt.Errorf("current password is incorrect")
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Update in DB
	config, err := model.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load config")
	}
	config.AdminPassword = string(hash)
	if err := v.DB.Put(ctx, config); err != nil {
		return nil, fmt.Errorf("failed to save password: %w", err)
	}

	// Update in-memory auth
	v.Auth.AdminHash = hash

	return map[string]any{"ok": true}, nil
}
