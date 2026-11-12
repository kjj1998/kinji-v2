package queue

import "github.com/google/uuid"

// the SQS message body sent by ingest and sweeper and read by enrich
type EventMessage struct {
	EventID uuid.UUID `json:"event_id"`
}
