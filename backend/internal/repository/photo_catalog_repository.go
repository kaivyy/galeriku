package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrPhotoCatalogNotFound is returned when no catalog photo row matches.
var ErrPhotoCatalogNotFound = errors.New("photo catalog entry not found")

// PhotoCatalogRepository provides read-only access to the public photo catalog.
// It joins photos, photo_prices, and photo_event_links to return storefront-ready
// results without exposing uploader identity or internal status.
type PhotoCatalogRepository interface {
	ListPublicPhotos(ctx context.Context, limit int) ([]PublicPhoto, error)
	ListPhotosByEvent(ctx context.Context, eventID uuid.UUID) ([]PublicPhoto, error)
	GetPhotoByID(ctx context.Context, photoID uuid.UUID) (*PublicPhoto, error)
}

// PublicPhoto is the storefront-safe view of one photo.
type PublicPhoto struct {
	PhotoID  uuid.UUID `json:"photo_id"`
	PriceIDR int64     `json:"price_idr"`
	EventID  *uuid.UUID `json:"event_id,omitempty"`
}

type postgresPhotoCatalogRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresPhotoCatalogRepository constructs a new catalog repository.
func NewPostgresPhotoCatalogRepository(pool *pgxpool.Pool) PhotoCatalogRepository {
	return &postgresPhotoCatalogRepository{pool: pool}
}

func (r *postgresPhotoCatalogRepository) ListPublicPhotos(ctx context.Context, limit int) ([]PublicPhoto, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, COALESCE(pp.price_idr, 0), pe.event_id
		FROM photos p
		LEFT JOIN photo_prices pp ON pp.photo_id = p.id
		LEFT JOIN photo_event_links pe ON pe.photo_id = p.id
		WHERE p.status = 'READY'
		ORDER BY p.created_at DESC
		LIMIT $1
	`, limit)
	return r.scanPhotos(rows, err)
}

func (r *postgresPhotoCatalogRepository) ListPhotosByEvent(ctx context.Context, eventID uuid.UUID) ([]PublicPhoto, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, COALESCE(pp.price_idr, 0), pe.event_id
		FROM photos p
		JOIN photo_event_links pe ON pe.photo_id = p.id
		LEFT JOIN photo_prices pp ON pp.photo_id = p.id
		WHERE p.status = 'READY' AND pe.event_id = $1
		ORDER BY p.created_at DESC
	`, eventID)
	return r.scanPhotos(rows, err)
}

func (r *postgresPhotoCatalogRepository) GetPhotoByID(ctx context.Context, photoID uuid.UUID) (*PublicPhoto, error) {
	var p PublicPhoto
	err := r.pool.QueryRow(ctx, `
		SELECT p.id, COALESCE(pp.price_idr, 0), pe.event_id
		FROM photos p
		LEFT JOIN photo_prices pp ON pp.photo_id = p.id
		LEFT JOIN photo_event_links pe ON pe.photo_id = p.id
		WHERE p.id = $1 AND p.status = 'READY'
	`, photoID).Scan(&p.PhotoID, &p.PriceIDR, &p.EventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPhotoCatalogNotFound
		}
		return nil, fmt.Errorf("failed to query public photo: %w", err)
	}
	return &p, nil
}

func (r *postgresPhotoCatalogRepository) scanPhotos(rows pgx.Rows, err error) ([]PublicPhoto, error) {
	if err != nil {
		return nil, fmt.Errorf("failed to query public photos: %w", err)
	}
	defer rows.Close()

	photos := make([]PublicPhoto, 0)
	for rows.Next() {
		var p PublicPhoto
		if err := rows.Scan(&p.PhotoID, &p.PriceIDR, &p.EventID); err != nil {
			return nil, fmt.Errorf("failed to scan public photo: %w", err)
		}
		photos = append(photos, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading public photos: %w", err)
	}
	return photos, nil
}
