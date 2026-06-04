package cupboard

import (
	"time"

	"github.com/google/uuid"
)

// Sip is the basic unit of information in Espresso API.
// Each sip is stored as an immutable record with a UUID primary key, a creation timestamp, and a
// JSON digest whose shape depends on the sip kind.
//
// There are three sip kinds:
//   - action: micro-level data points such as market performance for a given day
//   - event: a self-contained set of related micro actions and actions (for example a court ruling and its local fallout)
//   - signal: larger derived intelligence synthesized from related events and actions (for example cross-domain market and policy outlook)
//
// List endpoints do not return this struct directly; the router flattens Digest and merges id and created into each JSON object.
type Sip struct {
	ID      uuid.UUID      `db:"id" json:"id" swaggertype:"string" format:"uuid" example:"339366bc-464d-582f-8132-6875ccc814d2"`
	Created time.Time      `db:"created" json:"created" example:"2026-05-19T06:00:00-04:00"`
	Digest  map[string]any `db:"digest" swaggertype:"object"`
}

// Source describes a content publisher tracked in the database.
type Source struct {
	ID          uuid.UUID `db:"id" json:"id" swaggertype:"string" format:"uuid" example:"a1b2c3d4-e5f6-7890-abcd-ef1234567890"`
	BaseURL     string    `db:"base_url" json:"base_url" example:"https://example.com"`
	DomainName  string    `db:"domain_name" json:"domain_name" example:"example.com"`
	SiteName    string    `db:"site_name" json:"site_name" example:"Example News"`
	Description string    `db:"description" json:"description" example:"Independent business and policy coverage."`
	Favicon     string    `db:"favicon" json:"favicon" example:"https://example.com/favicon.ico"`
}

// Relation links two sips by a named relationship (for example SAME_AS or DERIVED_FROM).
type Relation struct {
	FromID       uuid.UUID `db:"from_id" json:"from_id" swaggertype:"string" format:"uuid" example:"b07049b5-54c0-50b0-a620-d3aea3f8a173"`
	ToID         uuid.UUID `db:"to_id" json:"to_id" swaggertype:"string" format:"uuid" example:"9c3cc0a2-6eea-5290-9e9b-b5c462aeaa3a"`
	Relationship string    `db:"relationship" json:"relationship" example:"SAME_AS" enums:"SAME_AS,DERIVED_FROM"`
}
