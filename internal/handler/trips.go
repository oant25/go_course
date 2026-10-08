package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"

	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/service"
	"github.com/oant25/go_course/internal/trip"
)

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	var req api.TripData
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, problemInvalidRequest, "request body is not a valid trip: "+err.Error())
		return
	}
	if req.UserId == uuid.Nil || req.DriverId == uuid.Nil {
		writeProblem(w, r, problemInvalidRequest, "user_id and driver_id must be non-empty UUIDs")
		return
	}

	created, err := h.trips.Create(r.Context(), service.CreateInput{
		UserID:   req.UserId,
		DriverID: req.DriverId,
		Start:    trip.Point{Latitude: req.StartPoint.Latitude, Longitude: req.StartPoint.Longitude},
		End:      trip.Point{Latitude: req.EndPoint.Latitude, Longitude: req.EndPoint.Longitude},
		Price:    req.Price,
	})
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+created.ID.String())
	writeJSON(w, http.StatusCreated, toAPITrip(created))
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	t, err := h.trips.Get(r.Context(), tripID)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPITrip(t))
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	t, err := h.trips.Finish(r.Context(), tripID)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPITrip(t))
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, trip.ErrNotFound):
		writeProblem(w, r, problemTripNotFound, "Trip was not found")
	case errors.Is(err, trip.ErrAlreadyCompleted):
		writeProblem(w, r, problemTripCompleted, "Operation is not allowed for a completed trip")
	case errors.Is(err, trip.ErrDriverBusy):
		writeProblem(w, r, problemDriverBusy, "Driver already has an active trip")
	default:
		slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		writeProblem(w, r, problemInternal, "Internal server error")
	}
}

func toAPITrip(t trip.Trip) api.Trip {
	finishedAt := nullable.NewNullNullable[time.Time]()
	if t.FinishedAt != nil {
		finishedAt = nullable.NewNullableWithValue(t.FinishedAt.UTC())
	}

	return api.Trip{
		Id:       t.ID,
		UserId:   t.UserID,
		DriverId: t.DriverID,
		StartPoint: api.Coordinates{
			Latitude:  t.Start.Latitude,
			Longitude: t.Start.Longitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  t.End.Latitude,
			Longitude: t.End.Longitude,
		},
		Price:          t.Price,
		Status:         api.TripStatus(t.Status),
		StartedAt:      t.StartedAt.UTC(),
		FinishedAt:     finishedAt,
		LastPositionAt: nullable.NewNullNullable[time.Time](),
	}
}
