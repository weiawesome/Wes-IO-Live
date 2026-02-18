package log

const (
	// Request
	FieldRequestID = "request_id"
	FieldMethod    = "method"
	FieldPath      = "path"
	FieldStatus    = "status"
	FieldLatency   = "latency_ms"
	FieldClientIP  = "client_ip"

	// Actor (matches pkg/middleware/auth.go keys)
	FieldUserID   = "user_id"
	FieldUsername  = "username"

	// Service
	FieldService = "service"

	// gRPC
	FieldGRPCMethod = "grpc_method"
	FieldGRPCCode   = "grpc_code"

	// Log type (for audit log)
	FieldLogType = "log_type"
	LogTypeAudit = "audit"
)
