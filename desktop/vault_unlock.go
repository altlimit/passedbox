package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// UnlockVaultWithShare3 recovers a vault using share1 (local) + share3 (from server)
// when the Dead Man's Switch has been released. No password needed.
func (vm *VaultManager) UnlockVaultWithShare3(vaultName string) error {
	vmeta, vstate, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return err
	}

	if !vmeta.DMS.Enabled {
		return errors.New("Dead Man's Switch is not enabled")
	}

	if len(vmeta.Secret.Share3Key) == 0 {
		return errors.New("share3 recovery key not available — vault must be re-enabled for DMS to support recovery")
	}

	// Fetch encrypted share3 from server
	serverURL := strings.TrimRight(vmeta.DMS.ServerURL, "/")
	req, err := dmsAuthRequest("GET", serverURL+"/api/v1/vaults/"+vmeta.VaultID+"/share", nil, vmeta.DMS.Token)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach server: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server error: %s", string(body))
	}

	var result struct {
		Share3Enc []byte `json:"share3Enc"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse server response: %w", err)
	}

	// Decrypt share3 using the locally stored share3Key
	share3, err := Decrypt(result.Share3Enc, vmeta.Secret.Share3Key)
	if err != nil {
		return fmt.Errorf("failed to decrypt share3: %w", err)
	}

	// Get share1 (may need pepper decryption)
	share1 := vmeta.Secret.Share1
	if vstate.UsePepper {
		info := getDevicePepperInfoOS()
		if !info.Available {
			return errors.New("device pepper not available")
		}
		pepperKey := DeriveKey([]byte(info.SerialID), vmeta.Secret.Salt)
		decShare1, err := Decrypt(share1, pepperKey)
		if err != nil {
			return errors.New("failed to decrypt share1 with device pepper")
		}
		share1 = decShare1
	}

	// Combine share1 + share3 to reconstruct master key
	masterKey, err := CombineShares([][]byte{share1, share3})
	if err != nil {
		return fmt.Errorf("failed to reconstruct master key: %w", err)
	}

	vm.MasterKeys[vaultName] = masterKey

	// Mark vault as recovered without password
	db, dbErr := vm.getDB(vaultName)
	if dbErr == nil {
		vmeta.Recovered = true
		if err := vmeta.Update(vm.ctx, db); err != nil {
			return fmt.Errorf("failed to update vault metadata: %w", err)
		}
	}

	fmt.Println("Vault recovered via Dead Man's Switch!")
	return nil
}
