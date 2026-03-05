package main

import (
	"errors"
	"fmt"
)

// ChangeVaultPassword updates the master password of a vault.
func (vm *VaultManager) ChangeVaultPassword(vaultName, oldPassword, newPassword string, useDevicePepper bool) error {
	db, err := vm.getDB(vaultName)
	if err != nil {
		return err
	}

	vmeta, vstate, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return err
	}

	share1 := vmeta.Secret.Share1
	encryptedShare2 := vmeta.Secret.Share2Enc
	salt := vmeta.Secret.Salt
	usePepperDoc := vstate.UsePepper

	// Decrypt Share 2 (and Share 1 if peppered) with old password
	finalOldPassword := oldPassword
	if usePepperDoc {
		info := getDevicePepperInfoOS()
		if info.Available {
			finalOldPassword = oldPassword + info.SerialID

			pepperKey := DeriveKey([]byte(info.SerialID), salt)
			decShare1, err := Decrypt(share1, pepperKey)
			if err != nil {
				return errors.New("failed to decrypt key with device")
			}
			share1 = decShare1
		} else {
			return errors.New("invalid device")
		}
	}

	keyEK := DeriveKey([]byte(finalOldPassword), salt)
	share2, err := Decrypt(encryptedShare2, keyEK)
	if err != nil {
		return errors.New("incorrect current password")
	}

	// 2. Encrypt Share 2 with new password and new salt
	newSalt, err := GenerateRandomBytes(16)
	if err != nil {
		return err
	}

	finalNewPassword := newPassword
	share1Final := share1
	if useDevicePepper {
		info := getDevicePepperInfoOS()
		if info.Available {
			finalNewPassword = newPassword + info.SerialID

			pepperKey := DeriveKey([]byte(info.SerialID), newSalt)
			encShare1, err := Encrypt(share1, pepperKey)
			if err != nil {
				return err
			}
			share1Final = encShare1
		} else {
			return errors.New("invalid device")
		}
	}

	newKeyEK := DeriveKey([]byte(finalNewPassword), newSalt)
	newEncryptedShare2, err := Encrypt(share2, newKeyEK)
	if err != nil {
		return err
	}

	// Double check we can still reconstruct (optional but safe)
	reconstructed, err := CombineShares([][]byte{share1, share2})
	if err != nil {
		return fmt.Errorf("failed to verify key reconstruction: %v", err)
	}

	currentMasterKey, ok := vm.MasterKeys[vaultName]
	if ok {
		// Verify it matches what we have in memory
		for i := range reconstructed {
			if reconstructed[i] != currentMasterKey[i] {
				return errors.New("integrity check failed: reconstructed key mismatch")
			}
		}
	}

	// 3. Update database
	vmeta.Secret.Share1 = share1Final
	vmeta.Secret.Share2Enc = newEncryptedShare2
	vmeta.Secret.Salt = newSalt
	vstate.UsePepper = useDevicePepper
	if err := db.Put(vm.ctx, vstate); err != nil {
		return fmt.Errorf("failed to update vault state: %w", err)
	}
	return vmeta.Update(vm.ctx, db)
}

// SetNewPassword sets a new password on a recovered vault that has no password.
// Uses the in-memory master key directly instead of decrypting from old password.
func (vm *VaultManager) SetNewPassword(vaultName, newPassword string, useDevicePepper bool) error {
	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return errors.New("vault must be unlocked")
	}

	db, err := vm.getDB(vaultName)
	if err != nil {
		return err
	}

	vmeta, vstate, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return err
	}

	if !vmeta.Recovered {
		return errors.New("vault is not in recovered state — use ChangeVaultPassword instead")
	}

	// Re-split master key into 2 shares (DMS will be disabled)
	shares, err := SplitKey(masterKey, 2, 2)
	if err != nil {
		return fmt.Errorf("failed to split key: %w", err)
	}

	// Generate new salt
	newSalt, err := GenerateRandomBytes(16)
	if err != nil {
		return err
	}

	// Handle device pepper
	finalPassword := newPassword
	share1Final := shares[0]
	if useDevicePepper {
		info := getDevicePepperInfoOS()
		if info.Available {
			finalPassword = newPassword + info.SerialID
			pepperKey := DeriveKey([]byte(info.SerialID), newSalt)
			encShare1, err := Encrypt(shares[0], pepperKey)
			if err != nil {
				return err
			}
			share1Final = encShare1
		} else {
			return errors.New("invalid device")
		}
	}

	// Encrypt share2 with password
	share2Key := DeriveKey([]byte(finalPassword), newSalt)
	newEncShare2, err := Encrypt(shares[1], share2Key)
	if err != nil {
		return err
	}

	// Verify reconstruction
	reconstructed, err := CombineShares([][]byte{shares[0], shares[1]})
	if err != nil {
		return fmt.Errorf("reconstruction verification failed: %w", err)
	}
	for i := range masterKey {
		if masterKey[i] != reconstructed[i] {
			return errors.New("integrity check failed")
		}
	}

	// Update metadata — disable DMS since shares are re-split
	vmeta.Secret.Share1 = share1Final
	vmeta.Secret.Share2Enc = newEncShare2
	vmeta.Secret.Salt = newSalt
	vmeta.Recovered = false
	vmeta.DMS.Enabled = false
	vmeta.DMS.ServerURL = ""
	vmeta.DMS.Token = ""
	vmeta.Secret.Share3Key = nil
	vstate.UsePepper = useDevicePepper
	if err := db.Put(vm.ctx, vstate); err != nil {
		return fmt.Errorf("failed to update vault state: %w", err)
	}
	return vmeta.Update(vm.ctx, db)
}
