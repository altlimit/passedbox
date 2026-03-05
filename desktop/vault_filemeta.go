package main

import (
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
	Name         string    `json:"name,omitempty" model:"name,encrypt"`
	Size         int64     `json:"size,omitempty" model:"size,encrypt"`
	CreatedAt    time.Time `json:"createdAt,omitempty" datastore:"createdAt" model:"created"`
	UpdatedAt    time.Time `json:"updatedAt,omitempty" datastore:"updatedAt" model:"modified"`
	IsFolder     bool      `json:"isFolder,omitempty" datastore:"isFolder"`
	PointerTo    string    `json:"pointerTo,omitempty" datastore:"pointerTo"`       // ID of the original file blob this struct references
	PointersFrom []string  `json:"pointersFrom,omitempty" datastore:"pointersFrom"` // IDs of copies that reference this file's blob
	IsDeleted    bool      `json:"isDeleted,omitempty" datastore:"isDeleted"`       // If true, hide from UI. Blob kept alive until PointersFrom is empty.
}
