package audit

import (
	"context"

	"github.com/weiawesome/wes-io-live/pkg/log"
)

// Audit actions for user-service.
const (
	ActionRegister       = "user.register"
	ActionLogin          = "user.login"
	ActionLoginFailed    = "user.login_failed"
	ActionLogout         = "user.logout"
	ActionRefreshToken   = "user.refresh_token"
	ActionGetProfile     = "user.get_profile"
	ActionUpdateProfile  = "user.update_profile"
	ActionChangePassword = "user.change_password"
	ActionDeleteAccount  = "user.delete_account"
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
