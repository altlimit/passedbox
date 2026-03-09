package main

import (
	"strings"

	"github.com/altlimit/dsorm"
)

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
		masterKey, ok := vm.MasterKeys[vault.Name]
		if !ok {
			continue
		}

		// Use cached file list if available, otherwise query and cache
		cached, hasCached := vm.searchCache[vault.Name]
		if !hasCached {
			ctx := dsorm.WithEncryptionKeyContext(vm.ctx, masterKey)
			db, err := vm.getDB(vault.Name)
			if err != nil {
				continue
			}

			q := dsorm.NewQuery("Filemetadata").FilterField("isDeleted", "=", false)
			files, _, err := dsorm.Query[*FileMetadata](ctx, db, q, "")
			if err != nil {
				continue
			}
			vm.searchCache[vault.Name] = files
			cached = files
		}

		// Build full file index for path lookups
		allFiles := make(map[string]*FileMetadata)
		for _, meta := range cached {
			allFiles[meta.ID] = meta
		}

		// Search and build results
		for _, meta := range cached {
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
