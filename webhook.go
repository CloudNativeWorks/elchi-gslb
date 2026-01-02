package elchi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SyncStatus tracks the last sync operation status.
type SyncStatus struct {
	mu             sync.RWMutex
	lastSyncTime   time.Time
	lastSyncStatus string // "success", "failed", "initial"
	lastError      string
}

// Update updates the sync status.
func (s *SyncStatus) Update(status string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSyncTime = time.Now()
	s.lastSyncStatus = status
	if err != nil {
		s.lastError = err.Error()
	} else {
		s.lastError = ""
	}
}

// Get returns the current sync status.
func (s *SyncStatus) Get() (time.Time, string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSyncTime, s.lastSyncStatus, s.lastError
}

// WebhookServer manages the HTTP server for webhook endpoints.
type WebhookServer struct {
	elchi  *Elchi
	server *http.Server
	mux    *http.ServeMux
}

// NewWebhookServer creates a new webhook server.
func NewWebhookServer(elchi *Elchi, addr string) *WebhookServer {
	mux := http.NewServeMux()

	ws := &WebhookServer{
		elchi: elchi,
		mux:   mux,
		server: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
	}

	// Register endpoints
	mux.HandleFunc("/notify", ws.authMiddleware(ws.handleNotify))
	mux.HandleFunc("/health", ws.handleHealth)
	mux.HandleFunc("/records", ws.authMiddleware(ws.handleRecords))

	return ws
}

// Start starts the webhook server in a goroutine.
func (ws *WebhookServer) Start() error {
	go func() {
		log.Infof("Starting webhook server on %s", ws.server.Addr)
		if err := ws.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("Webhook server error: %v", err)
		}
	}()
	return nil
}

// Stop gracefully stops the webhook server.
func (ws *WebhookServer) Stop(ctx context.Context) error {
	return ws.server.Shutdown(ctx)
}

// to prevent timing attacks.
func (ws *WebhookServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secret := r.Header.Get("X-Elchi-Secret")

		// Use constant-time comparison to prevent timing attacks
		// subtle.ConstantTimeCompare returns 1 if equal, 0 otherwise
		if subtle.ConstantTimeCompare([]byte(secret), []byte(ws.elchi.Secret)) != 1 {
			// Track unauthorized webhook request
			endpoint := strings.TrimPrefix(r.URL.Path, "/")
			webhookRequests.WithLabelValues(endpoint, "unauthorized").Inc()
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// NotifyRequest represents the POST /notify request body.
type NotifyRequest struct {
	Records []DNSRecord    `json:"records,omitempty"`
	Deletes []DeleteRecord `json:"deletes,omitempty"`
}

// NotifyResponse represents the POST /notify response.
type NotifyResponse struct {
	Status  string `json:"status"`
	Updated int    `json:"updated"`
	Deleted int    `json:"deleted"`
}

// handleNotify handles POST /notify endpoint for instant updates.
func (ws *WebhookServer) handleNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req NotifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webhookRequests.WithLabelValues("notify", "error").Inc()
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	updated := 0
	deleted := 0

	// Process updates
	if len(req.Records) > 0 {
		if err := ws.elchi.cache.Update(req.Records, ws.elchi.TTL); err != nil {
			log.Errorf("Failed to update records: %v", err)
			webhookRequests.WithLabelValues("notify", "error").Inc()
			http.Error(w, "Failed to update records", http.StatusInternalServerError)
			return
		}
		updated = len(req.Records)
		log.Infof("Updated %d records via webhook", updated)
	}

	// Process deletes
	if len(req.Deletes) > 0 {
		if err := ws.elchi.cache.Delete(req.Deletes); err != nil {
			log.Errorf("Failed to delete records: %v", err)
			webhookRequests.WithLabelValues("notify", "error").Inc()
			http.Error(w, "Failed to delete records", http.StatusInternalServerError)
			return
		}
		deleted = len(req.Deletes)
		log.Infof("Deleted %d records via webhook", deleted)
	}

	// Track successful webhook request
	webhookRequests.WithLabelValues("notify", "success").Inc()

	// Send response
	resp := NotifyResponse{
		Status:  "ok",
		Updated: updated,
		Deleted: deleted,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Errorf("Failed to encode notify response: %v", err)
	}
}

// HealthResponse represents the GET /health response.
type HealthResponse struct {
	Status         string `json:"status"`
	Zone           string `json:"zone"`
	RecordsCount   int    `json:"records_count"`
	VersionHash    string `json:"version_hash"`
	LastSync       string `json:"last_sync"`
	LastSyncStatus string `json:"last_sync_status"`
	Error          string `json:"error,omitempty"`
}

// handleHealth handles GET /health endpoint.
func (ws *WebhookServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lastSync, syncStatus, lastError := ws.elchi.syncStatus.Get()

	resp := HealthResponse{
		Zone:           ws.elchi.Zone,
		RecordsCount:   ws.elchi.cache.Count(),
		VersionHash:    ws.elchi.cache.GetVersionHash(),
		LastSync:       lastSync.Format(time.RFC3339),
		LastSyncStatus: syncStatus,
	}

	// Determine overall health status
	if syncStatus == "failed" && time.Since(lastSync) > 2*ws.elchi.SyncInterval {
		// Degraded if last sync failed and it's been more than 2 sync intervals
		resp.Status = "degraded"
		resp.Error = lastError
		webhookRequests.WithLabelValues("health", "success").Inc()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Errorf("Failed to encode health response: %v", err)
		}
		return
	}

	resp.Status = "healthy"
	webhookRequests.WithLabelValues("health", "success").Inc()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Errorf("Failed to encode health response: %v", err)
	}
}

// RecordsResponse represents the GET /records response.
type RecordsResponse struct {
	Zone        string      `json:"zone"`
	VersionHash string      `json:"version_hash"`
	Count       int         `json:"count"`
	Records     []DNSRecord `json:"records"`
}

// handleRecords handles GET /records endpoint.
func (ws *WebhookServer) handleRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get query parameters for filtering
	nameFilter := r.URL.Query().Get("name")
	typeFilter := strings.ToUpper(r.URL.Query().Get("type"))

	// Get all records from cache
	allRecords := ws.elchi.cache.GetAllRecords()

	// Apply filters
	var filteredRecords []DNSRecord
	for _, record := range allRecords {
		// Filter by name if specified
		if nameFilter != "" && !strings.Contains(record.Name, nameFilter) {
			continue
		}

		// Filter by type if specified
		if typeFilter != "" && record.Type != typeFilter {
			continue
		}

		filteredRecords = append(filteredRecords, record)
	}

	// Track successful webhook request
	webhookRequests.WithLabelValues("records", "success").Inc()

	resp := RecordsResponse{
		Zone:        ws.elchi.Zone,
		VersionHash: ws.elchi.cache.GetVersionHash(),
		Count:       len(filteredRecords),
		Records:     filteredRecords,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Errorf("Failed to encode records response: %v", err)
	}
}
