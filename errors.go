package pbsynadia

import (
	"errors"
	"fmt"
	"strings"
)

// Typed errors. The Synadia Cloud REST API is at /core/beta/* — surface is
// subject to change, so error messages from the SDK are wrapped but not
// mapped exhaustively.
var (
	// Configuration errors
	ErrInvalidOptions = errors.New("invalid options provided")
	ErrMissingToken   = errors.New("Synadia API token not configured")
	ErrMissingSystem  = errors.New("Synadia system ID not configured")

	// Collection / record errors
	ErrCollectionNotFound = errors.New("collection not found")
	ErrRecordNotFound     = errors.New("record not found")
	ErrInvalidRecord      = errors.New("invalid record data")

	// Account errors
	ErrAccountNotFound = errors.New("account not found")
	ErrAccountInactive = errors.New("account is inactive")

	// User errors
	ErrUserNotFound      = errors.New("user not found")
	ErrUserInactive      = errors.New("user is inactive")
	ErrUserAlreadyExists = errors.New("user already exists")

	// Role errors
	ErrRoleNotFound      = errors.New("role not found")
	ErrInvalidPermission = errors.New("invalid permission")

	// Synadia API errors
	ErrSynadiaUnavailable = errors.New("Synadia API unavailable")
	ErrSynadiaCallFailed  = errors.New("Synadia API call failed")
)

// IsTemporaryError reports whether err looks like a transient network or
// Synadia availability issue worth retrying. Used by Reconcile to decide
// whether to leave a record in pending_* state for a future retry.
func IsTemporaryError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrSynadiaUnavailable) || errors.Is(err, ErrSynadiaCallFailed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, p := range []string{
		"connection refused",
		"timeout",
		"temporary failure",
		"network is unreachable",
		"no route to host",
		"connection reset",
		"connection timed out",
		"503",
		"502",
		"504",
	} {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// IsPermanentError reports whether err is unlikely to succeed on retry —
// validation, authentication, or "not found" failures.
func IsPermanentError(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrInvalidRecord),
		errors.Is(err, ErrInvalidPermission),
		errors.Is(err, ErrInvalidOptions),
		errors.Is(err, ErrUserAlreadyExists):
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, p := range []string{
		"invalid",
		"forbidden",
		"unauthorized",
		"bad request",
		"already exists",
		"400",
		"401",
		"403",
	} {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// IsConfigurationError reports whether err is related to setup that requires
// admin intervention (missing token, missing system id, etc.).
func IsConfigurationError(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrInvalidOptions),
		errors.Is(err, ErrMissingToken),
		errors.Is(err, ErrMissingSystem):
		return true
	}
	return false
}

// WrapSynadiaError wraps an SDK call error with operation context.
func WrapSynadiaError(err error, operation string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("Synadia %s: %w", operation, err)
}

// WrapRecordError wraps a record-level error with the record id and operation.
func WrapRecordError(err error, recordID, operation string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("record %q %s: %w", recordID, operation, err)
}
