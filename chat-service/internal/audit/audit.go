package audit

import (
	"context"

	"github.com/weiawesome/wes-io-live/pkg/log"
)

// Audit actions for chat-service.
const (
	ActionAuth        = "chat.auth"
	ActionAuthFailed  = "chat.auth_failed"
	ActionJoinRoom    = "chat.join_room"
	ActionLeaveRoom   = "chat.leave_room"
	ActionSendMessage = "chat.send_message"
	ActionDisconnect  = "chat.disconnect"
)

// Field constants for audit entries.
const (
	FieldAction   = "action"
	FieldTargetID = "target_id"
	FieldDetail   = "detail"
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
