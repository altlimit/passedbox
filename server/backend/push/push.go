package push

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"cloud.google.com/go/datastore"
	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/altlimit/dsorm"
	"passedbox.com/model"
)

// Sender handles web push notification delivery.
type Sender struct {
	DB *dsorm.Client
}

// GetOrCreateVAPIDKeys returns existing VAPID keys or generates and stores new ones.
func (s *Sender) GetOrCreateVAPIDKeys(ctx context.Context) (publicKey, privateKey string, err error) {
	config, err := model.Config(ctx)
	if err == nil && config.PublicKey != "" && config.PrivateKey != "" {
		return config.PublicKey, config.PrivateKey, nil
	} else if err != nil && err != datastore.ErrNoSuchEntity {
		return "", "", err
	}

	// Generate new keys
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate VAPID keys: %w", err)
	}
	config.PublicKey = pub
	config.PrivateKey = priv
	if err := s.DB.Put(ctx, config); err != nil {
		return "", "", fmt.Errorf("failed to store VAPID keys: %w", err)
	}

	return pub, priv, nil
}

// NotifyPayload is the JSON body sent in push notifications.
type NotifyPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

// SendToVault sends a push notification to all subscriptions for a vault.
func (s *Sender) SendToVault(ctx context.Context, vaultID string, payload NotifyPayload) error {
	q := dsorm.NewQuery("PushSubscription").FilterField("vaultId", "=", vaultID)
	subs, _, err := dsorm.Query[*model.PushSubscription](ctx, s.DB, q, "")
	if err != nil {
		return fmt.Errorf("failed to query subscriptions: %w", err)
	}

	if len(subs) == 0 {
		return nil // No subscriptions, nothing to do
	}

	_, privateKey, err := s.GetOrCreateVAPIDKeys(ctx)
	if err != nil {
		return err
	}
	var publicKey string
	config, err := model.Config(ctx)
	if err == nil {
		publicKey = config.PublicKey
	}

	data, _ := json.Marshal(payload)

	for _, sub := range subs {
		subscription := &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				P256dh: sub.P256dh,
				Auth:   sub.Auth,
			},
		}

		resp, err := webpush.SendNotification(data, subscription, &webpush.Options{
			Subscriber:      "mailto:noreply@passedbox.com",
			VAPIDPublicKey:  publicKey,
			VAPIDPrivateKey: privateKey,
			TTL:             86400, // 24 hours
		})
		if err != nil {
			slog.Warn("Push: failed to send notification", "endpoint", sub.Endpoint, "error", err)
			continue
		}
		resp.Body.Close()
		// If subscription is gone (410), remove it
		if resp.StatusCode == 410 || resp.StatusCode == 404 {
			s.DB.Delete(ctx, sub)
			slog.Info("Push: removed stale subscription", "subscriptionId", sub.ID)
		}
	}

	return nil
}
