package i2plib

import "fmt"

const (
	SAMErrCantReachPeer  = "CANT_REACH_PEER"
	SAMErrDuplicatedDest = "DUPLICATED_DEST"
	SAMErrDuplicatedID   = "DUPLICATED_ID"
	SAMErrI2PError       = "I2P_ERROR"
	SAMErrInvalidID      = "INVALID_ID"
	SAMErrInvalidKey     = "INVALID_KEY"
	SAMErrKeyNotFound    = "KEY_NOT_FOUND"
	SAMErrPeerNotFound   = "PEER_NOT_FOUND"
	SAMErrTimeout        = "TIMEOUT"
)

// Base SAM error type
type SAMError struct {
	Code string
	Msg  string
}

func (e *SAMError) Error() string {
	if e.Msg == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}

func (e *SAMError) Is(target error) bool {
	t, ok := target.(*SAMError)
	if !ok {
		return false
	}
	if t.Code == "" {
		return false
	}
	return e.Code == t.Code
}

// Specific error constructors

func ErrCantReachPeer(msg string) error  { return &SAMError{Code: SAMErrCantReachPeer, Msg: msg} }
func ErrDuplicatedDest(msg string) error { return &SAMError{Code: SAMErrDuplicatedDest, Msg: msg} }
func ErrDuplicatedID(msg string) error   { return &SAMError{Code: SAMErrDuplicatedID, Msg: msg} }
func ErrI2PError(msg string) error       { return &SAMError{Code: SAMErrI2PError, Msg: msg} }
func ErrInvalidID(msg string) error      { return &SAMError{Code: SAMErrInvalidID, Msg: msg} }
func ErrInvalidKey(msg string) error     { return &SAMError{Code: SAMErrInvalidKey, Msg: msg} }
func ErrKeyNotFound(msg string) error    { return &SAMError{Code: SAMErrKeyNotFound, Msg: msg} }
func ErrPeerNotFound(msg string) error   { return &SAMError{Code: SAMErrPeerNotFound, Msg: msg} }
func ErrTimeout(msg string) error        { return &SAMError{Code: SAMErrTimeout, Msg: msg} }

// Lookup table — аналог SAM_EXCEPTIONS in Python
var SAMExceptionMap = map[string]func(string) error{
	SAMErrCantReachPeer:  ErrCantReachPeer,
	SAMErrDuplicatedDest: ErrDuplicatedDest,
	SAMErrDuplicatedID:   ErrDuplicatedID,
	SAMErrI2PError:       ErrI2PError,
	SAMErrInvalidID:      ErrInvalidID,
	SAMErrInvalidKey:     ErrInvalidKey,
	SAMErrKeyNotFound:    ErrKeyNotFound,
	SAMErrPeerNotFound:   ErrPeerNotFound,
	SAMErrTimeout:        ErrTimeout,
}
