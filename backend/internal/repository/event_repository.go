package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/galeriku/core/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrEventSlugExists is returned when the slug is already used inside the organization.
	ErrEventSlugExists = errors.New("an event with this slug already exists in this organization")

	// ErrSessionNotFound is returned when no session matches the requested identifier.
	ErrSessionNotFound = errors.New("event session not found")

	// ErrLocationNotFound is returned when no route location matches the requested identifier.
	ErrLocationNotFound = errors.New("event location not found")
)

// EventRepository defines data access for the FotoTree hierarchy.
type EventRepository interface {
	CreateEvent(ctx context.Context, event *domain.Event, eventType string) error
	GetEvent(ctx context.Context, eventID uuid.UUID) (*domain.Event, error)
	GetEventBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*domain.Event, error)
	ListEventsByOrganization(ctx context.Context, orgID uuid.UUID) ([]domain.Event, error)
	ListPublishedEvents(ctx context.Context) ([]domain.Event, error)
	UpdateEventSchedule(ctx context.Context, eventID uuid.UUID, name, status string, startsAt, endsAt *interface{}) error
	EventSlugExists(ctx context.Context, orgID uuid.UUID, slug string) (bool, error)

	CreateSession(ctx context.Context, session *domain.EventSession, sessionType string) error
	ListSessions(ctx context.Context, eventID uuid.UUID) ([]domain.EventSession, error)

	CreateLocation(ctx context.Context, location *domain.EventLocation) error
	ListLocations(ctx context.Context, eventID uuid.UUID) ([]domain.EventLocation, error)
}

// PostgresEventRepository implements EventRepository backed by PostgreSQL 16.
type PostgresEventRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresEventRepository constructs a new PostgresEventRepository.
func NewPostgresEventRepository(pool *pgxpool.Pool) *PostgresEventRepository {
	return &PostgresEventRepository{pool: pool}
}

// CreateEvent inserts one event and returns its generated identifier.
func (r *PostgresEventRepository) CreateEvent(ctx context.Context, event *domain.Event, eventType string) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO events (host_organization_id, slug, name, status, starts_at, ends_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, event.HostOrganizationID, event.Slug, event.Name, event.Status, event.StartsAt, event.EndsAt, event.CreatedBy).Scan(&event.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrEventSlugExists
		}
		return fmt.Errorf("failed to insert event: %w", err)
	}
	_ = eventType
	return nil
}

// GetEvent loads one event by identifier.
func (r *PostgresEventRepository) GetEvent(ctx context.Context, eventID uuid.UUID) (*domain.Event, error) {
	var e domain.Event
	err := r.pool.QueryRow(ctx, `
		SELECT id, host_organization_id, slug, name, status, starts_at, ends_at, created_by
		FROM events WHERE id = $1
	`, eventID).Scan(&e.ID, &e.HostOrganizationID, &e.Slug, &e.Name, &e.Status, &e.StartsAt, &e.EndsAt, &e.CreatedBy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, fmt.Errorf("failed to query event: %w", err)
	}
	return &e, nil
}

// GetEventBySlug loads one event by slug inside an organization.
func (r *PostgresEventRepository) GetEventBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*domain.Event, error) {
	var e domain.Event
	err := r.pool.QueryRow(ctx, `
		SELECT id, host_organization_id, slug, name, status, starts_at, ends_at, created_by
		FROM events WHERE host_organization_id = $1 AND slug = $2
	`, orgID, slug).Scan(&e.ID, &e.HostOrganizationID, &e.Slug, &e.Name, &e.Status, &e.StartsAt, &e.EndsAt, &e.CreatedBy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, fmt.Errorf("failed to query event by slug: %w", err)
	}
	return &e, nil
}

// ListPublishedEvents lists every PUBLISHED event across organizations for
// the public storefront catalog (guest browsing per UI_UX_SPEC W-01).
func (r *PostgresEventRepository) ListPublishedEvents(ctx context.Context) ([]domain.Event, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, host_organization_id, slug, name, status, starts_at, ends_at, created_by
		FROM events WHERE status = 'PUBLISHED'
		ORDER BY starts_at DESC NULLS LAST, name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list published events: %w", err)
	}
	defer rows.Close()

	events := make([]domain.Event, 0)
	for rows.Next() {
		var e domain.Event
		if err := rows.Scan(&e.ID, &e.HostOrganizationID, &e.Slug, &e.Name, &e.Status, &e.StartsAt, &e.EndsAt, &e.CreatedBy); err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// ListEventsByOrganization lists every event of one organization, newest first.
func (r *PostgresEventRepository) ListEventsByOrganization(ctx context.Context, orgID uuid.UUID) ([]domain.Event, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, host_organization_id, slug, name, status, starts_at, ends_at, created_by
		FROM events WHERE host_organization_id = $1
		ORDER BY starts_at DESC NULLS LAST, name ASC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}
	defer rows.Close()

	events := make([]domain.Event, 0)
	for rows.Next() {
		var e domain.Event
		if err := rows.Scan(&e.ID, &e.HostOrganizationID, &e.Slug, &e.Name, &e.Status, &e.StartsAt, &e.EndsAt, &e.CreatedBy); err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading event rows: %w", err)
	}
	return events, nil
}

// UpdateEventSchedule updates the mutable event fields and reports whether a row matched.
func (r *PostgresEventRepository) UpdateEventSchedule(ctx context.Context, eventID uuid.UUID, name, status string, startsAt, endsAt *interface{}) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE events
		SET name = COALESCE(NULLIF($2, ''), name),
		    status = COALESCE(NULLIF($3, ''), status),
		    starts_at = COALESCE($4, starts_at),
		    ends_at = COALESCE($5, ends_at)
		WHERE id = $1
	`, eventID, name, status, derefTime(startsAt), derefTime(endsAt))
	if err != nil {
		return fmt.Errorf("failed to update event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEventNotFound
	}
	return nil
}

func derefTime(v *interface{}) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

// EventSlugExists reports whether the slug is already taken inside the organization.
func (r *PostgresEventRepository) EventSlugExists(ctx context.Context, orgID uuid.UUID, slug string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM events WHERE host_organization_id = $1 AND slug = $2)", orgID, slug).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check event slug: %w", err)
	}
	return exists, nil
}

// CreateSession inserts one FotoTree session.
func (r *PostgresEventRepository) CreateSession(ctx context.Context, session *domain.EventSession, sessionType string) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO event_sessions (event_id, name, starts_at, ends_at, venue_id, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, session.EventID, session.Name, session.StartsAt, session.EndsAt, session.VenueID, session.Status).Scan(&session.ID)
	if err != nil {
		return fmt.Errorf("failed to insert event session: %w", err)
	}
	_ = sessionType
	return nil
}

// ListSessions lists every session of one event in schedule order.
func (r *PostgresEventRepository) ListSessions(ctx context.Context, eventID uuid.UUID) ([]domain.EventSession, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, event_id, name, starts_at, ends_at, venue_id, status
		FROM event_sessions WHERE event_id = $1
		ORDER BY starts_at ASC NULLS LAST, name ASC
	`, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to list event sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]domain.EventSession, 0)
	for rows.Next() {
		var s domain.EventSession
		if err := rows.Scan(&s.ID, &s.EventID, &s.Name, &s.StartsAt, &s.EndsAt, &s.VenueID, &s.Status); err != nil {
			return nil, fmt.Errorf("failed to scan event session: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading session rows: %w", err)
	}
	return sessions, nil
}

// CreateLocation inserts one route checkpoint.
func (r *PostgresEventRepository) CreateLocation(ctx context.Context, location *domain.EventLocation) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO event_locations (event_id, name, latitude, longitude, route_order, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, location.EventID, location.Name, location.Latitude, location.Longitude, location.RouteOrder, location.Status).Scan(&location.ID)
	if err != nil {
		return fmt.Errorf("failed to insert event location: %w", err)
	}
	return nil
}

// ListLocations lists every route checkpoint of one event in route order.
func (r *PostgresEventRepository) ListLocations(ctx context.Context, eventID uuid.UUID) ([]domain.EventLocation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, event_id, name, latitude, longitude, route_order, status
		FROM event_locations WHERE event_id = $1
		ORDER BY route_order ASC NULLS LAST, name ASC
	`, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to list event locations: %w", err)
	}
	defer rows.Close()

	locations := make([]domain.EventLocation, 0)
	for rows.Next() {
		var l domain.EventLocation
		if err := rows.Scan(&l.ID, &l.EventID, &l.Name, &l.Latitude, &l.Longitude, &l.RouteOrder, &l.Status); err != nil {
			return nil, fmt.Errorf("failed to scan event location: %w", err)
		}
		locations = append(locations, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading location rows: %w", err)
	}
	return locations, nil
}

// isUniqueViolation reports whether an error is a PostgreSQL unique constraint violation.
func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
