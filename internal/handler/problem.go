package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	api "github.com/oant25/go_course/internal/generated"
)

type problem struct {
	status int
	code   string
	title  string
	slug   string
}

const problemTypeBase = "https://tripgo.example/problems/"

var (
	problemInvalidRequest = problem{http.StatusBadRequest, "invalid_request", "Invalid request", "invalid-request"}
	problemTripNotFound   = problem{http.StatusNotFound, "trip_not_found", "Trip not found", "trip-not-found"}
	problemTripCompleted  = problem{http.StatusConflict, "trip_completed", "Trip completed", "trip-completed"}
	problemDriverBusy     = problem{http.StatusConflict, "driver_busy", "Driver busy", "driver-busy"}
	problemInternal       = problem{http.StatusInternalServerError, "internal_error", "Internal Server Error", "internal-error"}
)

func writeProblem(w http.ResponseWriter, r *http.Request, p problem, detail string) {
	instance := r.URL.Path
	body := api.Problem{
		Type:     problemTypeBase + p.slug,
		Title:    p.title,
		Status:   int32(p.status),
		Code:     p.code,
		Detail:   &detail,
		Instance: &instance,
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write problem response", "error", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write response", "error", err)
	}
}
