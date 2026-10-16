package usecase

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/galeriku/core/internal/audit"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/google/uuid"
)

// slugPattern keeps slugs URL-safe and stable: lowercase letters, digits, and single hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// EventTreeUsecase orchestrates FotoTree hierarchy management.
type EventTreeUsecase interface {
	CreateEvent(ctx context.Context, actorUserID, orgID uuid.UUID, req CreateEventRequest) (*domain.Event, error)
	GetEventTree(ctx context.Context, eventID uuid.UUID) (*domain.EventTree, error)
	ListEvents(ctx context.Context, orgID uuid.UUID) ([]domain.Event, error)
	ListPublishedEvents(ctx context.Context) ([]domain.Event, error)
	UpdateEvent(ctx context.Context, actorUserID, eventID uuid.UUID, req UpdateEventRequest) (*domain.Event, error)
	CreateSession(ctx context.Context, actorUserID, eventID uuid.UUID, req CreateSessionRequest) (*domain.EventSession, error)
	CreateLocation(ctx context.Context, actorUserID, eventID uuid.UUID, req CreateLocationRequest) (*domain.EventLocation, error)
}

// CreateEventRequest is the validated input for POST /events.
type CreateEventRequest struct {
	Name      string
	Slug      string
	EventType string
	StartsAt  *time.Time
	EndsAt    *time.Time
}

// UpdateEventRequest carries the mutable event fields.
type UpdateEventRequest struct {
	Name     string
	Status   string
	StartsAt *time.Time
	EndsAt   *time.Time
}

// CreateSessionRequest is the validated input for POST /events/{id}/sessions.
type CreateSessionRequest struct {
	Name        string
	SessionType string
	StartsAt    *time.Time
	EndsAt      *time.Time
}

// CreateLocationRequest is the validated input for POST /events/{id}/locations.
type CreateLocationRequest struct {
	Name       string
	Latitude   float64
	Longitude  float64
	RouteOrder *int
}

type eventTreeUsecase struct {
	repo    repository.EventRepository
	auditor audit.Logger
}

// NewEventTreeUsecase constructs the FotoTree usecase.
// auditor may be nil, in which case mutations proceed without audit records.
func NewEventTreeUsecase(repo repository.EventRepository, auditor audit.Logger) EventTreeUsecase {
	return &eventTreeUsecase{repo: repo, auditor: auditor}
}

func (u *eventTreeUsecase) recordAudit(ctx context.Context, entry audit.Entry) {
	if u.auditor == nil {
		return
	}
	if err := u.auditor.Log(ctx, entry); err != nil {
		log.Printf("[AUDIT] failed to record %s: %v", entry.ActionCode, err)
	}
}

func (u *eventTreeUsecase) CreateEvent(ctx context.Context, actorUserID, orgID uuid.UUID, req CreateEventRequest) (*domain.Event, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	req.EventType = strings.TrimSpace(req.EventType)

	if req.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if !slugPattern.MatchString(req.Slug) {
		return nil, fmt.Errorf("%w: slug must contain lowercase letters, digits, and single hyphens only", ErrInvalidInput)
	}
	if req.EventType == "" {
		return nil, fmt.Errorf("%w: event_type is required", ErrInvalidInput)
	}
	if req.StartsAt != nil && req.EndsAt != nil && req.EndsAt.Before(*req.StartsAt) {
		return nil, fmt.Errorf("%w: ends_at must not be earlier than starts_at", ErrInvalidInput)
	}
	if len(req.Name) > 255 || len(req.Slug) > 128 {
		return nil, fmt.Errorf("%w: name or slug exceeds the allowed length", ErrInvalidInput)
	}

	exists, err := u.repo.EventSlugExists(ctx, orgID, req.Slug)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, repository.ErrEventSlugExists
	}

	event := &domain.Event{
		HostOrganizationID: orgID,
		Slug:               req.Slug,
		Name:               req.Name,
		Status:             domain.EventStatusDraft,
		StartsAt:           req.StartsAt,
		EndsAt:             req.EndsAt,
		CreatedBy:          actorUserID,
	}
	if err := u.repo.CreateEvent(ctx, event, req.EventType); err != nil {
		return nil, err
	}

	u.recordAudit(ctx, audit.Entry{
		ActorUserID: &actorUserID,
		ActionCode:  "EVENT_CREATED",
		TargetType:  "event",
		TargetID:    &event.ID,
		Metadata:    map[string]interface{}{"organization_id": orgID.String(), "slug": event.Slug},
	})
	return event, nil
}

func (u *eventTreeUsecase) GetEventTree(ctx context.Context, eventID uuid.UUID) (*domain.EventTree, error) {
	event, err := u.repo.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	sessions, err := u.repo.ListSessions(ctx, eventID)
	if err != nil {
		return nil, err
	}
	locations, err := u.repo.ListLocations(ctx, eventID)
	if err != nil {
		return nil, err
	}
	return &domain.EventTree{Event: *event, Sessions: sessions, Locations: locations}, nil
}

func (u *eventTreeUsecase) ListEvents(ctx context.Context, orgID uuid.UUID) ([]domain.Event, error) {
	return u.repo.ListEventsByOrganization(ctx, orgID)
}

// ListPublishedEvents serves the public storefront catalog.
func (u *eventTreeUsecase) ListPublishedEvents(ctx context.Context) ([]domain.Event, error) {
	return u.repo.ListPublishedEvents(ctx)
}

func (u *eventTreeUsecase) UpdateEvent(ctx context.Context, actorUserID, eventID uuid.UUID, req UpdateEventRequest) (*domain.Event, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Status = strings.ToUpper(strings.TrimSpace(req.Status))

	if req.Status != "" && req.Status != domain.EventStatusDraft &&
		req.Status != domain.EventStatusPublished && req.Status != domain.EventStatusArchived {
		return nil, fmt.Errorf("%w: status must be DRAFT, PUBLISHED, or ARCHIVED", ErrInvalidInput)
	}
	if req.StartsAt != nil && req.EndsAt != nil && req.EndsAt.Before(*req.StartsAt) {
		return nil, fmt.Errorf("%w: ends_at must not be earlier than starts_at", ErrInvalidInput)
	}

	var startsArg, endsArg *interface{}
	if req.StartsAt != nil {
		var v interface{} = *req.StartsAt
		startsArg = &v
	}
	if req.EndsAt != nil {
		var v interface{} = *req.EndsAt
		endsArg = &v
	}

	if err := u.repo.UpdateEventSchedule(ctx, eventID, req.Name, req.Status, startsArg, endsArg); err != nil {
		return nil, err
	}

	event, err := u.repo.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}

	u.recordAudit(ctx, audit.Entry{
		ActorUserID: &actorUserID,
		ActionCode:  "EVENT_UPDATED",
		TargetType:  "event",
		TargetID:    &eventID,
		Metadata:    map[string]interface{}{"organization_id": event.HostOrganizationID.String(), "status": event.Status},
	})
	return event, nil
}

func (u *eventTreeUsecase) CreateSession(ctx context.Context, actorUserID, eventID uuid.UUID, req CreateSessionRequest) (*domain.EventSession, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.SessionType = strings.TrimSpace(req.SessionType)

	if req.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if req.SessionType == "" {
		return nil, fmt.Errorf("%w: session_type is required", ErrInvalidInput)
	}
	if req.StartsAt != nil && req.EndsAt != nil && req.EndsAt.Before(*req.StartsAt) {
		return nil, fmt.Errorf("%w: ends_at must not be earlier than starts_at", ErrInvalidInput)
	}

	session := &domain.EventSession{
		EventID:  eventID,
		Name:     req.Name,
		StartsAt: req.StartsAt,
		EndsAt:   req.EndsAt,
		Status:   domain.SessionStatusScheduled,
	}
	if err := u.repo.CreateSession(ctx, session, req.SessionType); err != nil {
		return nil, err
	}

	u.recordAudit(ctx, audit.Entry{
		ActorUserID: &actorUserID,
		ActionCode:  "EVENT_SESSION_CREATED",
		TargetType:  "event_session",
		TargetID:    &session.ID,
		Metadata:    map[string]interface{}{"event_id": eventID.String()},
	})
	return session, nil
}

func (u *eventTreeUsecase) CreateLocation(ctx context.Context, actorUserID, eventID uuid.UUID, req CreateLocationRequest) (*domain.EventLocation, error) {
	req.Name = strings.TrimSpace(req.Name)

	if req.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if req.Latitude < -90 || req.Latitude > 90 {
		return nil, fmt.Errorf("%w: latitude must be between -90 and 90", ErrInvalidInput)
	}
	if req.Longitude < -180 || req.Longitude > 180 {
		return nil, fmt.Errorf("%w: longitude must be between -180 and 180", ErrInvalidInput)
	}
	if req.RouteOrder != nil && *req.RouteOrder < 0 {
		return nil, fmt.Errorf("%w: route_order must not be negative", ErrInvalidInput)
	}

	location := &domain.EventLocation{
		EventID:    eventID,
		Name:       req.Name,
		Latitude:   req.Latitude,
		Longitude:  req.Longitude,
		RouteOrder: req.RouteOrder,
		Status:     domain.LocationStatusActive,
	}
	if err := u.repo.CreateLocation(ctx, location); err != nil {
		return nil, err
	}

	u.recordAudit(ctx, audit.Entry{
		ActorUserID: &actorUserID,
		ActionCode:  "EVENT_LOCATION_CREATED",
		TargetType:  "event_location",
		TargetID:    &location.ID,
		Metadata:    map[string]interface{}{"event_id": eventID.String()},
	})
	return location, nil
}
