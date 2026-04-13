package session

import "time"

const (
	StatusPlaying = "playing"
	StatusPaused  = "paused"
)

// PlaybackSession represents the active listening state for a user.
type PlaybackSession struct {
	UserID    string    `json:"user_id"`
	TrackID   string    `json:"track_id"`
	Position  int       `json:"position_sec"`
	Status    string    `json:"status"` // playing | paused
	DeviceID  string    `json:"device_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StartRequest is the body for POST /api/v1/sessions.
type StartRequest struct {
	TrackID  string `json:"track_id"  validate:"required"`
	DeviceID string `json:"device_id" validate:"required"`
	Position int    `json:"position_sec"`
}

// HeartbeatRequest is the body for PUT /api/v1/sessions/heartbeat.
type HeartbeatRequest struct {
	TrackID  string `json:"track_id"  validate:"required"`
	Position int    `json:"position_sec" validate:"min=0"`
	DeviceID string `json:"device_id" validate:"required"`
}

// PauseResumeRequest is the body for pause/resume endpoints.
type PauseResumeRequest struct {
	DeviceID string `json:"device_id" validate:"required"`
}
