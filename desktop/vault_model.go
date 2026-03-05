package main

import (
	"context"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/altlimit/dsorm"
	"golang.org/x/crypto/argon2"
)

type FileMetadata struct {
	dsorm.Base
	ID           string    `json:"id,omitempty" datastore:"-" model:"id"`
	ParentID     string    `json:"parentId,omitempty" datastore:"parentId"`
	Name         string    `json:"name,omitempty" model:"name,encrypt"`
	Size         int64     `json:"size,omitempty" model:"size,encrypt"`
	CreatedAt    time.Time `json:"createdAt,omitempty" datastore:"createdAt" model:"created"`
	UpdatedAt    time.Time `json:"updatedAt,omitempty" datastore:"updatedAt" model:"modified"`
	IsFolder     bool      `json:"isFolder,omitempty" datastore:"isFolder"`
	PointerTo    string    `json:"pointerTo,omitempty" datastore:"pointerTo"`       // ID of the original file blob this struct references
	PointersFrom []string  `json:"pointersFrom,omitempty" datastore:"pointersFrom"` // IDs of copies that reference this file's blob
	IsDeleted    bool      `json:"isDeleted,omitempty" datastore:"isDeleted"`       // If true, hide from UI. Blob kept alive until PointersFrom is empty.
}

type VaultMetadata struct {
	dsorm.Base
	ID        string `json:"id,omitempty" datastore:"-" model:"id"`
	VaultID   string `json:"vaultId,omitempty" datastore:"vaultId"`
	Secret    Secret `json:"secret,omitempty" model:"secret,encrypt" datastore:"-"`
	DMS       DMS    `json:"dms,omitempty" model:"dms,encrypt" datastore:"-"`
	Recovered bool   `json:"recovered,omitempty" datastore:"recovered"` // True when vault was recovered via DMS without password
}

type DMS struct {
	Enabled   bool   `json:"enabled,omitempty"`
	ServerURL string `json:"serverUrl,omitempty"`
	Token     string `json:"token,omitempty"`
}

type Secret struct {
	Share1    []byte `json:"share1,omitempty"`
	Share2Enc []byte `json:"share2,omitempty"`
	Share3Key []byte `json:"share3,omitempty"`
	Salt      []byte `json:"salt,omitempty"`
}

type VaultState struct {
	dsorm.Base
	ID        string `json:"id,omitempty" datastore:"-" model:"id"`
	UsePepper bool   `json:"usePepper,omitempty" datastore:"usePepper"`
	Key       []byte `json:"key,omitempty" model:"key,marshal"`
}

func (vs *VaultState) GetKey() []byte {
	if vs.UsePepper {
		info := getDevicePepperInfoOS()
		if info.Available {
			return argon2.IDKey([]byte(info.SerialID), vs.Key, 1, 16*1024, 1, 32)
		}
	}
	return vs.Key
}

func (vm *VaultMetadata) GetState(ctx context.Context, db *dsorm.Client) (*VaultState, error) {
	vs := &VaultState{ID: vm.ID}
	if err := db.Get(ctx, vs); err == datastore.ErrNoSuchEntity {
		var err error
		vs.Key, err = GenerateRandomBytes(32)
		if err != nil {
			return nil, err
		}
		if err := db.Put(ctx, vs); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return vs, nil
}

func (vm *VaultMetadata) Update(ctx context.Context, db *dsorm.Client) error {
	vs, err := vm.GetState(ctx, db)
	if err != nil {
		return err
	}
	ctx = dsorm.WithEncryptionKeyContext(ctx, vs.GetKey())
	return db.Put(ctx, vm)
}

func (vm *VaultManager) vaultMetadata(vaultName string) (*VaultMetadata, *VaultState, error) {
	db, err := vm.getDB(vaultName)
	if err != nil {
		return nil, nil, err
	}
	vmeta := &VaultMetadata{ID: "main"}
	vstate, err := vmeta.GetState(vm.ctx, db)
	if err != nil {
		return nil, nil, err
	}
	ctx := dsorm.WithEncryptionKeyContext(vm.ctx, vstate.GetKey())
	if err := db.Get(ctx, vmeta); err != nil {
		return nil, nil, err
	}
	return vmeta, vstate, nil
}
