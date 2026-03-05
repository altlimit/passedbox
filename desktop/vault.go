package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altlimit/dsorm"
	"github.com/altlimit/dsorm/ds/local"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	VaultExtension = ".pbx"
	MetaBucket     = "metadata"
	FilesBucket    = "files"
	KeyType        = "type"
	KeyGlobal      = "global"
)

type Vault struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// VaultManager manages vault operations.
type VaultManager struct {
	MasterKeys  map[string][]byte
	openDBs     map[string]*dsorm.Client
	dbMutex     sync.RWMutex
	isCancelled atomic.Bool // Used to abort long-running tasks
	ctx         context.Context
}

type PathNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SearchResult struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	IsFolder  bool       `json:"isFolder"`
	Size      int64      `json:"size"`
	VaultName string     `json:"vaultName"`
	ParentID  string     `json:"parentId"`
	Path      string     `json:"path"`      // breadcrumb path like "Folder > Subfolder"
	PathNodes []PathNode `json:"pathNodes"` // structured trail for navigation
}

type VaultInfo struct {
	VaultID      string `json:"vaultId"`
	TotalFiles   int    `json:"totalFiles"`
	TotalFolders int    `json:"totalFolders"`
	TotalSize    int64  `json:"totalSize"`
	UsePepper    bool   `json:"usePepper"`
	DMSEnabled   bool   `json:"dmsEnabled"`
	DMSServerURL string `json:"dmsServerUrl"`
}

// DMSStatus represents the current Dead Man's Switch status from the server.
type DMSStatus struct {
	Enabled         bool   `json:"enabled"`
	ServerURL       string `json:"serverUrl"`
	Status          string `json:"status"`
	ReleaseOnExpiry bool   `json:"releaseOnExpiry"`
	EnableKeepAlive bool   `json:"enableKeepAlive"`
	KeepAliveDays   int    `json:"keepAliveDays"`
	LastCheckIn     string `json:"lastCheckIn"`
	Released        bool   `json:"released"`
	ReleasedAt      string `json:"releasedAt"`
	Credits         int    `json:"credits"`
	ReleaseDate     string `json:"releaseDate"`
	CreditsActive   bool   `json:"creditsActive"`
	CalendarURL     string `json:"calendarUrl"`
	Token           string `json:"token"`
	CheckoutURL     string `json:"checkoutUrl,omitempty"`
	PaymentEnabled  bool   `json:"paymentEnabled"`
}

// DMSSettings represents the settings to update on the server.
type DMSSettings struct {
	ReleaseOnExpiry *bool `json:"releaseOnExpiry,omitempty"`
	EnableKeepAlive *bool `json:"enableKeepAlive,omitempty"`
	KeepAliveDays   *int  `json:"keepAliveDays,omitempty"`
}

type ProgressEvent struct {
	Op      string  `json:"op"`
	Current int     `json:"current"`
	Total   int     `json:"total"`
	Message string  `json:"message"`
	Percent float64 `json:"percent"`
}

func (vm *VaultManager) emitProgress(op string, current, total int, message string) {
	app := application.Get()
	if app == nil {
		return
	}
	percent := 0.0
	if total > 0 {
		percent = float64(current) / float64(total) * 100
	}
	app.Event.Emit("progress", ProgressEvent{
		Op:      op,
		Current: current,
		Total:   total,
		Message: message,
		Percent: percent,
	})
}

// NewVaultManager creates a new instance.
func NewVaultManager() *VaultManager {
	return &VaultManager{
		MasterKeys: make(map[string][]byte),
		openDBs:    make(map[string]*dsorm.Client),
		ctx:        context.Background(),
	}
}

func (vm *VaultManager) getDB(vaultName string) (*dsorm.Client, error) {
	vm.dbMutex.RLock()
	db, ok := vm.openDBs[vaultName]
	vm.dbMutex.RUnlock()
	if ok {
		return db, nil
	}

	vm.dbMutex.Lock()
	defer vm.dbMutex.Unlock()

	// Double-check after lock
	if db, ok := vm.openDBs[vaultName]; ok {
		return db, nil
	}

	dbPath := vaultName + VaultExtension
	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return nil, err
	}

	db, err := dsorm.New(vm.ctx, dsorm.WithStore(local.NewStore(dbPath)))
	if err != nil {
		return nil, err
	}
	vm.openDBs[vaultName] = db
	return db, nil
}

// GetDevicePepperInfo returns the device serial info for the current executable location.
func (vm *VaultManager) GetDevicePepperInfo() DevicePepperInfo {
	return getDevicePepperInfoOS()
}

// CancelAction safely flags long-running tasks to abort and returns an error
func (vm *VaultManager) CancelAction() {
	vm.isCancelled.Store(true)
}

// checkCancel throws an error if the user triggered a cancellation
func (vm *VaultManager) checkCancel() error {
	if vm.isCancelled.Load() {
		return errors.New("operation cancelled by user")
	}
	return nil
}

// LockVault removes the master key from memory, zeroing it first.
func (vm *VaultManager) LockVault(vaultName string) error {
	key, ok := vm.MasterKeys[vaultName]
	if ok {
		for i := range key {
			key[i] = 0
		}
		delete(vm.MasterKeys, vaultName)
	}

	vm.dbMutex.Lock()
	defer vm.dbMutex.Unlock()
	if db, ok := vm.openDBs[vaultName]; ok {
		db.Close()
		delete(vm.openDBs, vaultName)
	}
	return nil
}

func (vm *VaultManager) loadFile(vaultName, fileID string) (*FileMetadata, error) {
	db, err := vm.getDB(vaultName)
	if err != nil {
		return nil, err
	}
	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return nil, err
	}
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
	fm := &FileMetadata{ID: fileID}
	if err := db.Get(ctx, fm); err != nil {
		return nil, errors.New("file not found in metadata")
	}

	return fm, nil
}

// SearchFiles searches across all unlocked vaults for files/folders matching the searchTerm.
func (vm *VaultManager) SearchFiles(searchTerm string) ([]SearchResult, error) {
	if len(vm.MasterKeys) == 0 {
		return nil, nil
	}

	queryStr := strings.ToLower(strings.TrimSpace(searchTerm))
	if queryStr == "" {
		return nil, nil
	}

	vaults, err := vm.ListVaults()
	if err != nil {
		return nil, err
	}

	var results []SearchResult

	for _, vault := range vaults {
		_, ok := vm.MasterKeys[vault.Name]
		if !ok {
			continue
		}

		db, err := vm.getDB(vault.Name)
		if err != nil {
			continue
		}

		// Build full file index for this vault
		allFiles := make(map[string]*FileMetadata)

		q := dsorm.NewQuery("Filemetadata").FilterField("isDeleted", "=", false)
		files, _, err := dsorm.Query[*FileMetadata](vm.ctx, db, q, "")
		if err != nil {
			continue
		}

		for _, meta := range files {
			allFiles[meta.ID] = meta
		}

		// Search and build results
		for _, meta := range allFiles {
			if meta.IsDeleted {
				continue
			}
			if strings.Contains(strings.ToLower(meta.Name), queryStr) {
				// Build path breadcrumb
				pathStr, pathNodes := buildPath(allFiles, meta.ParentID)
				results = append(results, SearchResult{
					ID:        meta.ID,
					Name:      meta.Name,
					IsFolder:  meta.IsFolder,
					Size:      meta.Size,
					VaultName: vault.Name,
					ParentID:  meta.ParentID,
					Path:      pathStr,
					PathNodes: pathNodes,
				})
			}
		}
	}

	return results, nil
}

// buildPath builds a breadcrumb string and structured node array from parentID up to root
func buildPath(allFiles map[string]*FileMetadata, parentID string) (string, []PathNode) {
	var parts []string
	var nodes []PathNode
	current := parentID
	for current != "" {
		if f, ok := allFiles[current]; ok {
			parts = append([]string{f.Name}, parts...)
			nodes = append([]PathNode{{ID: f.ID, Name: f.Name}}, nodes...)
			current = f.ParentID
		} else {
			break
		}
	}
	return strings.Join(parts, " › "), nodes
}

// ListVaults scans the current directory for folders ending with .vault
func (vm *VaultManager) ListVaults() ([]Vault, error) {
	var vaults []Vault
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), VaultExtension) {
			vaultName := strings.TrimSuffix(entry.Name(), VaultExtension)
			vaultPath := entry.Name()

			meta, _, err := vm.vaultMetadata(vaultName)
			if err != nil {
				continue
			}
			vaults = append(vaults, Vault{
				ID:   meta.VaultID,
				Name: vaultName,
				Path: vaultPath,
			})
		}
	}
	return vaults, nil
}

// CreateVault creates a new vault with the given name and password.
func (vm *VaultManager) CreateVault(vaultName, password string, useDevicePepper bool) error {
	vaultID := uuid.New().String()

	// Generate Master Key
	masterKey, err := GenerateRandomBytes(32)
	if err != nil {
		return err
	}

	// Split Master Key into 2 shares (threshold 2)
	shares, err := SplitKey(masterKey, 2, 2)
	if err != nil {
		return err
	}

	// Prepare encryption for Share 2
	salt, err := GenerateRandomBytes(16)
	if err != nil {
		return err
	}

	finalPassword := password
	share1Final := shares[0]
	if useDevicePepper {
		info := getDevicePepperInfoOS()
		if info.Available {
			finalPassword = password + info.SerialID

			pepperKey := DeriveKey([]byte(info.SerialID), salt)
			encShare1, err := Encrypt(shares[0], pepperKey)
			if err != nil {
				return err
			}
			share1Final = encShare1
		} else {
			return errors.New("invalid device")
		}
	}
	keyEK := DeriveKey([]byte(finalPassword), salt)
	encryptedShare2, err := Encrypt(shares[1], keyEK)
	if err != nil {
		return err
	}

	db, err := vm.getDB(vaultName)
	if err != nil {
		return err
	}
	// Store metadata
	vmeta := &VaultMetadata{
		ID:      "main",
		VaultID: vaultID,
		Secret: Secret{
			Share1:    share1Final,
			Share2Enc: encryptedShare2,
			Salt:      salt,
		},
	}
	vstate, err := vmeta.GetState(vm.ctx, db)
	if err != nil {
		return err
	}
	vstate.UsePepper = useDevicePepper
	if err := db.Put(vm.ctx, vstate); err != nil {
		return err
	}
	return vmeta.Update(vm.ctx, db)
}

// UnlockVault attempts to unlock a vault with the provided password.
func (vm *VaultManager) UnlockVault(vaultName, password string) error {
	vmeta, vstate, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return err
	}
	share1 := vmeta.Secret.Share1
	encryptedShare2 := vmeta.Secret.Share2Enc
	salt := vmeta.Secret.Salt
	usePepper := vstate.UsePepper

	if share1 == nil || encryptedShare2 == nil || salt == nil {
		return errors.New("corrupt vault: missing keys")
	}

	// Decrypt Share 2
	finalPassword := password
	if usePepper {
		info := getDevicePepperInfoOS()
		if info.Available {
			finalPassword = password + info.SerialID

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

	keyEK := DeriveKey([]byte(finalPassword), salt)
	share2, err := Decrypt(encryptedShare2, keyEK)
	if err != nil {
		return errors.New("invalid password or corrupt data")
	}

	// Reconstruct Master Key
	masterKey, err := CombineShares([][]byte{share1, share2})
	if err != nil {
		return errors.New("failed to reconstruct master key")
	}

	vm.MasterKeys[vaultName] = masterKey
	fmt.Println("Vault unlocked successfully!")
	return nil
}

// ImportFile imports a file into the vault, encrypting it with the master key.
func (vm *VaultManager) ImportFile(vaultName, parentID, sourcePath string) (string, error) {
	masterKey, ok := vm.MasterKeys[vaultName]
	if !ok || len(masterKey) == 0 {
		return "", errors.New("vault is locked")
	}

	var size int64
	var err error
	var content []byte
	var encryptedContent []byte
	name := filepath.Base(sourcePath)
	if sourcePath != "" && name != sourcePath {
		var fileInfo os.FileInfo
		fileInfo, err = os.Stat(sourcePath)
		if err != nil {
			return "", err
		}
		size = fileInfo.Size()
		// Read source file
		content, err = os.ReadFile(sourcePath)
		if err != nil {
			return "", err
		}

	} else {
		content = []byte("")
	}

	// Encrypt content
	encryptedContent, err = Encrypt(content, masterKey)
	if err != nil {
		return "", err
	}

	vaultPath := vaultName + VaultExtension
	blobDir := filepath.Join(vaultPath, "data")
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		return "", err
	}

	db, err := vm.getDB(vaultName)
	if err != nil {
		return "", err
	}
	fm := &FileMetadata{
		ID:        uuid.New().String(),
		Name:      name,
		Size:      size,
		ParentID:  parentID,
		IsDeleted: false,
		IsFolder:  false,
	}
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
	if err := db.Put(ctx, fm); err != nil {
		return "", err
	}
	blobPath := filepath.Join(blobDir, fm.ID)

	// Store encrypted blob
	if err := os.WriteFile(blobPath, encryptedContent, 0600); err != nil {
		db.Delete(ctx, fm)
		return "", err
	}

	return fm.ID, nil
}

// ImportFolder recursively imports a physical OS directory into the vault.
func (vm *VaultManager) ImportFolder(vaultName, parentID, sourceDirPath string) error {
	// Pre-calculate total files for progress reporting
	totalFiles := 0
	filepath.Walk(sourceDirPath, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			totalFiles++
		}
		return nil
	})

	current := 0
	folderName := filepath.Base(sourceDirPath)
	vm.isCancelled.Store(false) // Reset cancel state before starting
	return vm.importFolderInternal(vaultName, parentID, sourceDirPath, folderName, &current, totalFiles)
}

func (vm *VaultManager) importFolderInternal(vaultName, parentID, sourceDirPath, rootFolderName string, current *int, total int) error {
	if err := vm.checkCancel(); err != nil {
		return err
	}
	folderName := filepath.Base(sourceDirPath)
	vm.emitProgress("import", *current, total, fmt.Sprintf("Importing %s to %s: Creating folder %s", rootFolderName, vaultName, folderName))

	folderID, err := vm.CreateFolder(vaultName, parentID, folderName)
	if err != nil {
		return err
	}
	*current++

	files, err := os.ReadDir(sourceDirPath)
	if err != nil {
		return err
	}

	for _, f := range files {
		fullPath := filepath.Join(sourceDirPath, f.Name())
		if f.IsDir() {
			if err := vm.importFolderInternal(vaultName, folderID, fullPath, rootFolderName, current, total); err != nil {
				return err
			}
		} else {
			if err := vm.checkCancel(); err != nil {
				return err
			}
			vm.emitProgress("import", *current, total, fmt.Sprintf("Importing %s to %s: %s", rootFolderName, vaultName, fullPath))
			if _, err := vm.ImportFile(vaultName, folderID, fullPath); err != nil {
				return err
			}
			*current++
		}
	}
	vm.emitProgress("import", total, total, "Import complete")
	return nil
}

// ImportFiles imports multiple files directly, emitting true total progress.
func (vm *VaultManager) ImportFiles(vaultName, parentID string, sourcePaths []string) error {
	masterKey, ok := vm.MasterKeys[vaultName]
	if !ok || len(masterKey) == 0 {
		return errors.New("vault is locked")
	}

	total := len(sourcePaths)
	vm.isCancelled.Store(false) // Reset cancel state

	for i, sourcePath := range sourcePaths {
		if err := vm.checkCancel(); err != nil {
			return err
		}

		fileName := filepath.Base(sourcePath)
		vm.emitProgress("import", i, total, fmt.Sprintf("Importing to %s: %s", vaultName, fileName))

		if _, err := vm.ImportFile(vaultName, parentID, sourcePath); err != nil {
			return err
		}
	}

	vm.emitProgress("import", total, total, "Import complete")
	return nil
}

// ImportPaths imports multiple mixed paths (files and directories).
func (vm *VaultManager) ImportPaths(vaultName, parentID string, sourcePaths []string) error {
	masterKey, ok := vm.MasterKeys[vaultName]
	if !ok || len(masterKey) == 0 {
		return errors.New("vault is locked")
	}

	vm.isCancelled.Store(false) // Reset cancel state

	for _, sourcePath := range sourcePaths {
		if err := vm.checkCancel(); err != nil {
			return err
		}

		info, err := os.Stat(sourcePath)
		if err != nil {
			continue
		}
		if info.IsDir() {
			if err := vm.ImportFolder(vaultName, parentID, sourcePath); err != nil {
				return err
			}
		} else {
			if err := vm.ImportFiles(vaultName, parentID, []string{sourcePath}); err != nil {
				return err
			}
		}
	}
	return nil
}

// CreateFile creates a new empty file in the vault and returns its ID.
func (vm *VaultManager) CreateFile(vaultName, parentID, name string) (string, error) {
	return vm.ImportFile(vaultName, parentID, name)
}

func (vm *VaultManager) masterKey(vaultName string) ([]byte, error) {
	masterKey, ok := vm.MasterKeys[vaultName]
	if !ok || len(masterKey) == 0 {
		return nil, errors.New("vault is locked")
	}
	return masterKey, nil
}

// UpdateFileContent updates the content of an existing file in the vault.
// If the file is a pointer to another file (a copy), editing it breaks the pointer
// and creates an independent blob, updating reference counts on the original.
func (vm *VaultManager) UpdateFileContent(vaultName string, fileID string, content string) error {
	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return err
	}
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
	contentBytes := []byte(content)
	encryptedContent, err := Encrypt(contentBytes, masterKey)
	if err != nil {
		return err
	}

	db, err := vm.getDB(vaultName)
	if err != nil {
		return err
	}

	meta, err := vm.loadFile(vaultName, fileID)
	if err != nil {
		return err
	}
	vaultPath := vaultName + VaultExtension

	// Handle copy-on-write logic
	if meta.PointerTo != "" {
		originalID := meta.PointerTo

		// 1. Fetch the original
		originalMeta, err := vm.loadFile(vaultName, originalID)
		if err != nil {
			return err
		}
		// 2. Remove this fileID from the original's PointersFrom list
		var newPointers []string
		for _, p := range originalMeta.PointersFrom {
			if p != fileID {
				newPointers = append(newPointers, p)
			}
		}
		originalMeta.PointersFrom = newPointers

		orig := &FileMetadata{ID: originalID}
		if originalMeta.IsDeleted && len(originalMeta.PointersFrom) == 0 {
			// Delete original entirely
			db.Delete(ctx, orig)
			blobPath := filepath.Join(vaultPath, "data", originalID)
			os.Remove(blobPath) // clean up original blob
		} else {
			if err := db.Get(ctx, orig); err == nil {
				orig.PointersFrom = originalMeta.PointersFrom
				db.Put(ctx, orig)
			}
		}
		// Detach pointer
		meta.PointerTo = ""
	} else if len(meta.PointersFrom) > 0 {
		// This is an ORIGINAL file that has copies pointing to it.
		// The user wants to edit this file, but we can't overwrite the physical blob
		// because copies depend on it.

		// 1. Create a "hidden" replica of the old state
		oldID := uuid.New().String()
		oldMeta := *meta
		oldMeta.ID = oldID
		oldMeta.IsDeleted = true // Hidden from UI

		// 2. Update all copies to point to oldID instead of fileID
		for _, copyID := range meta.PointersFrom {
			copy := &FileMetadata{ID: copyID}
			if err := db.Get(ctx, copy); err == nil {
				copy.PointerTo = oldID
				db.Put(ctx, copy)
			}
		}

		db.Put(ctx, oldMeta)

		// 4. Physically rename the old blob file to the oldID
		oldBlobPath := filepath.Join(vaultPath, "data", fileID)
		newBlobPath := filepath.Join(vaultPath, "data", oldID)
		if err := os.Rename(oldBlobPath, newBlobPath); err != nil {
			return err
		}

		// 5. Clear this file's pointer lists since it will now be a brand new blob
		meta.PointersFrom = nil
	}

	// Now write the new blob for this specific fileID (it is now independent)
	blobPath := filepath.Join(vaultPath, "data", fileID)
	if err := os.WriteFile(blobPath, encryptedContent, 0600); err != nil {
		return err
	}

	// Update size and save
	meta.Size = int64(len(contentBytes))
	return db.Put(ctx, meta)
}

// ListFiles retrieves files from the vault for a given parentID.
func (vm *VaultManager) ListFiles(vaultName, parentID string) ([]*FileMetadata, error) {
	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return nil, err
	}

	db, err := vm.getDB(vaultName)
	if err != nil {
		return nil, err
	}
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
	q := dsorm.NewQuery("FileMetadata").FilterField("parentId", "=", parentID).FilterField("isDeleted", "=", false)
	files, _, err := dsorm.Query[*FileMetadata](ctx, db, q, "")
	if err != nil {
		return nil, err
	}

	return files, nil
}

// GetVaultInfo calculates metrics for a vault.
func (vm *VaultManager) GetVaultInfo(vaultName string) (VaultInfo, error) {
	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return VaultInfo{}, err
	}

	db, err := vm.getDB(vaultName)
	if err != nil {
		return VaultInfo{}, err
	}

	var info VaultInfo

	vmeta, vstate, err := vm.vaultMetadata(vaultName)
	if err != nil {
		return VaultInfo{}, err
	}

	info.VaultID = vmeta.VaultID
	info.UsePepper = vstate.UsePepper
	info.DMSEnabled = vmeta.DMS.Enabled
	info.DMSServerURL = vmeta.DMS.ServerURL

	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
	q := dsorm.NewQuery("FileMetadata").FilterField("isDeleted", "=", false)
	files, _, err := dsorm.Query[*FileMetadata](ctx, db, q, "")
	if err != nil {
		return VaultInfo{}, err
	}

	for _, meta := range files {
		if meta.IsFolder {
			info.TotalFolders++
		} else {
			info.TotalFiles++
		}
	}

	// Calculate actual storage size by iterating over the physical blobs in data/ directory
	dataDir := filepath.Join(vaultName+VaultExtension, "data")
	entries, err := os.ReadDir(dataDir)
	if err == nil {
		var actualSize int64
		for _, entry := range entries {
			if !entry.IsDir() {
				if fInfo, err := entry.Info(); err == nil {
					actualSize += fInfo.Size()
				}
			}
		}
		info.TotalSize = actualSize
	}

	return info, nil
}

// GetFile retrieves the decrypted content of a file.
// WARNING: This loads the entire file into memory. Use only for small files (viewing).
func (vm *VaultManager) GetFile(vaultName, fileID string) ([]byte, error) {
	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return nil, err
	}

	// Retrieve metadata to check for pointer
	meta, err := vm.GetFileMetadata(vaultName, fileID)
	if err != nil {
		return nil, err
	}

	blobID := fileID
	if meta.PointerTo != "" {
		blobID = meta.PointerTo
	}

	vaultPath := vaultName + VaultExtension
	blobPath := filepath.Join(vaultPath, "data", blobID)

	// Read encrypted content
	encryptedContent, err := os.ReadFile(blobPath)
	if err != nil {
		return nil, err
	}

	// Decrypt content
	return Decrypt(encryptedContent, masterKey)
}

// ExportFile decrypts a file and saves it to the destination path.
// This is memory efficient for large files if we streamed, but for now using simple implementation
// as `Encrypt`/`Decrypt` helpers are byte-slice based.
// TODO: Refactor crypto to support streaming for large files.
func (vm *VaultManager) ExportFile(vaultName, fileID, destPath string) error {
	content, err := vm.GetFile(vaultName, fileID)
	if err != nil {
		return err
	}

	return os.WriteFile(destPath, content, 0644)
}

// ExportFiles decrypts and saves multiple files and folders to a target directory.
func (vm *VaultManager) ExportFiles(vaultName, destDir string, fileIDs []string) error {
	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return err
	}

	db, err := vm.getDB(vaultName)
	if err != nil {
		return err
	}

	totalToExport, _ := vm.countItems(db, fileIDs, masterKey)
	current := 0
	vm.isCancelled.Store(false) // Reset cancel state
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
	var exportItem func(db *dsorm.Client, fID, currentDestPath string) error
	exportItem = func(db *dsorm.Client, fID, currentDestPath string) error {
		if err := vm.checkCancel(); err != nil {
			return err
		}

		meta, err := vm.loadFile(vaultName, fID)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(currentDestPath, meta.Name)

		if meta.IsFolder {
			vm.emitProgress("export", current, totalToExport, fmt.Sprintf("Exporting to %s: Creating folder %s", destDir, meta.Name))
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
			current++

			// Recurse children
			q := dsorm.NewQuery("FileMetadata").FilterField("parentId", "=", fID).FilterField("isDeleted", "=", false)
			files, _, err := dsorm.Query[*FileMetadata](ctx, db, q, "")
			if err == nil {
				for _, child := range files {
					if err := exportItem(db, child.ID, targetPath); err != nil {
						return err
					}
				}
			}
		} else {
			vm.emitProgress("export", current, totalToExport, fmt.Sprintf("Exporting to %s: %s", destDir, meta.Name))
			blobID := fID
			if meta.PointerTo != "" {
				blobID = meta.PointerTo
			}
			vaultPath := vaultName + VaultExtension
			blobPath := filepath.Join(vaultPath, "data", blobID)

			encryptedContent, err := os.ReadFile(blobPath)
			if err == nil {
				decryptedContent, err := Decrypt(encryptedContent, masterKey)
				if err == nil {
					if err := os.WriteFile(targetPath, decryptedContent, 0644); err != nil {
						return err
					}
					current++
				}
			}
		}
		return nil
	}

	for _, id := range fileIDs {
		if err := exportItem(db, id, destDir); err != nil {
			return err
		}
	}
	vm.emitProgress("export", totalToExport, totalToExport, "Export complete")
	return nil
}

// GetFileMetadata retrieves the metadata for a specific file.
func (vm *VaultManager) GetFileMetadata(vaultName, fileID string) (*FileMetadata, error) {
	return vm.loadFile(vaultName, fileID)
}

// CopyFiles creates a semantic copy of files/folders in a target destination.
func (vm *VaultManager) CopyFiles(vaultName, targetParentID string, sourceIDs []string) error {
	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return err
	}

	db, err := vm.getDB(vaultName)
	if err != nil {
		return err
	}

	for _, srcID := range sourceIDs {
		if err := vm.copyItem(db, srcID, targetParentID, masterKey); err != nil {
			return err
		}
	}
	return nil
}

// copyItem recursively copies a file or folder using copy-on-write pointers.
func (vm *VaultManager) copyItem(db *dsorm.Client, srcID, targetParentID string, masterKey []byte) error {
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
	meta := &FileMetadata{ID: srcID}
	if err := db.Get(ctx, meta); err != nil {
		return errors.New("source item not found")
	}

	if meta.IsDeleted {
		return nil
	}

	var newID string
	var err error
	if meta.IsFolder {
		newID, err = vm.newFolder(db, targetParentID, meta.Name+" (Copy)", masterKey)
		if err != nil {
			return err
		}

		// Find children to copy
		q := dsorm.NewQuery("FileMetadata").FilterField("parentId", "=", srcID)
		files, _, err := dsorm.Query[*FileMetadata](ctx, db, q, "")
		if err == nil {
			for _, child := range files {
				vm.copyItemInternal(db, child.ID, newID, masterKey)
			}
		}
	} else {
		newID = uuid.New().String()

		originalID := srcID
		if meta.PointerTo != "" {
			originalID = meta.PointerTo
		}
		newFile := &FileMetadata{
			ID:        newID,
			Name:      meta.Name + " (Copy)",
			Size:      meta.Size,
			ParentID:  targetParentID,
			CreatedAt: time.Now(),
			IsFolder:  false,
			PointerTo: originalID,
		}

		orig := &FileMetadata{ID: originalID}
		if err := db.Get(ctx, orig); err == nil {
			orig.PointersFrom = append(orig.PointersFrom, newID)
			if err := db.Put(ctx, orig); err != nil {
				return err
			}
		}

		if err := db.Put(ctx, newFile); err != nil {
			return err
		}
	}

	return nil
}

// copyItemInternal is like copyItem but does not append " (Copy)" to the name.
func (vm *VaultManager) copyItemInternal(db *dsorm.Client, srcID, targetParentID string, masterKey []byte) error {
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
	meta := &FileMetadata{ID: srcID}
	if err := db.Get(ctx, meta); err != nil {
		return err
	}

	if meta.IsDeleted {
		return nil
	}

	newID := uuid.New().String()

	if meta.IsFolder {
		folderID, err := vm.newFolder(db, targetParentID, meta.Name, masterKey)
		if err != nil {
			return err
		}
		q := dsorm.NewQuery("FileMetadata").FilterField("parentId", "=", srcID)
		files, _, _ := dsorm.Query[*FileMetadata](ctx, db, q, "")
		for _, child := range files {
			if err := vm.copyItemInternal(db, child.ID, folderID, masterKey); err != nil {
				return err
			}
		}
	} else {
		originalID := srcID
		if meta.PointerTo != "" {
			originalID = meta.PointerTo
		}

		newFile := meta
		newFile.ID = newID
		newFile.ParentID = targetParentID
		newFile.CreatedAt = time.Now()
		newFile.PointerTo = originalID
		newFile.PointersFrom = nil

		// Update original
		meta = &FileMetadata{ID: originalID}

		if err := db.Get(ctx, meta); err == nil {
			meta.PointersFrom = append(meta.PointersFrom, newID)
			if err := db.Put(ctx, meta); err != nil {
				return err
			}
		}

		if err := db.Put(ctx, newFile); err != nil {
			return err
		}
	}

	return nil
}

// CreateFolder creates a folder entry in the vault metadata (no blob data) and returns its ID.
func (vm *VaultManager) CreateFolder(vaultName, parentID, folderName string) (string, error) {
	db, err := vm.getDB(vaultName)
	if err != nil {
		return "", err
	}

	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return "", err
	}

	return vm.newFolder(db, parentID, folderName, masterKey)
}

func (vm *VaultManager) newFolder(db *dsorm.Client, parentID string, folderName string, masterKey []byte) (string, error) {
	meta := &FileMetadata{
		ID:        uuid.New().String(),
		Name:      folderName,
		ParentID:  parentID,
		CreatedAt: time.Now(),
		IsFolder:  true,
	}
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
	return meta.ID, db.Put(ctx, meta)
}

// MoveFile updates a file or folder's parent ID (moves it to a different folder).
func (vm *VaultManager) MoveFile(vaultName, fileID, newParentID string) error {
	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return err
	}
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
	db, err := vm.getDB(vaultName)
	if err != nil {
		return err
	}

	meta := &FileMetadata{ID: fileID}
	if db.Get(ctx, meta) != nil {
		return errors.New("file not found")
	}

	// Prevent moving a folder into itself
	if meta.ID == newParentID {
		return errors.New("cannot move a folder into itself")
	}

	meta.ParentID = newParentID
	return db.Put(ctx, meta)
}

// RenameFile renames a file or folder in the vault.
func (vm *VaultManager) RenameFile(vaultName, fileID, newName string) error {
	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return err
	}
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
	db, err := vm.getDB(vaultName)
	if err != nil {
		return err
	}

	meta := &FileMetadata{ID: fileID}
	if db.Get(ctx, meta) != nil {
		return errors.New("file not found")
	}
	meta.Name = newName
	return db.Put(ctx, meta)
}

// DeleteFile removes a file or folder from the vault.
func (vm *VaultManager) DeleteFile(vaultName, fileID string) error {
	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return err
	}
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
	db, err := vm.getDB(vaultName)
	if err != nil {
		return err
	}

	var idsToDelete []string
	var idsToMarkDeleted []string

	// 1. Collect all initial IDs (including children if folder)
	var collectedIDs []string
	meta, err := vm.loadFile(vaultName, fileID)
	if err != nil {
		return err
	}

	collectedIDs = append(collectedIDs, fileID)
	if meta.IsFolder {
		if err := vm.collectDescendants(db, fileID, &collectedIDs, masterKey); err != nil {
			return err
		}
	}

	// 2. Process each collected ID for Pointer logic
	for _, id := range collectedIDs {
		m, err := vm.loadFile(vaultName, id)
		if err != nil {
			continue
		}
		if m.PointerTo != "" {
			origID := m.PointerTo
			oMeta, err := vm.loadFile(vaultName, origID)
			if err != nil {
				continue
			}
			var newPointers []string
			for _, p := range oMeta.PointersFrom {
				if p != id {
					newPointers = append(newPointers, p)
				}
			}
			oMeta.PointersFrom = newPointers
			if oMeta.IsDeleted && len(oMeta.PointersFrom) == 0 {
				idsToDelete = append(idsToDelete, origID)
			} else {
				orig := &FileMetadata{ID: origID}
				if db.Get(ctx, orig) == nil {
					orig.PointersFrom = oMeta.PointersFrom
					db.Put(ctx, orig)
				}
			}
			idsToDelete = append(idsToDelete, id)
		} else {
			if len(m.PointersFrom) > 0 {
				idsToMarkDeleted = append(idsToMarkDeleted, id)
			} else {
				idsToDelete = append(idsToDelete, id)
			}
		}
	}

	// 3. Mark hidden
	for _, id := range idsToMarkDeleted {
		meta := &FileMetadata{ID: id}
		if db.Get(ctx, meta) == nil {
			meta.IsDeleted = true
			db.Put(ctx, meta)
		}
	}

	// 4. Delete metadata for fully destroyed items
	for _, id := range idsToDelete {
		db.Delete(ctx, &FileMetadata{ID: id})
	}

	// 5. Delete corresponding blobs from disk
	blobDir := filepath.Join(vaultName+VaultExtension, "data")
	for _, id := range idsToDelete {
		blobPath := filepath.Join(blobDir, id)
		os.Remove(blobPath)
	}
	return nil
}

// CopyAcrossVaults extracts raw blobs from a source vault and copies them into a target vault.
// This is necessary because Cross-Vault operations cannot use Pointer duplication (different Master Keys).
func (vm *VaultManager) CopyAcrossVaults(sourceVault, targetVault, targetParentID string, sourceIDs []string) error {
	sourceKey, ok := vm.MasterKeys[sourceVault]
	if !ok || len(sourceKey) == 0 {
		return errors.New("source vault is locked")
	}

	targetKey, ok := vm.MasterKeys[targetVault]
	if !ok || len(targetKey) == 0 {
		return errors.New("target vault is locked")
	}

	srcDB, err := vm.getDB(sourceVault)
	if err != nil {
		return err
	}

	tgtDB, err := vm.getDB(targetVault)
	if err != nil {
		return err
	}

	totalToCopy, _ := vm.countItems(srcDB, sourceIDs, sourceKey)
	current := 0
	vm.isCancelled.Store(false)
	srcCtx := dsorm.WithEncryptionKeyContext(vm.ctx, sourceKey)
	tgtCtx := dsorm.WithEncryptionKeyContext(vm.ctx, targetKey)

	var copyCrossItem func(srcDB, tgtDB *dsorm.Client, srcID, tgtParent string) error
	copyCrossItem = func(srcDB, tgtDB *dsorm.Client, srcID, tgtParent string) error {
		if err := vm.checkCancel(); err != nil {
			return err
		}
		meta := &FileMetadata{ID: srcID}
		if err := srcDB.Get(srcCtx, meta); err != nil {
			return err
		}
		if err != nil {
			return nil
		}
		if meta.IsDeleted {
			return nil
		}

		vm.emitProgress("copy", current, totalToCopy, fmt.Sprintf("Copying: %s", meta.Name))
		newID := uuid.New().String()

		if meta.IsFolder {
			newFolder := &FileMetadata{
				ID:        newID,
				ParentID:  tgtParent,
				Name:      meta.Name,
				Size:      0,
				CreatedAt: time.Now(),
				IsFolder:  true,
			}
			if err := tgtDB.Put(tgtCtx, newFolder); err != nil {
				return err
			}
			current++

			// Recurse children
			q := dsorm.NewQuery("FileMetadata").FilterField("parentId", "=", srcID)
			files, _, _ := dsorm.Query[*FileMetadata](srcCtx, srcDB, q, "")
			for _, child := range files {
				if err := copyCrossItem(srcDB, tgtDB, child.ID, newID); err != nil {
					return err
				}
			}
		} else {
			// File: Decrypt blob, re-encrypt, and save to target
			blobIDToRead := srcID
			if meta.PointerTo != "" {
				blobIDToRead = meta.PointerTo // Resolve pointer
			}

			srcBlobPath := filepath.Join(sourceVault+VaultExtension, "data", blobIDToRead)
			encryptedContent, err := os.ReadFile(srcBlobPath)
			if err == nil {
				decryptedContent, err := Decrypt(encryptedContent, sourceKey)
				if err == nil {
					reEncrypted, err := Encrypt(decryptedContent, targetKey)
					if err == nil {
						tgtBlobDir := filepath.Join(targetVault+VaultExtension, "data")
						os.MkdirAll(tgtBlobDir, 0755)
						tgtBlobPath := filepath.Join(tgtBlobDir, newID)
						os.WriteFile(tgtBlobPath, reEncrypted, 0600)

						newFile := &FileMetadata{
							ID:        newID,
							ParentID:  tgtParent,
							Name:      meta.Name,
							Size:      meta.Size,
							CreatedAt: time.Now(),
							IsFolder:  false,
						}
						if err := tgtDB.Put(tgtCtx, newFile); err != nil {
							return err
						}
						current++
					}
				}
			}
		}
		return nil
	}

	for _, srcID := range sourceIDs {
		if err := copyCrossItem(srcDB, tgtDB, srcID, targetParentID); err != nil {
			return err
		}
	}
	vm.emitProgress("copy", totalToCopy, totalToCopy, "Copy complete")
	return nil
}

// MoveAcrossVaults performs a cross-vault copy of items, then deletes the source items upon success.
func (vm *VaultManager) MoveAcrossVaults(sourceVault, targetVault, targetParentID string, sourceIDs []string) error {
	if err := vm.CopyAcrossVaults(sourceVault, targetVault, targetParentID, sourceIDs); err != nil {
		return err
	}
	// On successful copy, delete from source
	return vm.DeleteFiles(sourceVault, sourceIDs)
}

// countItems calculates the total number of items recursively given a list of IDs.
func (vm *VaultManager) countItems(db *dsorm.Client, ids []string, key []byte) (int, error) {
	total := 0
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, key)
	for _, id := range ids {
		total++
		meta := &FileMetadata{ID: id}
		if err := db.Get(ctx, meta); err != nil {
			continue
		}
		if meta.IsFolder {
			q := dsorm.NewQuery("FileMetadata").FilterField("parentId", "=", id)
			files, _, _ := dsorm.Query[*FileMetadata](ctx, db, q, "")
			var childIDs []string
			for _, d := range files {
				childIDs = append(childIDs, d.ID)
			}
			sub, _ := vm.countItems(db, childIDs, key)
			total += sub
		}
	}
	return total, nil
}

// collectDescendants recursively collects all child IDs of a folder.
func (vm *VaultManager) collectDescendants(db *dsorm.Client, parentID string, ids *[]string, masterKey []byte) error {
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
	q := dsorm.NewQuery("FileMetadata").FilterField("parentId", "=", parentID)
	files, _, _ := dsorm.Query[*FileMetadata](ctx, db, q, "")

	for _, meta := range files {
		*ids = append(*ids, meta.ID)

		if meta.IsFolder {
			vm.collectDescendants(db, meta.ID, ids, masterKey)
		}
	}
	return nil
}

// DeleteFiles removes multiple files/folders from the vault.
func (vm *VaultManager) DeleteFiles(vaultName string, fileIDs []string) error {
	masterKey, err := vm.masterKey(vaultName)
	if err != nil {
		return err
	}

	db, err := vm.getDB(vaultName)
	if err != nil {
		return err
	}

	total, _ := vm.countItems(db, fileIDs, masterKey)
	vm.isCancelled.Store(false)

	for i, id := range fileIDs {
		if err := vm.checkCancel(); err != nil {
			return err
		}
		vm.emitProgress("delete", i, total, fmt.Sprintf("Deleting item %d of %d", i+1, total))
		if err := vm.DeleteFile(vaultName, id); err != nil {
			return fmt.Errorf("failed to delete %s: %w", id, err)
		}
	}
	vm.emitProgress("delete", total, total, "Delete complete")
	return nil
}
