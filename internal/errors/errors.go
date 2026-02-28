package errors

import "errors"

// Sentinel errors for internal use.
// These are translated to RFC 9457 Problem Details responses at the HTTP layer.

// Authentication errors
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailNotVerified   = errors.New("email not verified")
	ErrAccountLocked      = errors.New("account locked")
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("token expired")
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionRevoked     = errors.New("session revoked")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrInsufficientRole   = errors.New("insufficient role")
)

// User errors
var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrUserDeleted       = errors.New("user deleted")
	ErrAccountDisabled   = errors.New("account disabled")
)

// Space errors
var (
	ErrSpaceNotFound      = errors.New("space not found")
	ErrSpaceAlreadyExists = errors.New("space already exists")
	ErrSpaceDeleted       = errors.New("space deleted")
)

// Channel errors
var (
	ErrChannelNotFound      = errors.New("channel not found")
	ErrChannelAlreadyExists = errors.New("channel already exists")
	ErrChannelDeleted       = errors.New("channel deleted")
)

// Membership errors
var (
	ErrMembershipNotFound = errors.New("membership not found")
	ErrAlreadyMember      = errors.New("already a member")
	ErrNotMember          = errors.New("not a member")
	ErrCannotKickOwner    = errors.New("cannot kick owner")
	ErrCannotChangeOwner  = errors.New("cannot change owner role")
)

// Invite errors
var (
	ErrInviteNotFound  = errors.New("invite not found")
	ErrInviteExpired   = errors.New("invite expired")
	ErrInviteRevoked   = errors.New("invite revoked")
	ErrInviteExhausted = errors.New("invite max uses reached")
	ErrInvalidInvite   = errors.New("invalid invite code")
)

// Validation errors
var (
	ErrValidation      = errors.New("validation error")
	ErrInvalidInput    = errors.New("invalid input")
	ErrPasswordTooWeak = errors.New("password too weak")
	ErrInvalidEmail    = errors.New("invalid email format")
	ErrInvalidUsername = errors.New("invalid username format")
)

// Rate limiting errors
var (
	ErrRateLimited = errors.New("rate limited")
)

// Storage errors
var (
	ErrFileTooLarge       = errors.New("file too large")
	ErrInvalidFileType    = errors.New("invalid file type")
	ErrStorageError       = errors.New("storage error")
	ErrAvatarNotFound     = errors.New("avatar not found")
	ErrAttachmentNotFound = errors.New("attachment not found")
	ErrTooManyAttachments = errors.New("too many attachments")
)

// Voice/LiveKit errors
var (
	ErrNotVoiceChannel      = errors.New("not a voice channel")
	ErrAlreadyInVoice       = errors.New("already in a voice channel")
	ErrVoiceChannelFull     = errors.New("voice channel is full")
	ErrNotInVoiceChannel    = errors.New("not in voice channel")
	ErrLiveKitUnavailable   = errors.New("livekit service unavailable")
	ErrCannotMuteSelf       = errors.New("cannot server-mute yourself")
	ErrCannotMuteHigherRole = errors.New("cannot mute user with higher role")
)

// Message errors
var (
	ErrMessageNotFound    = errors.New("message not found")
	ErrMessageDeleted     = errors.New("message deleted")
	ErrMessageTooLong     = errors.New("message too long")
	ErrMessageEmpty       = errors.New("message empty")
	ErrCodeBlockTooLong   = errors.New("code block too long")
	ErrEditWindowExpired  = errors.New("edit window expired")
	ErrCannotEditOthers   = errors.New("cannot edit others messages")
	ErrCannotDeleteOthers = errors.New("cannot delete others messages")
	ErrInvalidEmoji       = errors.New("invalid emoji")
	ErrThreadDepthExceeded = errors.New("thread depth exceeded")
)

// General errors
var (
	ErrInternal = errors.New("internal error")
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)
