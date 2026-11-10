package trip

import (
	"math"

	"github.com/google/uuid"
)

type Details struct {
	UserID   uuid.UUID
	DriverID uuid.UUID
	Start    Point
	End      Point
	Price    int64
}

func (d Details) Validate() error {
	if d.UserID == uuid.Nil {
		return &ValidationError{Field: "user_id", Reason: nonEmptyUUIDReason}
	}
	if d.DriverID == uuid.Nil {
		return &ValidationError{Field: "driver_id", Reason: nonEmptyUUIDReason}
	}
	if err := d.Start.validate("start_point"); err != nil {
		return err
	}
	if err := d.End.validate("end_point"); err != nil {
		return err
	}
	if d.Price < 0 {
		return &ValidationError{Field: "price", Reason: "must be non-negative"}
	}
	return nil
}

func (p Point) validate(field string) error {
	if !inRange(p.Latitude, -90, 90) {
		return &ValidationError{Field: field + ".latitude", Reason: "must be finite and between -90 and 90"}
	}
	if !inRange(p.Longitude, -180, 180) {
		return &ValidationError{Field: field + ".longitude", Reason: "must be finite and between -180 and 180"}
	}
	return nil
}

func inRange(value, lower, upper float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= lower && value <= upper
}
