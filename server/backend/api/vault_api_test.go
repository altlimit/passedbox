package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/altlimit/dsorm"
	"github.com/altlimit/dsorm/ds/local"
	rs "github.com/altlimit/restruct"
	"passedbox.com/model"
	"passedbox.com/push"
)

func setupAPI(t *testing.T) (*VaultAPI, *dsorm.Client) {
	t.Helper()

	dir, _ := os.MkdirTemp("", "pbtest-*")
	t.Cleanup(func() { os.RemoveAll(dir) }) // best-effort, may fail on Windows due to SQLite lock

	store := local.NewStore(dir)
	client, err := dsorm.New(context.Background(), dsorm.WithStore(store), dsorm.WithEncryptionKey([]byte("test-encryption-key-for-unittest")))
	if err != nil {
		t.Fatalf("failed to create dsorm client: %v", err)
	}
	api := &VaultAPI{
		DB:   client,
		Push: &push.Sender{DB: client},
	}
	model.Client = client
	Cache = client.Cache()

	// Seed default server config (payment disabled)
	client.Put(context.Background(), &model.ServerConfig{ID: "main"})

	return api, client
}

func ctxWithID(id string) context.Context {
	return rs.SetVars(context.Background(), map[string]string{"id": id})
}

// dummyReq returns an *http.Request suitable for tests (sets RemoteAddr for rate limiting).
func dummyReq() *http.Request {
	return &http.Request{
		URL:        &url.URL{},
		RemoteAddr: "127.0.0.1:9999",
		Header:     http.Header{},
	}
}

// --- Tests ---

func TestRegisterVault(t *testing.T) {
	api, _ := setupAPI(t)

	result, err := api.RegisterVault(dummyReq(), struct {
		ID              string `json:"id"`
		Token           string `json:"token"`
		Share3Enc       []byte `json:"share3Enc"`
		ReleaseOnExpiry bool   `json:"releaseOnExpiry"`
		EnableKeepAlive bool   `json:"enableKeepAlive"`
		KeepAliveDays   int64  `json:"keepAliveDays"`
		Years           int64  `json:"years"`
	}{
		ID:              "test-vault-001",
		Share3Enc:       []byte("encrypted-share3-data"),
		ReleaseOnExpiry: true,
		EnableKeepAlive: false,
		KeepAliveDays:   30,
		Years:           1,
	})

	if err != nil {
		t.Fatalf("RegisterVault failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	jsonResult := result.(*rs.Json)
	content := jsonResult.Content.(map[string]any)
	if content["id"] != "test-vault-001" {
		t.Errorf("expected id=test-vault-001, got %v", content["id"])
	}
	if content["token"] == nil || content["token"] == "" {
		t.Error("expected non-empty token")
	}
}

func TestRegisterVault_MissingID(t *testing.T) {
	api, _ := setupAPI(t)

	_, err := api.RegisterVault(dummyReq(), struct {
		ID              string `json:"id"`
		Token           string `json:"token"`
		Share3Enc       []byte `json:"share3Enc"`
		ReleaseOnExpiry bool   `json:"releaseOnExpiry"`
		EnableKeepAlive bool   `json:"enableKeepAlive"`
		KeepAliveDays   int64  `json:"keepAliveDays"`
		Years           int64  `json:"years"`
	}{
		Share3Enc: []byte("data"),
	})

	if err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestGetVault(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	client.Put(ctx, &model.Vault{
		ID:              "vault-get-test",
		Share3Enc:       []byte("share3"),
		ReleaseOnExpiry: true,
		EnableKeepAlive: true,
		KeepAliveDays:   14,
		LastCheckIn:     time.Now(),
		Token:           "secret-token",
	})

	result, err := api.GetVault(ctxWithID("vault-get-test"))
	if err != nil {
		t.Fatalf("GetVault failed: %v", err)
	}

	m := result.(map[string]any)
	if m["id"] != "vault-get-test" {
		t.Errorf("expected vault-get-test, got %v", m["id"])
	}
	if m["releaseOnExpiry"] != true {
		t.Error("expected releaseOnExpiry=true")
	}
	if m["enableKeepAlive"] != true {
		t.Error("expected enableKeepAlive=true")
	}
}

func TestGetVault_NotFound(t *testing.T) {
	api, _ := setupAPI(t)

	_, err := api.GetVault(ctxWithID("nonexistent"))
	if err == nil {
		t.Fatal("expected error for nonexistent vault")
	}
}

func TestUpdateVault(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	client.Put(ctx, &model.Vault{
		ID:              "vault-update",
		Status:          "active",
		Share3Enc:       []byte("share3"),
		ReleaseOnExpiry: false,
		EnableKeepAlive: false,
		KeepAliveDays:   30,
		LastCheckIn:     time.Now(),
		Token:           "tok",
	})

	keepAlive := true
	days := int64(7)
	_, err := api.UpdateVault(ctxWithID("vault-update"), struct {
		ReleaseOnExpiry *bool  `json:"releaseOnExpiry"`
		EnableKeepAlive *bool  `json:"enableKeepAlive"`
		KeepAliveDays   *int64 `json:"keepAliveDays"`
	}{
		EnableKeepAlive: &keepAlive,
		KeepAliveDays:   &days,
	})

	if err != nil {
		t.Fatalf("UpdateVault failed: %v", err)
	}

	updated := &model.Vault{ID: "vault-update"}
	client.Get(ctx, updated)
	if !updated.EnableKeepAlive {
		t.Error("expected enableKeepAlive=true after update")
	}
	if updated.KeepAliveDays != 7 {
		t.Errorf("expected keepAliveDays=7, got %d", updated.KeepAliveDays)
	}
}

func TestDeleteVault(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	client.Put(ctx, &model.Vault{
		ID:        "vault-delete",
		Share3Enc: []byte("share3"),
		Token:     "tok",
	})

	_, err := api.DeleteVault(ctxWithID("vault-delete"))
	if err != nil {
		t.Fatalf("DeleteVault failed: %v", err)
	}

	check := &model.Vault{ID: "vault-delete"}
	if err := client.Get(ctx, check); err == nil {
		t.Error("expected vault to be deleted")
	}
}

func TestCheckIn(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	oldTime := time.Now().Add(-24 * time.Hour)
	client.Put(ctx, &model.Vault{
		ID:          "vault-checkin",
		Share3Enc:   []byte("share3"),
		LastCheckIn: oldTime,
		Token:       "my-token",
	})

	req := &http.Request{URL: &url.URL{RawQuery: "token=my-token&method=test"}}
	req = req.WithContext(ctxWithID("vault-checkin"))
	_, err := api.CheckIn(req)
	if err != nil {
		t.Fatalf("CheckIn failed: %v", err)
	}

	updated := &model.Vault{ID: "vault-checkin"}
	client.Get(ctx, updated)
	if !updated.LastCheckIn.After(oldTime) {
		t.Error("expected lastCheckIn to be updated")
	}
}

func TestCheckIn_InvalidToken(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	client.Put(ctx, &model.Vault{
		ID:        "vault-badtoken",
		Share3Enc: []byte("share3"),
		Token:     "correct-token",
	})

	req := &http.Request{URL: &url.URL{RawQuery: "token=wrong-token"}}
	req = req.WithContext(ctxWithID("vault-badtoken"))
	_, err := api.CheckIn(req)
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestCheckIn_ReleasedVault(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	client.Put(ctx, &model.Vault{
		ID:        "vault-released",
		Share3Enc: []byte("share3"),
		Released:  true,
		Token:     "tok",
	})

	req := &http.Request{URL: &url.URL{RawQuery: ""}}
	req = req.WithContext(ctxWithID("vault-released"))
	_, err := api.CheckIn(req)
	if err == nil {
		t.Fatal("expected error for released vault check-in")
	}
}

func TestAddCredits(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	client.Put(ctx, &model.Vault{
		ID:        "vault-credits",
		Share3Enc: []byte("share3"),
		Token:     "tok",
	})

	result, err := api.AddCredits(ctxWithID("vault-credits"), struct {
		Years int64 `json:"years"`
	}{Years: 2})

	if err != nil {
		t.Fatalf("AddCredits failed: %v", err)
	}
	content := result.(*rs.Json).Content.(map[string]any)
	if content["credits"].(int64) != 2 {
		t.Errorf("expected credits=2, got %v", content["credits"])
	}
}

func TestAddCredits_InvalidRange(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	client.Put(ctx, &model.Vault{
		ID:        "vault-cred-range",
		Share3Enc: []byte("share3"),
		Token:     "tok",
	})

	_, err := api.AddCredits(ctxWithID("vault-cred-range"), struct {
		Years int64 `json:"years"`
	}{Years: 31})
	if err == nil {
		t.Fatal("expected error for years > 30")
	}

	_, err2 := api.AddCredits(ctxWithID("vault-cred-range"), struct {
		Years int64 `json:"years"`
	}{Years: 0})
	if err2 == nil {
		t.Fatal("expected error for years = 0")
	}
}

func TestAddCreditsExtendRelease(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	client.Put(ctx, &model.Vault{ID: "vault-ac", Share3Enc: []byte("s"), Token: "t", Status: "active"})

	result, err := api.AddCredits(ctxWithID("vault-ac"), struct {
		Years int64 `json:"years"`
	}{Years: 2})
	if err != nil {
		t.Fatalf("AddCredits failed: %v", err)
	}

	resp := result.(*rs.Json)
	content := resp.Content.(map[string]any)
	if content["credits"].(int64) != 2 {
		t.Errorf("expected 2 credits, got %v", content["credits"])
	}
}

func TestGetShare_NotReleased(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	client.Put(ctx, &model.Vault{ID: "vault-locked", Share3Enc: []byte("secret"), Released: false, Token: "t"})

	req := dummyReq()
	req = req.WithContext(ctxWithID("vault-locked"))
	_, err := api.GetShare(req)
	if err == nil {
		t.Fatal("share should not be available for unreleased vault")
	}
}

func TestGetShare_Released(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	client.Put(ctx, &model.Vault{ID: "vault-open", Share3Enc: []byte("secret"), Released: true, ReleasedAt: time.Now(), Token: "t"})

	req := dummyReq()
	req = req.WithContext(ctxWithID("vault-open"))
	result, err := api.GetShare(req)
	if err != nil {
		t.Fatalf("GetShare failed: %v", err)
	}

	m := result.(map[string]any)
	if m["id"] != "vault-open" {
		t.Errorf("expected id=vault-open, got %v", m["id"])
	}
	if m["share3Enc"] == nil {
		t.Error("expected share3Enc in response")
	}
}

func TestListVaults(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		client.Put(ctx, &model.Vault{
			ID: "vault-list-" + string(rune('a'+i)), Share3Enc: []byte("s"), Token: "t",
		})
	}

	req := &http.Request{URL: &url.URL{RawQuery: ""}}
	result, err := api.ListVaults(context.Background(), req)
	if err != nil {
		t.Fatalf("ListVaults failed: %v", err)
	}

	resp := result.(map[string]any)
	vaults := resp["vaults"].([]map[string]any)
	if len(vaults) < 3 {
		t.Errorf("expected at least 3 vaults, got %d", len(vaults))
	}
}

func TestPushSubscribe(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	client.Put(ctx, &model.Vault{ID: "vault-push", Share3Enc: []byte("s"), Token: "t"})

	req := dummyReq()
	req = req.WithContext(ctxWithID("vault-push"))
	result, err := api.PushSubscribe(req, struct {
		Endpoint string `json:"endpoint"`
		P256dh   string `json:"p256dh"`
		Auth     string `json:"auth"`
	}{
		Endpoint: "https://fcm.googleapis.com/fcm/send/abc123",
		P256dh:   "BNcRdreALRFXTkOOUHK1EtK2wtaz5Ry4YfYCA",
		Auth:     "tBHItJI5svbpC7",
	})

	if err != nil {
		t.Fatalf("PushSubscribe failed: %v", err)
	}
	content := result.Content.(map[string]any)
	if content["id"] == nil || content["id"] == "" {
		t.Error("expected non-empty subscription id")
	}
}

func TestPushSubscribe_MissingEndpoint(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	client.Put(ctx, &model.Vault{ID: "vault-push2", Share3Enc: []byte("s"), Token: "t"})

	req := dummyReq()
	req = req.WithContext(ctxWithID("vault-push2"))
	_, err := api.PushSubscribe(req, struct {
		Endpoint string `json:"endpoint"`
		P256dh   string `json:"p256dh"`
		Auth     string `json:"auth"`
	}{P256dh: "key", Auth: "auth"})

	if err == nil {
		t.Fatal("expected error for missing endpoint")
	}
}

func TestVAPIDPublicKey(t *testing.T) {
	api, _ := setupAPI(t)

	result, err := api.VAPIDPublicKey(context.Background())
	if err != nil {
		t.Fatalf("VAPIDPublicKey failed: %v", err)
	}

	m := result.(map[string]any)
	if m["publicKey"] == nil || m["publicKey"] == "" {
		t.Error("expected non-empty publicKey")
	}
}

func TestCreditsStacking(t *testing.T) {
	api, client := setupAPI(t)

	ctx := context.Background()
	client.Put(ctx, &model.Vault{ID: "vault-stack", Share3Enc: []byte("s"), Token: "t"})

	// First credit
	r1, err := api.AddCredits(ctxWithID("vault-stack"), struct {
		Years int64 `json:"years"`
	}{Years: 1})
	if err != nil {
		t.Fatalf("first credit failed: %v", err)
	}
	c1 := r1.(*rs.Json).Content.(map[string]any)
	exp1 := c1["releaseDate"].(time.Time)

	// Second credit — should stack
	r2, err := api.AddCredits(ctxWithID("vault-stack"), struct {
		Years int64 `json:"years"`
	}{Years: 2})
	if err != nil {
		t.Fatalf("second credit failed: %v", err)
	}
	c2 := r2.(*rs.Json).Content.(map[string]any)
	exp2 := c2["releaseDate"].(time.Time)

	if !exp2.After(exp1) {
		t.Error("second credit expiry should be after first (stacking)")
	}

	// Verify total years
	info, err := api.GetVault(ctxWithID("vault-stack"))
	if err != nil {
		t.Fatalf("GetVault failed: %v", err)
	}
	m := info.(map[string]any)
	if m["credits"].(int64) != 3 {
		t.Errorf("expected 3 credits, got %v", m["credits"])
	}
}

func TestRateLimitLogin(t *testing.T) {
	api, _ := setupAPI(t)
	api.Auth = &AuthConfig{} // no valid hash — every attempt is "wrong password"

	// Use a unique IP per test to avoid cross-test pollution.
	req := &http.Request{
		URL:        &url.URL{},
		RemoteAddr: "10.0.0.1:1234",
		Header:     http.Header{},
	}

	for i := 0; i < 10; i++ {
		result := api.Login(req, struct {
			Password string `json:"password"`
		}{Password: "wrong"}, nil)
		if _, ok := result.(rs.Error); !ok {
			t.Fatalf("iteration %d: expected rs.Error, got %T", i, result)
		}
		e := result.(rs.Error)
		if e.Status == http.StatusTooManyRequests {
			t.Fatalf("should not be rate limited at iteration %d", i)
		}
	}

	// 11th call should be rate limited
	result := api.Login(req, struct {
		Password string `json:"password"`
	}{Password: "wrong"}, nil)
	e, ok := result.(rs.Error)
	if !ok {
		t.Fatalf("expected rs.Error, got %T", result)
	}
	if e.Status != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", e.Status)
	}
}

func TestRateLimitRegisterVault(t *testing.T) {
	api, _ := setupAPI(t)

	req := &http.Request{
		URL:        &url.URL{},
		RemoteAddr: "10.0.0.2:1234",
		Header:     http.Header{},
	}

	for i := 0; i < 5; i++ {
		_, err := api.RegisterVault(req, struct {
			ID              string `json:"id"`
			Token           string `json:"token"`
			Share3Enc       []byte `json:"share3Enc"`
			ReleaseOnExpiry bool   `json:"releaseOnExpiry"`
			EnableKeepAlive bool   `json:"enableKeepAlive"`
			KeepAliveDays   int64  `json:"keepAliveDays"`
			Years           int64  `json:"years"`
		}{
			ID:        fmt.Sprintf("rl-vault-%d", i),
			Share3Enc: []byte("data"),
		})
		if err != nil {
			if rsErr, ok := err.(rs.Error); ok && rsErr.Status == http.StatusTooManyRequests {
				t.Fatalf("should not be rate limited at iteration %d", i)
			}
		}
	}

	// 6th call should be rate limited
	_, err := api.RegisterVault(req, struct {
		ID              string `json:"id"`
		Token           string `json:"token"`
		Share3Enc       []byte `json:"share3Enc"`
		ReleaseOnExpiry bool   `json:"releaseOnExpiry"`
		EnableKeepAlive bool   `json:"enableKeepAlive"`
		KeepAliveDays   int64  `json:"keepAliveDays"`
		Years           int64  `json:"years"`
	}{
		ID:        "rl-vault-blocked",
		Share3Enc: []byte("data"),
	})
	if err == nil {
		t.Fatal("expected error from rate limiting")
	}
	rsErr, ok := err.(rs.Error)
	if !ok {
		t.Fatalf("expected rs.Error, got %T: %v", err, err)
	}
	if rsErr.Status != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rsErr.Status)
	}
}
