package trip

import "errors"

var (
	ErrTripNotFound  = errors.New("trip not found")
	ErrTripCompleted = errors.New("trip already completed")
	ErrDriverBusy    = errors.New("driver already has an active trip")
	ErrInvalidState  = errors.New("invalid stored trip state")
)

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Reason
}

type StateError struct {
	cause error
}

func (e *StateError) Error() string {
	return ErrInvalidState.Error() + ": " + e.cause.Error()
}

func (e *StateError) Unwrap() error {
	return e.cause
}

func (e *StateError) Is(target error) bool {
	return target == ErrInvalidState
}
