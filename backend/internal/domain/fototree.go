package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	// EventStatusDraft is the initial state of a newly created event.
	EventStatusDraft = "DRAFT"

	// EventStatusPublished makes the event visible to the public catalog.
	EventStatusPublished = "PUBLISHED"

	// EventStatusArchived hides a completed event from the public catalog.
	EventStatusArchived = "ARCHIVED"

	// SessionStatusScheduled marks a session that has not started.
	SessionStatusScheduled = "SCHEDULED"

	// SessionStatusActive marks a running session.
	SessionStatusActive = "ACTIVE"

	// SessionStatusCompleted marks a finished session.
	SessionStatusCompleted = "COMPLETED"

	// LocationStatusActive marks a route checkpoint in use.
	LocationStatusActive = "ACTIVE"

	// LocationStatusInactive marks a retired route checkpoint.
	LocationStatusInactive = "INACTIVE"
)

// Event is one FotoTree root: a photography event owned by a Host organization.
type Event struct {
	ID                 uuid.UUID  `json:"id"`
	HostOrganizationID uuid.UUID  `json:"host_organization_id"`
	Slug               string     `json:"slug"`
	Name               string     `json:"name"`
	Status             string     `json:"status"`
	StartsAt           *time.Time `json:"starts_at,omitempty"`
	EndsAt             *time.Time `json:"ends_at,omitempty"`
	CreatedBy          uuid.UUID  `json:"created_by"`
}

// EventSession is one timing branch of an event, such as a start wave or a track checkpoint.
type EventSession struct {
	ID       uuid.UUID  `json:"id"`
	EventID  uuid.UUID  `json:"event_id"`
	Name     string     `json:"name"`
	StartsAt *time.Time `json:"starts_at,omitempty"`
	EndsAt   *time.Time `json:"ends_at,omitempty"`
	VenueID  *uuid.UUID `json:"venue_id,omitempty"`
	Status   string     `json:"status"`
}

// EventLocation is one route checkpoint with geographic coordinates.
type EventLocation struct {
	ID         uuid.UUID `json:"id"`
	EventID    uuid.UUID `json:"event_id"`
	Name       string    `json:"name"`
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	RouteOrder *int      `json:"route_order,omitempty"`
	Status     string    `json:"status"`
}

// EventTree is the full FotoTree for one event: its sessions and route locations.
type EventTree struct {
	Event     Event           `json:"event"`
	Sessions  []EventSession  `json:"sessions"`
	Locations []EventLocation `json:"locations"`
}
