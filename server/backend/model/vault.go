package model

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/altlimit/dsorm"
)

var (
	Client *dsorm.Client
)

type Vault struct {
	dsorm.Base
	ID                string    `model:"id" json:"id"`
	Status            string    `datastore:"status" json:"status"`                                   // "pending", "active", "released"
	Share3Enc         []byte    `datastore:"share3Enc,noindex,omitempty" json:"share3Enc,omitempty"` // Encrypted share3 from client
	ReleaseOnExpiry   bool      `datastore:"releaseOnExpiry" json:"releaseOnExpiry"`                 // Release when credits run out
	EnableKeepAlive   bool      `datastore:"enableKeepAlive" json:"enableKeepAlive"`                 // Enable keep-alive checking
	KeepAliveDays     int64     `datastore:"keepAliveDays" json:"keepAliveDays"`                     // Days before considering missed
	LastCheckIn       time.Time `datastore:"lastCheckIn" json:"lastCheckIn"`                         // Last keep-alive timestamp
	LastCheckInMethod string    `datastore:"lastCheckInMethod" json:"lastCheckInMethod"`             // "calendar", "webpush", "manual"
	Released          bool      `datastore:"released" json:"released"`                               // Whether share3 has been released
	ReleasedAt        time.Time `datastore:"releasedAt" json:"releasedAt"`                           // When it was released
	Token             string    `datastore:"token,noindex,omitempty" json:"-"`                       // Secret token for check-in links
	Credits           int64     `datastore:"credits" json:"credits"`                                 // Total credits purchased (incrementing)
	ReleaseDate       time.Time `datastore:"releaseDate" json:"releaseDate"`                         // When vault will expire/release
	HasPendingPayment bool      `datastore:"pendingPayment,omitempty" json:"hasPendingPayment"`      // Whether there are unconfirmed payments
	PendingSessions   []string  `datastore:"pendingSessions,noindex,omitempty" json:"-"`             // Unconfirmed Stripe session IDs
	LastReminderSent  time.Time `datastore:"lastReminderSent,omitempty" json:"-"`                    // Last keep-alive reminder sent
	CreatedAt         time.Time `model:"created" json:"createdAt" datastore:"createdAt"`
	UpdatedAt         time.Time `model:"modified" json:"updatedAt" datastore:"updatedAt"`
}

type PushSubscription struct {
	dsorm.Base
	ID        int64     `model:"id" json:"id"`
	VaultID   string    `datastore:"vaultId" json:"vaultId"`
	Endpoint  string    `datastore:"endpoint,noindex" json:"endpoint"`
	P256dh    string    `datastore:"p256dh,noindex" json:"p256dh"`
	Auth      string    `datastore:"auth,noindex" json:"auth"`
	CreatedAt time.Time `model:"created" json:"createdAt"`
}

type ServerConfig struct {
	dsorm.Base
	ID              string    `model:"id" json:"id"`
	PublicKey       string    `datastore:"publicKey" json:"publicKey"`
	PrivateKey      string    `datastore:"privateKey,noindex" json:"-"`
	CreatedAt       time.Time `model:"created" json:"createdAt"`
	AdminPassword   string    `datastore:"password" json:"-"`
	JWTKey          string    `datastore:"jwtKey,noindex" json:"-"`
	PaymentEnabled  bool      `datastore:"paymentEnabled" json:"paymentEnabled"`
	StripeSecretKey string    `datastore:"stripeSecretKey,noindex" json:"-"`
	StripePriceID   string    `datastore:"stripePriceId" json:"-"`
	BaseURL         string    `datastore:"baseUrl" json:"baseUrl"`
}

func (v *Vault) AddCredits(years int64) {
	v.Credits += years
	startFrom := time.Now()
	if v.ReleaseDate.After(startFrom) {
		startFrom = v.ReleaseDate
	}
	v.ReleaseDate = startFrom.AddDate(int(years), 0, 0)
}

// AddPendingSession appends a Stripe session ID to the pending list.
func (v *Vault) AddPendingSession(sessionID string) {
	v.PendingSessions = append(v.PendingSessions, sessionID)
	v.HasPendingPayment = true
}

// RemovePendingSession removes a session ID from the pending list.
func (v *Vault) RemovePendingSession(sessionID string) {
	filtered := v.PendingSessions[:0]
	for _, s := range v.PendingSessions {
		if s != sessionID {
			filtered = append(filtered, s)
		}
	}
	v.PendingSessions = filtered
	v.HasPendingPayment = len(filtered) > 0
}

func Config(ctx context.Context) (*ServerConfig, error) {
	config := &ServerConfig{ID: "main"}
	if err := Client.Get(ctx, config); err != nil {
		if err == datastore.ErrNoSuchEntity {
			return config, Client.Put(ctx, config)
		}
		return nil, fmt.Errorf("failed to load server config: %v", err)
	}
	return config, nil
}
