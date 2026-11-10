package identifier

import "github.com/google/uuid"

type UUID struct{}

func (UUID) New() uuid.UUID { return uuid.New() }
