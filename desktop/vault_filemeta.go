package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/altlimit/dsorm"
)

type Payload struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type FileMetadata struct {
	dsorm.Base
	ID           string    `json:"id,omitempty" datastore:"-" model:"id"`
	ParentID     string    `json:"parentId,omitempty" datastore:"parentId"`
	Name         string    `json:"name,omitempty" datastore:"-"`
	Size         int64     `json:"size,omitempty" datastore:"-"`
	CreatedAt    time.Time `json:"createdAt,omitempty" datastore:"createdAt" model:"created"`
	UpdatedAt    time.Time `json:"updatedAt,omitempty" datastore:"updatedAt" model:"modified"`
	IsFolder     bool      `json:"isFolder,omitempty" datastore:"isFolder"`
	PointerTo    string    `json:"pointerTo,omitempty" datastore:"pointerTo"`       // ID of the original file blob this struct references
	PointersFrom []string  `json:"pointersFrom,omitempty" datastore:"pointersFrom"` // IDs of copies that reference this file's blob
	IsDeleted    bool      `json:"isDeleted,omitempty" datastore:"isDeleted"`       // If true, hide from UI. Blob kept alive until PointersFrom is empty.
	Payload      []byte    `json:"payload,omitempty" model:"payload,marshal"`       // Encrypted file data
}

func (fm *FileMetadata) OnLoad(ctx context.Context) error {

	masterKey := ctx.Value("masterKey").([]byte)
	return fm.Decrypt(masterKey)
}

func (fm *FileMetadata) BeforeSave(ctx context.Context, old dsorm.Model) error {
	masterKey := ctx.Value("masterKey").([]byte)
	return fm.Encrypt(masterKey)
}

func (fm *FileMetadata) Decrypt(masterKey []byte) error {
	decrypted, err := Decrypt(fm.Payload, masterKey)
	if err != nil {
		return err
	}
	payload := &Payload{}
	if err := json.Unmarshal(decrypted, payload); err != nil {
		return err
	}
	fm.Name = payload.Name
	fm.Size = payload.Size
	return nil
}

func (fm *FileMetadata) Encrypt(masterKey []byte) error {
	payload := &Payload{
		Name: fm.Name,
		Size: fm.Size,
	}
	enc, err := payload.Encrypt(masterKey)
	fm.Payload = enc
	return err
}

func (p *Payload) Encrypt(masterKey []byte) ([]byte, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %v", err)
	}
	encryptedData, err := Encrypt(data, masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt metadata: %v", err)
	}

	return encryptedData, nil
}
