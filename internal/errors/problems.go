package errors

import (
	"fmt"
	"net/http"

	"alpineworks.io/rfc9457"
)

// ProblemBaseURI is the base URI for all problem types.
const ProblemBaseURI = "https://redoubt.app/problems/"

// FieldError represents a single validation error for a specific field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// requestIDKey is the context key for request ID.
type requestIDKey struct{}

// GetRequestID retrieves the request ID from the request context.
func GetRequestID(r *http.Request) string {
	if id, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// SetRequestID returns a context key for storing request ID.
func RequestIDKey() any {
	return requestIDKey{}
}

// ValidationError returns a 400 response with field-level validation errors.
func ValidationError(w http.ResponseWriter, r *http.Request, fieldErrors []FieldError) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"validation-error"),
		rfc9457.WithTitle("Validation Failed"),
		rfc9457.WithStatus(http.StatusBadRequest),
		rfc9457.WithDetail("The request body contains invalid fields"),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
			rfc9457.NewExtension("errors", fieldErrors),
		),
	).ServeHTTP(w, r)
}

// InvalidCredentials returns a 401 response for bad login attempts.
func InvalidCredentials(w http.ResponseWriter, r *http.Request) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"invalid-credentials"),
		rfc9457.WithTitle("Invalid Credentials"),
		rfc9457.WithStatus(http.StatusUnauthorized),
		rfc9457.WithDetail("The email or password provided is incorrect"),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// Unauthorized returns a 401 response for missing or invalid authentication.
func Unauthorized(w http.ResponseWriter, r *http.Request) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"unauthorized"),
		rfc9457.WithTitle("Unauthorized"),
		rfc9457.WithStatus(http.StatusUnauthorized),
		rfc9457.WithDetail("Authentication is required to access this resource"),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// EmailNotVerified returns a 403 response when email verification is required.
func EmailNotVerified(w http.ResponseWriter, r *http.Request) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"email-not-verified"),
		rfc9457.WithTitle("Email Not Verified"),
		rfc9457.WithStatus(http.StatusForbidden),
		rfc9457.WithDetail("Please verify your email address before accessing this resource"),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// Forbidden returns a 403 response for insufficient permissions.
func Forbidden(w http.ResponseWriter, r *http.Request) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"forbidden"),
		rfc9457.WithTitle("Forbidden"),
		rfc9457.WithStatus(http.StatusForbidden),
		rfc9457.WithDetail("You do not have permission to access this resource"),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// NotFound returns a 404 response for missing resources.
func NotFound(w http.ResponseWriter, r *http.Request, resource string) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"not-found"),
		rfc9457.WithTitle("Not Found"),
		rfc9457.WithStatus(http.StatusNotFound),
		rfc9457.WithDetail(fmt.Sprintf("%s not found", resource)),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// Conflict returns a 409 response for conflicting operations.
func Conflict(w http.ResponseWriter, r *http.Request, detail string) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"conflict"),
		rfc9457.WithTitle("Conflict"),
		rfc9457.WithStatus(http.StatusConflict),
		rfc9457.WithDetail(detail),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// RateLimited returns a 429 response with retry information.
func RateLimited(w http.ResponseWriter, r *http.Request, retryAfter int) {
	w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"rate-limited"),
		rfc9457.WithTitle("Too Many Requests"),
		rfc9457.WithStatus(http.StatusTooManyRequests),
		rfc9457.WithDetail(fmt.Sprintf("Rate limit exceeded. Try again in %d seconds.", retryAfter)),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// AccountLocked returns a 423 response for locked accounts.
func AccountLocked(w http.ResponseWriter, r *http.Request, remainingMinutes int) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"account-locked"),
		rfc9457.WithTitle("Account Locked"),
		rfc9457.WithStatus(http.StatusLocked),
		rfc9457.WithDetail(fmt.Sprintf("Account is temporarily locked due to too many failed login attempts. Try again in %d minutes.", remainingMinutes)),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// InternalError returns a 500 response for server errors.
// The actual error is logged but not exposed to the client.
func InternalError(w http.ResponseWriter, r *http.Request) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"internal-error"),
		rfc9457.WithTitle("Internal Server Error"),
		rfc9457.WithStatus(http.StatusInternalServerError),
		rfc9457.WithDetail("An unexpected error occurred. Please try again later."),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// BadRequest returns a 400 response for malformed requests.
func BadRequest(w http.ResponseWriter, r *http.Request, detail string) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"bad-request"),
		rfc9457.WithTitle("Bad Request"),
		rfc9457.WithStatus(http.StatusBadRequest),
		rfc9457.WithDetail(detail),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// InviteInvalid returns a 400 response for invalid invite codes.
func InviteInvalid(w http.ResponseWriter, r *http.Request, reason string) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"invite-invalid"),
		rfc9457.WithTitle("Invalid Invite"),
		rfc9457.WithStatus(http.StatusBadRequest),
		rfc9457.WithDetail(reason),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// FileTooLarge returns a 413 response for oversized files.
func FileTooLarge(w http.ResponseWriter, r *http.Request) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"file-too-large"),
		rfc9457.WithTitle("File Too Large"),
		rfc9457.WithStatus(http.StatusRequestEntityTooLarge),
		rfc9457.WithDetail("The uploaded file exceeds the maximum allowed size of 5 MB"),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// InvalidFileType returns a 415 response for unsupported file types.
func InvalidFileType(w http.ResponseWriter, r *http.Request) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"invalid-file-type"),
		rfc9457.WithTitle("Invalid File Type"),
		rfc9457.WithStatus(http.StatusUnsupportedMediaType),
		rfc9457.WithDetail("The uploaded file type is not supported. Allowed types: PNG, JPEG, WebP, GIF"),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// ServiceUnavailable returns a 503 response when a required service is unavailable.
func ServiceUnavailable(w http.ResponseWriter, r *http.Request, detail string) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"service-unavailable"),
		rfc9457.WithTitle("Service Unavailable"),
		rfc9457.WithStatus(http.StatusServiceUnavailable),
		rfc9457.WithDetail(detail),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}

// TooManyRequests returns a 429 response without retry-after information.
// Use RateLimited if you have retry-after information available.
func TooManyRequests(w http.ResponseWriter, r *http.Request) {
	rfc9457.NewRFC9457(
		rfc9457.WithType(ProblemBaseURI+"rate-limited"),
		rfc9457.WithTitle("Too Many Requests"),
		rfc9457.WithStatus(http.StatusTooManyRequests),
		rfc9457.WithDetail("Rate limit exceeded. Please try again later."),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithExtensions(
			rfc9457.NewExtension("request_id", GetRequestID(r)),
		),
	).ServeHTTP(w, r)
}
