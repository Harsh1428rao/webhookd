package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Harsh1428rao/webhookd/internal/db"
	"github.com/Harsh1428rao/webhookd/internal/delivery"
	"github.com/Harsh1428rao/webhookd/internal/handlers"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/rs/cors"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	database, err := db.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Start background delivery worker (goroutine)
	go delivery.ProcessPending(database)

	r := mux.NewRouter()
	h := handlers.New(database)

	// Auth
	r.HandleFunc("/api/auth/register", h.Register).Methods("POST")
	

	// Endpoints
	r.HandleFunc("/api/endpoints", h.AuthMiddleware(h.CreateEndpoint)).Methods("POST")
	r.HandleFunc("/api/endpoints", h.AuthMiddleware(h.GetEndpoints)).Methods("GET")
	r.HandleFunc("/api/endpoints/{id}", h.AuthMiddleware(h.DeleteEndpoint)).Methods("DELETE")

	// Webhooks
	r.HandleFunc("/api/webhooks/send", h.AuthMiddleware(h.SendWebhook)).Methods("POST")
	r.HandleFunc("/api/webhooks", h.AuthMiddleware(h.GetDeliveries)).Methods("GET")
	r.HandleFunc("/api/webhooks/{id}", h.AuthMiddleware(h.GetDelivery)).Methods("GET")
	r.HandleFunc("/api/webhooks/{id}/attempts", h.AuthMiddleware(h.GetAttempts)).Methods("GET")
	r.HandleFunc("/api/webhooks/{id}/retry", h.AuthMiddleware(h.RetryDelivery)).Methods("POST")

	// Health
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok"}`)
	}).Methods("GET")

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("webhookd running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, c.Handler(r)))
}
