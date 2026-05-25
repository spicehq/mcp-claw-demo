// Expense reports REST API — stand-in for an internal HR/finance service.
//
// In-memory state, single instance. The point of the demo is to show that
// Spice's MCP gateway can front an arbitrary HTTP service (reads AND writes),
// not to be a real expense system. Restart = blank slate.
//
// Endpoints:
//   GET    /healthz
//   GET    /reports                  → list all reports
//   POST   /reports                  → file a new report
//   POST   /reports/{id}/approve     → approve a filed report
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Report struct {
	ID          string    `json:"id"`
	Submitter   string    `json:"submitter"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // "filed" | "approved"
	FiledAt     time.Time `json:"filed_at"`
	ApprovedAt  *time.Time `json:"approved_at,omitempty"`
	ApprovedBy  string    `json:"approved_by,omitempty"`
}

type store struct {
	mu      sync.Mutex
	next    int
	reports map[string]*Report
}

func newStore() *store {
	return &store{
		next:    1001,
		reports: map[string]*Report{},
	}
}

func (s *store) list() []*Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Report, 0, len(s.reports))
	for _, r := range s.reports {
		out = append(out, r)
	}
	return out
}

func (s *store) file(submitter, currency, category, description string, amount float64) *Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("exp_%d", s.next)
	s.next++
	r := &Report{
		ID:          id,
		Submitter:   submitter,
		Amount:      amount,
		Currency:    currency,
		Category:    category,
		Description: description,
		Status:      "filed",
		FiledAt:     time.Now().UTC(),
	}
	s.reports[id] = r
	return r
}

func (s *store) approve(id, approver string) (*Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reports[id]
	if !ok {
		return nil, fmt.Errorf("report %q not found", id)
	}
	if r.Status == "approved" {
		return nil, fmt.Errorf("report %q is already approved", id)
	}
	now := time.Now().UTC()
	r.Status = "approved"
	r.ApprovedAt = &now
	r.ApprovedBy = approver
	return r, nil
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func main() {
	addr := os.Getenv("EXPENSE_API_ADDR")
	if addr == "" {
		addr = ":8093"
	}
	s := newStore()

	// Seed two reports so list calls return something interesting on a fresh boot.
	s.file("alice@acme.example", "USD", "travel", "Flight to onsite review", 412.55)
	s.file("bob@acme.example", "USD", "software", "JetBrains All Products Pack — annual", 779.00)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /reports", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.list())
	})

	mux.HandleFunc("POST /reports", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Submitter   string  `json:"submitter"`
			Amount      float64 `json:"amount"`
			Currency    string  `json:"currency"`
			Category    string  `json:"category"`
			Description string  `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
		if in.Submitter == "" || in.Amount <= 0 || in.Currency == "" {
			writeErr(w, http.StatusBadRequest, "submitter, amount (>0), and currency are required")
			return
		}
		if in.Category == "" {
			in.Category = "other"
		}
		report := s.file(in.Submitter, in.Currency, in.Category, in.Description, in.Amount)
		writeJSON(w, http.StatusCreated, report)
	})

	mux.HandleFunc("POST /reports/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var in struct {
			Approver string `json:"approver"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil && !strings.Contains(err.Error(), "EOF") {
			writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
		if in.Approver == "" {
			writeErr(w, http.StatusBadRequest, "approver is required")
			return
		}
		report, err := s.approve(id, in.Approver)
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, report)
	})

	log.Printf("expense-api listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
