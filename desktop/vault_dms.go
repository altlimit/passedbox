package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// dmsAuthRequest creates an HTTP request with Bearer vault-token auth for server API calls.
func dmsAuthRequest(method, url string, body io.Reader, vaultToken string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if vaultToken != "" {
		req.Header.Set("Authorization", "Bearer "+vaultToken)
	}
	if method == "POST" || method == "PUT" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// DisableDeadManSwitch disables DMS. Calls server deactivate endpoint which
// deletes the vault if no credits, or sets it to inactive if credits exist.
func (vm *VaultManager) DisableDeadManSwitch(vaultName string) error {
	_, err := vm.masterKey(vaultName)
	if err != nil {
		return errors.New("vault must be unlocked to disable Dead Man's Switch")
	}

	db, err := vm.getDB(vaultName)
	if err != nil {
		return err
	}

	vmeta, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return err
	}

	if !vmeta.DMSEnabled {
		return errors.New("Dead Man's Switch is not enabled")
	}

	// Call deactivate endpoint (server decides delete vs inactive)
	serverURL := strings.TrimRight(vmeta.DMSServerURL, "/")
	req, err := dmsAuthRequest("POST", serverURL+"/api/v1/vaults/"+vmeta.VaultID+"/deactivate", nil, vmeta.DMSToken)
	if err != nil {
		return err
	}

	var action string
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Server might be unreachable, still allow disabling locally
		fmt.Printf("WARN: failed to deactivate on server: %v\n", err)
		action = "deleted" // assume deleted when unreachable
	} else {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var result map[string]any
		if json.Unmarshal(body, &result) == nil {
			action, _ = result["action"].(string)
		}
	}

	// Update local metadata
	clearToken := action == "deleted"
	vmeta.DMSEnabled = false
	vmeta.DMSServerURL = ""
	if clearToken {
		vmeta.DMSToken = ""
	}
	return db.Put(vm.ctx, vmeta)
}

// ResetDeadManSwitch force-clears all local DMS metadata regardless of server state.
// Use this when the DMS is in a broken state (e.g. token mismatch).
func (vm *VaultManager) ResetDeadManSwitch(vaultName string, resetVaultID bool) error {
	db, err := vm.getDB(vaultName)
	if err != nil {
		return err
	}

	vmeta, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return err
	}

	// Clear DMS fields
	vmeta.DMSEnabled = false
	vmeta.DMSServerURL = ""
	vmeta.DMSToken = ""
	vmeta.Share3Key = nil

	if resetVaultID {
		vmeta.VaultID = uuid.New().String()
	}

	return db.Put(vm.ctx, vmeta)
}

// UpdateDeadManSwitchSettings updates DMS settings on the server.
func (vm *VaultManager) UpdateDeadManSwitchSettings(vaultName string, settings DMSSettings) error {
	vmeta, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return err
	}

	if !vmeta.DMSEnabled {
		return errors.New("Dead Man's Switch is not enabled")
	}

	serverURL := strings.TrimRight(vmeta.DMSServerURL, "/")
	payloadBytes, err := json.Marshal(settings)
	if err != nil {
		return err
	}

	req, err := dmsAuthRequest("PUT", serverURL+"/api/v1/vaults/"+vmeta.VaultID, bytes.NewReader(payloadBytes), vmeta.DMSToken)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s", string(body))
	}

	return nil
}

// BuyCredits creates a Stripe checkout session on the server and returns the checkout URL.
func (vm *VaultManager) BuyCredits(vaultName string, years int) (string, error) {
	vmeta, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return "", err
	}

	if !vmeta.DMSEnabled {
		return "", errors.New("Dead Man's Switch is not enabled")
	}

	serverURL := strings.TrimRight(vmeta.DMSServerURL, "/")
	body, _ := json.Marshal(map[string]int{"years": years})
	req, err := dmsAuthRequest("POST", serverURL+"/api/v1/vaults/"+vmeta.VaultID+"/buy", bytes.NewReader(body), vmeta.DMSToken)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to reach server: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server error: %s", string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	checkoutURL, _ := result["checkoutUrl"].(string)
	if checkoutURL == "" {
		return "", errors.New("no checkout URL returned")
	}

	return checkoutURL, nil
}

// GetShare3ForRecovery fetches the released share3 from the server for vault recovery.
func (vm *VaultManager) GetShare3ForRecovery(vaultName string) (string, error) {
	vmeta, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return "", err
	}

	if !vmeta.DMSEnabled {
		return "", errors.New("Dead Man's Switch is not enabled")
	}

	serverURL := strings.TrimRight(vmeta.DMSServerURL, "/")
	req, err := dmsAuthRequest("GET", serverURL+"/api/v1/vaults/"+vmeta.VaultID+"/share", nil, vmeta.DMSToken)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to reach server: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server error: %s", string(body))
	}

	var result struct {
		Share3Enc []byte `json:"share3Enc"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(result.Share3Enc), nil
}

// IsRecoveredVault returns true if the vault was recovered via DMS without a password.
func (vm *VaultManager) IsRecoveredVault(vaultName string) (bool, error) {
	vmeta, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return false, err
	}
	return vmeta.RecoveredNoPassword, nil
}

// GetDeadManSwitchStatus fetches the current DMS status from the server.
func (vm *VaultManager) GetDeadManSwitchStatus(vaultName string) (DMSStatus, error) {
	vmeta, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return DMSStatus{}, err
	}

	if !vmeta.DMSEnabled {
		return DMSStatus{Enabled: false}, nil
	}

	serverURL := strings.TrimRight(vmeta.DMSServerURL, "/")
	req, err := dmsAuthRequest("GET", serverURL+"/api/v1/vaults/"+vmeta.VaultID, nil, vmeta.DMSToken)
	if err != nil {
		return DMSStatus{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return DMSStatus{}, fmt.Errorf("failed to reach server: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return DMSStatus{}, fmt.Errorf("UNAUTHORIZED: %s", string(body))
	}
	if resp.StatusCode != http.StatusOK {
		return DMSStatus{}, fmt.Errorf("server error: %s", string(body))
	}

	var serverStatus map[string]any
	if err := json.Unmarshal(body, &serverStatus); err != nil {
		return DMSStatus{}, err
	}

	// Fetch server info to check payment mode
	infoReq, _ := http.NewRequest("GET", serverURL+"/api/v1/info", nil)
	var paymentEnabled bool
	if infoResp, err := http.DefaultClient.Do(infoReq); err == nil {
		defer infoResp.Body.Close()
		var infoData map[string]any
		infoBody, _ := io.ReadAll(infoResp.Body)
		if json.Unmarshal(infoBody, &infoData) == nil {
			paymentEnabled, _ = infoData["paymentEnabled"].(bool)
		}
	}

	status := DMSStatus{
		Enabled:        true,
		ServerURL:      vmeta.DMSServerURL,
		Token:          vmeta.DMSToken,
		CalendarURL:    serverURL + "/api/v1/vaults/" + vmeta.VaultID + "/calendar.ics",
		PaymentEnabled: paymentEnabled,
	}

	if v, ok := serverStatus["status"].(string); ok {
		status.Status = v
	}
	if v, ok := serverStatus["releaseOnExpiry"].(bool); ok {
		status.ReleaseOnExpiry = v
	}
	if v, ok := serverStatus["enableKeepAlive"].(bool); ok {
		status.EnableKeepAlive = v
	}
	if v, ok := serverStatus["keepAliveDays"].(float64); ok {
		status.KeepAliveDays = int(v)
	}
	if v, ok := serverStatus["lastCheckIn"].(string); ok {
		status.LastCheckIn = v
	}
	if v, ok := serverStatus["released"].(bool); ok {
		status.Released = v
	}
	if v, ok := serverStatus["releasedAt"].(string); ok {
		status.ReleasedAt = v
	}
	if v, ok := serverStatus["credits"].(float64); ok {
		status.Credits = int(v)
	}
	if v, ok := serverStatus["releaseDate"].(string); ok {
		status.ReleaseDate = v
	}
	if v, ok := serverStatus["creditsActive"].(bool); ok {
		status.CreditsActive = v
	}

	return status, nil
}

// EnableDeadManSwitch enables DMS for a vault: re-splits the master key into 3-of-2 shares,
// encrypts share3 and sends it to the specified server.
func (vm *VaultManager) EnableDeadManSwitch(vaultName, serverURL, password string) (DMSStatus, error) {
	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return DMSStatus{}, errors.New("vault must be unlocked to enable Dead Man's Switch")
	}

	db, err := vm.getDB(vaultName)
	if err != nil {
		return DMSStatus{}, err
	}

	vmeta, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return DMSStatus{}, err
	}

	if vmeta.DMSEnabled {
		return DMSStatus{}, errors.New("Dead Man's Switch is already enabled")
	}

	// Verify the password is correct before re-splitting keys.
	// Use the same logic as UnlockVault: derive a key from password+salt
	// and attempt to decrypt the existing Share2Enc.
	verifyPassword := password
	if vmeta.UsePepper {
		info := getDevicePepperInfoOS()
		if !info.Available {
			return DMSStatus{}, errors.New("invalid device")
		}
		verifyPassword = password + info.SerialID
	}
	verifyKey := DeriveKey([]byte(verifyPassword), vmeta.Salt)
	if _, err := Decrypt(vmeta.Share2Enc, verifyKey); err != nil {
		return DMSStatus{}, errors.New("incorrect vault password")
	}

	// Re-split master key into 3 shares (threshold 2)
	shares, err := SplitKey(masterKey, 3, 2)
	if err != nil {
		return DMSStatus{}, fmt.Errorf("failed to split key: %w", err)
	}

	// Generate new salt for share encryption
	newSalt, err := GenerateRandomBytes(16)
	if err != nil {
		return DMSStatus{}, err
	}

	// Encrypt share3 with a key derived from the master key for transport to server
	share3Key := DeriveKey(masterKey, newSalt)
	encryptedShare3, err := Encrypt(shares[2], share3Key)
	if err != nil {
		return DMSStatus{}, fmt.Errorf("failed to encrypt share3: %w", err)
	}

	// Send share3 to server
	serverURL = strings.TrimRight(serverURL, "/")
	regPayload := map[string]any{
		"id":              vmeta.VaultID,
		"token":           vmeta.DMSToken,
		"share3Enc":       encryptedShare3,
		"releaseOnExpiry": false,
		"enableKeepAlive": false,
		"keepAliveDays":   30,
	}
	payloadBytes, err := json.Marshal(regPayload)
	if err != nil {
		return DMSStatus{}, err
	}

	// RegisterVault is public (no auth needed)
	resp, err := http.Post(serverURL+"/api/v1/vaults", "application/json", bytes.NewReader(payloadBytes))
	if err != nil {
		return DMSStatus{}, fmt.Errorf("failed to reach server: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusConflict {
		return DMSStatus{}, fmt.Errorf("CONFLICT: %s", string(body))
	}
	if resp.StatusCode != http.StatusCreated {
		return DMSStatus{}, fmt.Errorf("server error: %s", string(body))
	}

	var regResult struct {
		ID          string `json:"id"`
		Token       string `json:"token"`
		Status      string `json:"status"`
		CheckoutURL string `json:"checkoutUrl"`
	}
	if err := json.Unmarshal(body, &regResult); err != nil {
		return DMSStatus{}, fmt.Errorf("failed to parse server response: %w", err)
	}

	// Re-encrypt share1 and share2 using the user's password (matching UnlockVault logic).
	// This ensures the vault can still be unlocked with the same password after DMS is enabled.
	finalPassword := password
	share1Final := shares[0]
	if vmeta.UsePepper {
		info := getDevicePepperInfoOS()
		if info.Available {
			finalPassword = password + info.SerialID

			pepperKey := DeriveKey([]byte(info.SerialID), newSalt)
			encShare1, err := Encrypt(shares[0], pepperKey)
			if err != nil {
				return DMSStatus{}, err
			}
			share1Final = encShare1
		} else {
			return DMSStatus{}, errors.New("invalid device")
		}
	}

	// Encrypt share2 with password-derived key (same method as CreateVault/UnlockVault)
	share2Key := DeriveKey([]byte(finalPassword), newSalt)
	encryptedShare2, err := Encrypt(shares[1], share2Key)
	if err != nil {
		return DMSStatus{}, err
	}

	// Verify reconstruction works
	testedKey, err := CombineShares([][]byte{shares[0], shares[1]})
	if err != nil {
		return DMSStatus{}, fmt.Errorf("reconstruction verification failed: %w", err)
	}
	for i := range masterKey {
		if masterKey[i] != testedKey[i] {
			return DMSStatus{}, errors.New("reconstruction integrity check failed")
		}
	}

	// Update metadata
	vmeta.Share1 = share1Final
	vmeta.Share2Enc = encryptedShare2
	vmeta.Salt = newSalt
	vmeta.DMSEnabled = true
	vmeta.DMSServerURL = serverURL
	vmeta.DMSToken = regResult.Token
	vmeta.Share3Key = share3Key
	if err := db.Put(vm.ctx, vmeta); err != nil {
		return DMSStatus{}, fmt.Errorf("failed to update vault metadata: %w", err)
	}

	return DMSStatus{
		Enabled:     true,
		ServerURL:   serverURL,
		Status:      regResult.Status,
		Token:       regResult.Token,
		CalendarURL: serverURL + "/api/v1/vaults/" + vmeta.VaultID + "/calendar.ics",
		CheckoutURL: regResult.CheckoutURL,
	}, nil
}

// CheckDMSRelease checks with the server whether this vault has been released.
// Returns the DMSStatus with release information.
func (vm *VaultManager) CheckDMSRelease(vaultName string) (DMSStatus, error) {
	vmeta, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return DMSStatus{}, err
	}

	if !vmeta.DMSEnabled {
		return DMSStatus{Enabled: false}, nil
	}

	serverURL := strings.TrimRight(vmeta.DMSServerURL, "/")
	req, err := dmsAuthRequest("GET", serverURL+"/api/v1/vaults/"+vmeta.VaultID, nil, vmeta.DMSToken)
	if err != nil {
		return DMSStatus{Enabled: true}, nil // Server unreachable, still report DMS enabled
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return DMSStatus{Enabled: true}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return DMSStatus{Enabled: true}, nil
	}

	var serverStatus map[string]any
	if err := json.Unmarshal(body, &serverStatus); err != nil {
		return DMSStatus{Enabled: true}, nil
	}

	status := DMSStatus{
		Enabled:   true,
		ServerURL: vmeta.DMSServerURL,
		Token:     vmeta.DMSToken,
	}
	if v, ok := serverStatus["released"].(bool); ok {
		status.Released = v
	}
	if v, ok := serverStatus["releasedAt"].(string); ok {
		status.ReleasedAt = v
	}
	return status, nil
}
