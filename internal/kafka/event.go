package kafka

import (
	"encoding/json"
	"fmt"
	"time"
)

// Envelope wraps every domain event with metadata for schema evolution and tracing.
type Envelope struct {
	Type      string          `json:"type"`
	Version   int             `json:"version"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

func NewEnvelope(eventType string, version int, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal event payload: %w", err)
	}

	env := Envelope{
		Type:      eventType,
		Version:   version,
		Timestamp: time.Now().UTC(),
		Payload:   raw,
	}

	data, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}
	return data, nil
}

func ParseEnvelope(data []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Envelope{}, fmt.Errorf("unmarshal envelope: %w", err)
	}
	return env, nil
}
