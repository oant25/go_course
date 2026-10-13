package handler

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"

	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/service"
	"github.com/oant25/go_course/internal/trip"
)

const tripsPath = "/api/v1/trips"

func (h *Handler) CreateTrip(ctx context.Context, req api.CreateTripRequestObject) (api.CreateTripResponseObject, error) {
	body := req.Body
	if body == nil {
		return api.CreateTrip400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: api.BadRequestApplicationProblemPlusJSONResponse(
				newProblem(problemInvalidRequest, tripsPath, "request body is required")),
		}, nil
	}
	if body.UserId == uuid.Nil || body.DriverId == uuid.Nil {
		return api.CreateTrip400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: api.BadRequestApplicationProblemPlusJSONResponse(
				newProblem(problemInvalidRequest, tripsPath, "user_id and driver_id must be non-empty UUIDs")),
		}, nil
	}

	created, err := h.trips.Create(ctx, service.CreateInput{
		UserID:   body.UserId,
		DriverID: body.DriverId,
		Start:    trip.Point{Latitude: body.StartPoint.Latitude, Longitude: body.StartPoint.Longitude},
		End:      trip.Point{Latitude: body.EndPoint.Latitude, Longitude: body.EndPoint.Longitude},
		Price:    body.Price,
	})
	switch {
	case errors.Is(err, trip.ErrDriverBusy):
		return api.CreateTrip409ApplicationProblemPlusJSONResponse{
			CreateTripConflictApplicationProblemPlusJSONResponse: api.CreateTripConflictApplicationProblemPlusJSONResponse(
				newProblem(problemDriverBusy, tripsPath, "Driver already has an active trip")),
		}, nil
	case err != nil:
		return nil, err
	}

	location := tripPath(created.ID)
	return api.CreateTrip201JSONResponse{
		Body:    toAPITrip(created),
		Headers: api.CreateTrip201ResponseHeaders{Location: &location},
	}, nil
}

func (h *Handler) GetTrip(ctx context.Context, req api.GetTripRequestObject) (api.GetTripResponseObject, error) {
	t, err := h.trips.Get(ctx, req.TripId)
	switch {
	case errors.Is(err, trip.ErrNotFound):
		return api.GetTrip404ApplicationProblemPlusJSONResponse{
			TripNotFoundApplicationProblemPlusJSONResponse: api.TripNotFoundApplicationProblemPlusJSONResponse(
				newProblem(problemTripNotFound, tripPath(req.TripId), "Trip was not found")),
		}, nil
	case err != nil:
		return nil, err
	}
	return api.GetTrip200JSONResponse(toAPITrip(t)), nil
}

func (h *Handler) FinishTrip(ctx context.Context, req api.FinishTripRequestObject) (api.FinishTripResponseObject, error) {
	instance := tripPath(req.TripId) + "/finish"

	t, err := h.trips.Finish(ctx, req.TripId)
	switch {
	case errors.Is(err, trip.ErrNotFound):
		return api.FinishTrip404ApplicationProblemPlusJSONResponse{
			TripNotFoundApplicationProblemPlusJSONResponse: api.TripNotFoundApplicationProblemPlusJSONResponse(
				newProblem(problemTripNotFound, instance, "Trip was not found")),
		}, nil
	case errors.Is(err, trip.ErrAlreadyCompleted):
		return api.FinishTrip409ApplicationProblemPlusJSONResponse{
			TripCompletedApplicationProblemPlusJSONResponse: api.TripCompletedApplicationProblemPlusJSONResponse(
				newProblem(problemTripCompleted, instance, "Operation is not allowed for a completed trip")),
		}, nil
	case err != nil:
		return nil, err
	}
	return api.FinishTrip200JSONResponse(toAPITrip(t)), nil
}

func tripPath(id uuid.UUID) string {
	return tripsPath + "/" + id.String()
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
