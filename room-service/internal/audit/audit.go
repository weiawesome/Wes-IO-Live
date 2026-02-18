package audit

import (
	"context"

	"github.com/weiawesome/wes-io-live/pkg/log"
)

// Audit actions for room-service.
const (
	ActionCreateRoom = "room.create"
	ActionCloseRoom  = "room.close"
)

// Field constants for audit entries.
const (
	FieldAction = "action"
	FieldRoomID = "room_id"
	FieldDetail = "detail"
)

// Log emits a structured audit log entry via the context logger.
func Log(ctx context.Context, action string, userID string, msg string) {
	l := log.Ctx(ctx)
	l.Info().
		Str(log.FieldLogType, log.LogTypeAudit).
		Str(FieldAction, action).
		Str(log.FieldUserID, userID).
		Msg(msg)
}

// LogWithDetail emits an audit log with extra detail field.
func LogWithDetail(ctx context.Context, action string, userID string, detail string, msg string) {
	l := log.Ctx(ctx)
	l.Info().
		Str(log.FieldLogType, log.LogTypeAudit).
		Str(FieldAction, action).
		Str(log.FieldUserID, userID).
		Str(FieldDetail, detail).
		Msg(msg)
}
