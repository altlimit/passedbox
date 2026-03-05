package model

import (
	"context"

	"cloud.google.com/go/datastore"
	"github.com/altlimit/dsorm"
)

type Stats struct {
	dsorm.Base
	ID        string `model:"id" json:"id"`
	Total     int    `datastore:"total,noindex" json:"total"`
	Active    int    `datastore:"active,noindex" json:"active"`
	Released  int    `datastore:"released,noindex" json:"released"`
	Pending   int    `datastore:"pending,noindex" json:"pending"`
	KeepAlive int    `datastore:"keepAlive,noindex" json:"keepAlive"`
}

// GetOrCreateStats fetches or initializes the singleton stats entity.
func GetOrCreateStats(ctx context.Context) (*Stats, error) {
	s := &Stats{ID: "main"}
	if err := Client.Get(ctx, s); err != nil {
		if err == datastore.ErrNoSuchEntity {
			return RecalculateStats(ctx)
		} else {
			return nil, err
		}
	}
	return s, nil
}

// RecalculateStats rebuilds stats from all vaults.
func RecalculateStats(ctx context.Context) (*Stats, error) {
	q := dsorm.NewQuery("Vault")
	vaults, _, err := dsorm.Query[*Vault](ctx, Client, q, "")
	if err != nil {
		return nil, err
	}

	s := &Stats{ID: "main"}
	s.Total = len(vaults)
	for _, v := range vaults {
		switch {
		case v.Released:
			s.Released++
		case v.Status == "pending":
			s.Pending++
		default:
			s.Active++
		}
		if v.EnableKeepAlive && !v.Released {
			s.KeepAlive++
		}
	}

	if err := Client.Put(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}
