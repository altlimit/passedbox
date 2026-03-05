package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/altlimit/dsorm"
	rs "github.com/altlimit/restruct"
	"passedbox.com/api"
	"passedbox.com/model"
	"passedbox.com/push"
)

type Task struct {
	taskKey    string
	DB         *dsorm.Client `route:"-"`
	PushSender *push.Sender  `route:"-"`
	VaultAPI   *api.VaultAPI `route:"-"`
}

// StartWorker runs the dead man's switch background checker on the given interval.
func NewTask(db *dsorm.Client, pushSender *push.Sender, vaultAPI *api.VaultAPI) *Task {
	return &Task{
		DB:         db,
		PushSender: pushSender,
		VaultAPI:   vaultAPI,
	}
}

func (t *Task) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Task-Key") != t.taskKey {
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (t *Task) Middlewares() []rs.Middleware {
	if t.taskKey == "" {
		return nil
	}
	return []rs.Middleware{t.authMiddleware}
}

func (t *Task) StartWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("Worker started", "interval", interval)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Worker shutting down")
			return
		case <-ticker.C:
			if err := t.CheckDeadManSwitches(ctx); err != nil {
				slog.Error("Worker: error checking switches", "error", err)
			}
			if err := t.SendKeepAliveReminders(ctx); err != nil {
				slog.Error("Worker: error sending reminders", "error", err)
			}
			if err := t.PendingPayments(ctx); err != nil {
				slog.Error("Worker: error checking pending payments", "error", err)
			}
			if err := t.RecalculateStats(ctx); err != nil {
				slog.Error("Worker: error recalculating stats", "error", err)
			}
		}
	}
}

func (t *Task) PendingPayments(ctx context.Context) error {
	return t.VaultAPI.ProcessPendingPayments(ctx)
}

func (t *Task) RecalculateStats(ctx context.Context) error {
	_, err := model.RecalculateStats(ctx)
	return err
}

func (t *Task) CheckDeadManSwitches(ctx context.Context) error {
	q := dsorm.NewQuery("Vault").FilterField("released", "=", false).FilterField("status", "=", "active")
	vaults, _, err := dsorm.Query[*model.Vault](ctx, t.DB, q, "")
	if err != nil {
		return fmt.Errorf("failed to query vaults: %w", err)
	}

	now := time.Now()

	for _, vault := range vaults {
		shouldRelease := false
		reason := ""

		// Check release date expiry
		if vault.ReleaseOnExpiry && vault.Credits > 0 && !vault.ReleaseDate.IsZero() && now.After(vault.ReleaseDate) {
			shouldRelease = true
			reason = "credits expired"
		}

		// Check keep-alive
		if vault.EnableKeepAlive && vault.KeepAliveDays > 0 {
			deadline := vault.LastCheckIn.AddDate(0, 0, int(vault.KeepAliveDays))
			if now.After(deadline) {
				shouldRelease = true
				reason = "keep-alive missed"
			}
		}

		if shouldRelease {
			vault.Released = true
			vault.ReleasedAt = now
			vault.Status = "released"
			if err := t.DB.Put(ctx, vault); err != nil {
				slog.Error("Worker: failed to release vault", "vaultId", vault.ID, "error", err)
				continue
			}
			slog.Info("Worker: released vault", "vaultId", vault.ID, "reason", reason)

			// Send push notification about release
			if t.PushSender != nil {
				if err := t.PushSender.SendToVault(ctx, vault.ID, push.NotifyPayload{
					Title: "Vault Released",
					Body:  fmt.Sprintf("Your vault has been released (%s).", reason),
					URL:   fmt.Sprintf("/checkin?id=%s&token=%s", vault.ID, vault.Token),
					Tag:   "vault-released-" + vault.ID,
				}); err != nil {
					slog.Error("Worker: failed to send release notification for vault", "vaultId", vault.ID, "error", err)
				}
			}
		}
	}

	return nil
}

// sendKeepAliveReminders sends push notifications to vaults approaching their keep-alive deadline.
func (t *Task) SendKeepAliveReminders(ctx context.Context) error {
	if t.PushSender == nil {
		return nil
	}

	q := dsorm.NewQuery("Vault").FilterField("released", "=", false).FilterField("status", "=", "active")
	vaults, _, err := dsorm.Query[*model.Vault](ctx, t.DB, q, "")
	if err != nil {
		return fmt.Errorf("failed to query vaults: %w", err)
	}

	now := time.Now()

	for _, vault := range vaults {
		if !vault.EnableKeepAlive || vault.KeepAliveDays <= 0 {
			continue
		}

		deadline := vault.LastCheckIn.AddDate(0, 0, int(vault.KeepAliveDays))
		daysUntil := int(deadline.Sub(now).Hours() / 24)

		// Send reminder when half the keep-alive period has passed, or 1 day before deadline
		halfPeriod := vault.KeepAliveDays / 2
		if halfPeriod < 1 {
			halfPeriod = 1
		}

		if daysUntil == int(halfPeriod) || daysUntil == 1 {
			// Skip if we already sent a reminder today
			if sameDay(vault.LastReminderSent, now) {
				continue
			}

			t.PushSender.SendToVault(ctx, vault.ID, push.NotifyPayload{
				Title: "Check-In Reminder",
				Body:  fmt.Sprintf("Please check in within %d day(s) to keep your vault active.", daysUntil),
				URL:   fmt.Sprintf("/checkin?id=%s&token=%s", vault.ID, vault.Token),
				Tag:   "checkin-reminder-" + vault.ID,
			})

			vault.LastReminderSent = now
			if err := t.DB.Put(ctx, vault); err != nil {
				slog.Error("Failed to save reminder timestamp", "vaultId", vault.ID, "error", err)
			}
		}
	}

	return nil
}

// sameDay returns true if both times fall on the same UTC calendar date.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}
