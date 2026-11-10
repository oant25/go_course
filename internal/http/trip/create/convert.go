package create

import (
	"github.com/oant25/go_course/internal/domain/trip"
	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/model"
)

func toInput(body api.CreateTripJSONRequestBody) model.CreateTripInput {
	return model.CreateTripInput{
		UserID: body.UserId, DriverID: body.DriverId, Price: body.Price,
		Start: trip.Point{Latitude: body.StartPoint.Latitude, Longitude: body.StartPoint.Longitude},
		End:   trip.Point{Latitude: body.EndPoint.Latitude, Longitude: body.EndPoint.Longitude},
	}
}
