package models

import "time"

// User represents a registered developer
type User struct {
	ID        int       `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// Endpoint is a target URL registered by a user to receive webhooks
type Endpoint struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret,omitempty"` // HMAC signing secret
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// DeliveryStatus constants
const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
	StatusRetrying  = "retrying"
)

// Delivery represents a single webhook delivery attempt log
type Delivery struct {
	ID             int        `json:"id"`
	EndpointID     int        `json:"endpoint_id"`
	Payload        string     `json:"payload"`
	EventType      string     `json:"event_type"`
	Status         string     `json:"status"`
	AttemptCount   int        `json:"attempt_count"`
	MaxAttempts    int        `json:"max_attempts"`
	NextRetryAt    *time.Time `json:"next_retry_at,omitempty"`
	LastStatusCode int        `json:"last_status_code,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// DeliveryAttempt is a log of each individual HTTP attempt
type DeliveryAttempt struct {
	ID         int       `json:"id"`
	DeliveryID int       `json:"delivery_id"`
	StatusCode int       `json:"status_code"`
	Error      string    `json:"error,omitempty"`
	Duration   int       `json:"duration_ms"`
	AttemptedAt time.Time `json:"attempted_at"`
}

// --- Request / Response structs ---

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type CreateEndpointRequest struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

type SendWebhookRequest struct {
	EndpointID  int    `json:"endpoint_id"`
	EventType   string `json:"event_type"`
	Payload     string `json:"payload"`
	MaxAttempts int    `json:"max_attempts"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type MessageResponse struct {
	Message string `json:"message"`
}
