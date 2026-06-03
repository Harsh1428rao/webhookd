package delivery

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	workerInterval = 5 * time.Second
	httpTimeout    = 10 * time.Second
)

// BackoffDuration returns exponential backoff duration for a given attempt number.
// Attempt 1 → 1s, 2 → 2s, 3 → 4s, 4 → 8s, 5 → 16s (capped at 5min)
func BackoffDuration(attempt int) time.Duration {
	seconds := 1 << uint(attempt-1) // 2^(attempt-1)
	if seconds > 300 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

// sign generates an HMAC-SHA256 signature of the payload using the endpoint secret.
// This lets the receiving server verify the webhook came from webhookd.
func sign(secret, payload string) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// attempt performs a single HTTP POST to the endpoint URL
func attempt(url, secret, payload, eventType string) (statusCode int, errMsg string, duration int) {
	start := time.Now()
	client := &http.Client{Timeout: httpTimeout}

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(payload))
	if err != nil {
		return 0, err.Error(), int(time.Since(start).Milliseconds())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", eventType)
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	sig := sign(secret, payload)
	if sig != "" {
		req.Header.Set("X-Webhook-Signature", "sha256="+sig)
	}

	resp, err := client.Do(req)
	duration = int(time.Since(start).Milliseconds())
	if err != nil {
		return 0, err.Error(), duration
	}
	defer resp.Body.Close()
	return resp.StatusCode, "", duration
}

// ProcessPending is the core worker loop. It runs every workerInterval,
// picks up pending/retrying deliveries whose next_retry_at is due,
// and attempts delivery with exponential backoff.
func ProcessPending(db *sql.DB) {
	log.Println("[worker] Delivery worker started")
	ticker := time.NewTicker(workerInterval)
	defer ticker.Stop()

	for range ticker.C {
		processBatch(db)
	}
}

func processBatch(db *sql.DB) {
	rows, err := db.Query(`
		SELECT d.id, d.endpoint_id, d.payload, d.event_type,
		       d.attempt_count, d.max_attempts,
		       e.url, COALESCE(e.secret, '')
		FROM   deliveries d
		JOIN   endpoints e ON e.id = d.endpoint_id
		WHERE  d.status IN ('pending', 'retrying')
		  AND  (d.next_retry_at IS NULL OR d.next_retry_at <= NOW())
		ORDER  BY d.created_at ASC
		LIMIT  20
	`)
	if err != nil {
		log.Printf("[worker] query error: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			deliveryID, endpointID, attemptCount, maxAttempts int
			payload, eventType, url, secret                   string
		)
		if err := rows.Scan(&deliveryID, &endpointID, &payload, &eventType,
			&attemptCount, &maxAttempts, &url, &secret); err != nil {
			log.Printf("[worker] scan error: %v", err)
			continue
		}

		go deliver(db, deliveryID, url, secret, payload, eventType, attemptCount, maxAttempts)
	}
}

func deliver(db *sql.DB, deliveryID int, url, secret, payload, eventType string, attemptCount, maxAttempts int) {
	newAttempt := attemptCount + 1
	statusCode, errMsg, duration := attempt(url, secret, payload, eventType)

	// Log the attempt
	db.Exec(`
		INSERT INTO delivery_attempts (delivery_id, status_code, error, duration_ms)
		VALUES ($1, $2, $3, $4)`,
		deliveryID, statusCode, errMsg, duration,
	)

	success := statusCode >= 200 && statusCode < 300

	if success {
		now := time.Now()
		db.Exec(`
			UPDATE deliveries
			SET status = 'delivered', attempt_count = $1,
			    last_status_code = $2, delivered_at = $3, next_retry_at = NULL
			WHERE id = $4`,
			newAttempt, statusCode, now, deliveryID,
		)
		log.Printf("[worker] delivery %d → delivered (status %d, attempt %d)", deliveryID, statusCode, newAttempt)
		return
	}

	// Failed — decide whether to retry
	if newAttempt >= maxAttempts {
		db.Exec(`
			UPDATE deliveries
			SET status = 'failed', attempt_count = $1,
			    last_status_code = $2, last_error = $3, next_retry_at = NULL
			WHERE id = $4`,
			newAttempt, statusCode, errMsg, deliveryID,
		)
		log.Printf("[worker] delivery %d → failed after %d attempts", deliveryID, newAttempt)
		return
	}

	// Schedule next retry with exponential backoff
	nextRetry := time.Now().Add(BackoffDuration(newAttempt))
	db.Exec(`
		UPDATE deliveries
		SET status = 'retrying', attempt_count = $1,
		    last_status_code = $2, last_error = $3, next_retry_at = $4
		WHERE id = $5`,
		newAttempt, statusCode, errMsg, nextRetry, deliveryID,
	)
	log.Printf("[worker] delivery %d → retrying in %s (attempt %d/%d)",
		deliveryID, BackoffDuration(newAttempt), newAttempt, maxAttempts)
}
