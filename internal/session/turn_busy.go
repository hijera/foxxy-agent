package session

import "errors"

// ErrSessionTurnBusy is returned when another agent turn holds the session turn lock.
var ErrSessionTurnBusy = errors.New("session busy: another agent turn is in progress")
