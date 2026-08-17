package rooms

import (
	"errors"
	"fmt"
)

var (
	ErrWorkspaceNotFound   = errors.New("workspace not found")
	ErrRoomNotFound        = errors.New("room not found")
	ErrMessageNotFound     = errors.New("room message not found")
	ErrMentionNotFound     = errors.New("room mention not found")
	ErrParticipantNotFound = errors.New("one or more mentioned actors are unavailable")
	ErrSlugTaken           = errors.New("a room with this slug already exists in the workspace")
	ErrForbidden           = errors.New("the current identity cannot act as this message author")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with another room request")
	ErrCommandIncomplete   = errors.New("an earlier room command has not completed")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}
