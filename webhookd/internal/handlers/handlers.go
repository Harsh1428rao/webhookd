package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Harsh1428rao/webhookd/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	db *sql.DB
}

func New(db *sql.DB) *Handler {
	return &Handler{db: db}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, models.ErrorResponse{Error: msg})
}

func jwtSecret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		s = "webhookd-secret-change-in-production"
	}
	return []byte(s)
}

func generateToken(userID int) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(72 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret())
}

// ─── Auth Middleware ──────────────────────────────────────────────────────────

func (h *Handler) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing or invalid authorization header")
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			return jwtSecret(), nil
		})
		if err != nil || !token.Valid {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid token claims")
			return
		}
		userID := strconv.FormatFloat(claims["user_id"].(float64), 'f', 0, 64)
		r.Header.Set("X-User-ID", userID)
		next(w, r)
	}
}

// ─── Auth ─────────────────────────────────────────────────────────────────────

// POST /api/auth/register
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	var user models.User
	err = h.db.QueryRow(
		`INSERT INTO users (email, password) VALUES ($1, $2) RETURNING id, email, created_at`,
		req.Email, string(hashed),
	).Scan(&user.ID, &user.Email, &user.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	token, err := generateToken(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	writeJSON(w, http.StatusCreated, models.AuthResponse{Token: token, User: user})
}

// POST /api/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var user models.User
	err := h.db.QueryRow(
		`SELECT id, email, password, created_at FROM users WHERE email = $1`, req.Email,
	).Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := generateToken(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	writeJSON(w, http.StatusOK, models.AuthResponse{Token: token, User: user})
}

// ─── Endpoints ────────────────────────────────────────────────────────────────

// POST /api/endpoints
func (h *Handler) CreateEndpoint(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.Atoi(r.Header.Get("X-User-ID"))
	var req models.CreateEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "name and url are required")
		return
	}
	var ep models.Endpoint
	err := h.db.QueryRow(
		`INSERT INTO endpoints (user_id, name, url, secret)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, user_id, name, url, secret, is_active, created_at`,
		userID, req.Name, req.URL, req.Secret,
	).Scan(&ep.ID, &ep.UserID, &ep.Name, &ep.URL, &ep.Secret, &ep.IsActive, &ep.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create endpoint")
		return
	}
	writeJSON(w, http.StatusCreated, ep)
}

// GET /api/endpoints
func (h *Handler) GetEndpoints(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.Atoi(r.Header.Get("X-User-ID"))
	rows, err := h.db.Query(
		`SELECT id, user_id, name, url, is_active, created_at
		 FROM endpoints WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch endpoints")
		return
	}
	defer rows.Close()
	endpoints := []models.Endpoint{}
	for rows.Next() {
		var ep models.Endpoint
		rows.Scan(&ep.ID, &ep.UserID, &ep.Name, &ep.URL, &ep.IsActive, &ep.CreatedAt)
		endpoints = append(endpoints, ep)
	}
	writeJSON(w, http.StatusOK, endpoints)
}

// DELETE /api/endpoints/{id}
func (h *Handler) DeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.Atoi(r.Header.Get("X-User-ID"))
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	res, err := h.db.Exec(`DELETE FROM endpoints WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete endpoint")
		return
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "endpoint not found")
		return
	}
	writeJSON(w, http.StatusOK, models.MessageResponse{Message: "endpoint deleted"})
}

// ─── Webhooks ─────────────────────────────────────────────────────────────────

// POST /api/webhooks/send
func (h *Handler) SendWebhook(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.Atoi(r.Header.Get("X-User-ID"))

	var req models.SendWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.EndpointID == 0 || req.Payload == "" {
		writeError(w, http.StatusBadRequest, "endpoint_id and payload are required")
		return
	}
	if req.EventType == "" {
		req.EventType = "generic"
	}
	if req.MaxAttempts == 0 {
		req.MaxAttempts = 5
	}

	// Verify endpoint belongs to user
	var count int
	h.db.QueryRow(
		`SELECT COUNT(*) FROM endpoints WHERE id = $1 AND user_id = $2 AND is_active = true`,
		req.EndpointID, userID,
	).Scan(&count)
	if count == 0 {
		writeError(w, http.StatusNotFound, "endpoint not found or inactive")
		return
	}

	var delivery models.Delivery
	err := h.db.QueryRow(`
		INSERT INTO deliveries (endpoint_id, payload, event_type, status, max_attempts, next_retry_at)
		VALUES ($1, $2, $3, 'pending', $4, NOW())
		RETURNING id, endpoint_id, payload, event_type, status, attempt_count, max_attempts, created_at`,
		req.EndpointID, req.Payload, req.EventType, req.MaxAttempts,
	).Scan(&delivery.ID, &delivery.EndpointID, &delivery.Payload, &delivery.EventType,
		&delivery.Status, &delivery.AttemptCount, &delivery.MaxAttempts, &delivery.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue webhook")
		return
	}

	writeJSON(w, http.StatusAccepted, delivery)
}

// GET /api/webhooks
func (h *Handler) GetDeliveries(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.Atoi(r.Header.Get("X-User-ID"))
	status := r.URL.Query().Get("status")

	query := `
		SELECT d.id, d.endpoint_id, d.payload, d.event_type, d.status,
		       d.attempt_count, d.max_attempts, d.next_retry_at,
		       d.last_status_code, COALESCE(d.last_error,''), d.delivered_at, d.created_at
		FROM   deliveries d
		JOIN   endpoints e ON e.id = d.endpoint_id
		WHERE  e.user_id = $1`
	args := []interface{}{userID}

	if status != "" {
		query += " AND d.status = $2"
		args = append(args, status)
	}
	query += " ORDER BY d.created_at DESC LIMIT 50"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch deliveries")
		return
	}
	defer rows.Close()

	deliveries := []models.Delivery{}
	for rows.Next() {
		var d models.Delivery
		rows.Scan(&d.ID, &d.EndpointID, &d.Payload, &d.EventType, &d.Status,
			&d.AttemptCount, &d.MaxAttempts, &d.NextRetryAt,
			&d.LastStatusCode, &d.LastError, &d.DeliveredAt, &d.CreatedAt)
		deliveries = append(deliveries, d)
	}
	writeJSON(w, http.StatusOK, deliveries)
}

// GET /api/webhooks/{id}
func (h *Handler) GetDelivery(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.Atoi(r.Header.Get("X-User-ID"))
	id, _ := strconv.Atoi(mux.Vars(r)["id"])

	var d models.Delivery
	err := h.db.QueryRow(`
		SELECT d.id, d.endpoint_id, d.payload, d.event_type, d.status,
		       d.attempt_count, d.max_attempts, d.next_retry_at,
		       d.last_status_code, COALESCE(d.last_error,''), d.delivered_at, d.created_at
		FROM   deliveries d
		JOIN   endpoints e ON e.id = d.endpoint_id
		WHERE  d.id = $1 AND e.user_id = $2`, id, userID,
	).Scan(&d.ID, &d.EndpointID, &d.Payload, &d.EventType, &d.Status,
		&d.AttemptCount, &d.MaxAttempts, &d.NextRetryAt,
		&d.LastStatusCode, &d.LastError, &d.DeliveredAt, &d.CreatedAt)

	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "delivery not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// GET /api/webhooks/{id}/attempts
func (h *Handler) GetAttempts(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.Atoi(r.Header.Get("X-User-ID"))
	id, _ := strconv.Atoi(mux.Vars(r)["id"])

	// Verify ownership
	var count int
	h.db.QueryRow(`
		SELECT COUNT(*) FROM deliveries d
		JOIN endpoints e ON e.id = d.endpoint_id
		WHERE d.id = $1 AND e.user_id = $2`, id, userID,
	).Scan(&count)
	if count == 0 {
		writeError(w, http.StatusNotFound, "delivery not found")
		return
	}

	rows, err := h.db.Query(`
		SELECT id, delivery_id, status_code, COALESCE(error,''), duration_ms, attempted_at
		FROM delivery_attempts WHERE delivery_id = $1
		ORDER BY attempted_at ASC`, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch attempts")
		return
	}
	defer rows.Close()

	attempts := []models.DeliveryAttempt{}
	for rows.Next() {
		var a models.DeliveryAttempt
		rows.Scan(&a.ID, &a.DeliveryID, &a.StatusCode, &a.Error, &a.Duration, &a.AttemptedAt)
		attempts = append(attempts, a)
	}
	writeJSON(w, http.StatusOK, attempts)
}

// POST /api/webhooks/{id}/retry
func (h *Handler) RetryDelivery(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.Atoi(r.Header.Get("X-User-ID"))
	id, _ := strconv.Atoi(mux.Vars(r)["id"])

	res, err := h.db.Exec(`
		UPDATE deliveries d SET status = 'retrying', next_retry_at = NOW()
		FROM endpoints e
		WHERE d.endpoint_id = e.id AND d.id = $1 AND e.user_id = $2
		  AND d.status = 'failed'`, id, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to retry delivery")
		return
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "delivery not found or not in failed state")
		return
	}
	writeJSON(w, http.StatusOK, models.MessageResponse{Message: "delivery queued for retry"})
}
