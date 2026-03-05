package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestVaultFullLifecycle(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "passedbox_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Change working directory to temp dir
	originalWd, _ := os.Getwd()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalWd)

	vm := NewVaultManager()
	vaultName := "testvault"
	password := "initialpassword"

	// 1. Test Creation
	t.Run("CreateVault", func(t *testing.T) {
		if err := vm.CreateVault(vaultName, password, false); err != nil {
			t.Fatalf("Failed to create vault: %v", err)
		}
		if _, err := os.Stat(vaultName + VaultExtension); os.IsNotExist(err) {
			t.Fatalf("Vault folder not created")
		}
	})

	// 2. Test Unlock
	t.Run("UnlockVault", func(t *testing.T) {
		if err := vm.UnlockVault(vaultName, password); err != nil {
			t.Fatalf("Failed to unlock vault: %v", err)
		}
		if vm.MasterKeys[vaultName] == nil {
			t.Fatal("Master key is nil after unlock")
		}
	})

	// 3. Test File Operations
	var fileID string
	t.Run("FileOperations", func(t *testing.T) {
		content := []byte("hello world")
		tmpFile := filepath.Join(tempDir, "hello.txt")
		if err := os.WriteFile(tmpFile, content, 0644); err != nil {
			t.Fatal(err)
		}

		if _, err := vm.ImportFile(vaultName, "", tmpFile); err != nil {
			t.Fatalf("Failed to import file: %v", err)
		}

		// List files to get the ID
		files, err := vm.ListFiles(vaultName, "")
		if err != nil {
			t.Fatalf("Failed to list files: %v", err)
		}
		found := false
		for _, f := range files {
			if f.Name == "hello.txt" {
				fileID = f.ID
				found = true
				break
			}
		}
		if !found {
			t.Fatal("Imported file not found in ListFiles")
		}

		// Read file
		readContent, err := vm.GetFile(vaultName, fileID)
		if err != nil {
			t.Fatalf("Failed to get file: %v", err)
		}
		if !bytes.Equal(content, readContent) {
			t.Fatalf("Content mismatch: got %s, want %s", string(readContent), string(content))
		}
	})

	// 4. Test Folder Operations
	var folderID string
	t.Run("FolderOperations", func(t *testing.T) {
		if _, err := vm.CreateFolder(vaultName, "", "myfolder"); err != nil {
			t.Fatalf("Failed to create folder: %v", err)
		}

		// List files to get the ID
		files, err := vm.ListFiles(vaultName, "")
		if err != nil {
			t.Fatalf("Failed to list files: %v", err)
		}
		found := false
		for _, f := range files {
			if f.Name == "myfolder" {
				folderID = f.ID
				found = true
				break
			}
		}
		if !found {
			t.Fatal("Created folder not found in ListFiles")
		}

		// Move file to folder
		if err := vm.MoveFile(vaultName, fileID, folderID); err != nil {
			t.Fatalf("Failed to move file: %v", err)
		}

		// Verify file in folder
		files, err = vm.ListFiles(vaultName, folderID)
		if err != nil {
			t.Fatalf("Failed to list files in folder: %v", err)
		}
		if len(files) != 1 || files[0].ID != fileID {
			t.Fatalf("File not found in folder after move")
		}
	})

	// 5. Test Copy (Pointer-based)
	t.Run("CopyFiles", func(t *testing.T) {
		if err := vm.CopyFiles(vaultName, "", []string{fileID}); err != nil {
			t.Fatalf("Failed to copy file: %v", err)
		}
		files, err := vm.ListFiles(vaultName, "")
		if err != nil {
			t.Fatalf("Failed to list files in root: %v", err)
		}
		// Should have 1 folder and 1 copied file
		// Note: CopyFiles appends " (Copy)" to the name
		if len(files) != 2 {
			t.Fatalf("Expected 2 items in root, got %d", len(files))
		}
	})

	// 6. Test Vault Info
	t.Run("GetVaultInfo", func(t *testing.T) {
		info, err := vm.GetVaultInfo(vaultName)
		if err != nil {
			t.Fatalf("Failed to get vault info: %v", err)
		}
		// 2 files (original + copy), 1 folder
		if info.TotalFiles != 2 || info.TotalFolders != 1 {
			t.Fatalf("Incorrect vault info: %+v", info)
		}
	})

	// 7. Test Change Password
	t.Run("ChangeVaultPassword", func(t *testing.T) {
		newPassword := "newsupersecret"
		if err := vm.ChangeVaultPassword(vaultName, password, newPassword, false); err != nil {
			t.Fatalf("Failed to change password: %v", err)
		}

		// Lock and try to unlock with new password
		vm.LockVault(vaultName)
		if err := vm.UnlockVault(vaultName, password); err == nil {
			t.Fatal("Expected error with old password, got nil")
		}
		if err := vm.UnlockVault(vaultName, newPassword); err != nil {
			t.Fatalf("Failed to unlock with new password: %v", err)
		}
	})

	// 8. Test Cross-Vault Operations
	t.Run("CrossVaultOperations", func(t *testing.T) {
		vault2 := "vault2"
		pass2 := "pass2"
		if err := vm.CreateVault(vault2, pass2, false); err != nil {
			t.Fatalf("Failed to create vault2: %v", err)
		}
		if err := vm.UnlockVault(vault2, pass2); err != nil {
			t.Fatalf("Failed to unlock vault2: %v", err)
		}

		// Copy from vault1 folder to vault2 root
		if err := vm.CopyAcrossVaults(vaultName, vault2, "", []string{folderID}); err != nil {
			t.Fatalf("Failed cross-vault copy: %v", err)
		}

		// Verify vault2 content
		files, err := vm.ListFiles(vault2, "")
		if err != nil {
			t.Fatalf("Failed to list vault2 root: %v", err)
		}
		if len(files) != 1 || files[0].Name != "myfolder" {
			t.Fatalf("Cross-vault copy failed verification: %+v", files)
		}

		// Verify nested file in vault2
		nested, err := vm.ListFiles(vault2, files[0].ID)
		if err != nil {
			t.Fatalf("Failed to list nested in vault2: %v", err)
		}
		if len(nested) != 1 || nested[0].Name != "hello.txt" {
			t.Fatalf("Nested cross-vault copy failed: %+v", nested)
		}
	})
}
