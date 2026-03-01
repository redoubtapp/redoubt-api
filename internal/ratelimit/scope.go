package ratelimit

// Scope constants for rate limiting.
const (
	ScopeRegister       = "register"
	ScopeLogin          = "login"
	ScopeForgotPassword = "forgot_password"
	ScopeVerifyEmail    = "verify_email"
	ScopeGeneral        = "general"
	ScopeFileUpload     = "file_upload"

	// Message rate limiting scopes
	ScopeMessageSend = "message_send"
	ScopeMessageEdit = "message_edit"
	ScopeReactionAdd = "reaction_add"
)
