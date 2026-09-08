package binance

import (
	"errors"
	"net"
	"strings"
)

// Retryable reports transient venue/network faults. Filter and auth errors are not.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	var api *APIError
	if errors.As(err, &api) {
		if api.Status >= 500 {
			return true
		}
		switch api.Code {
		case -1001, -1003, -1006, -1007, -1015, -1021:
			return true
		default:
			return false
		}
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "connection reset")
}

func FilterFailure(err error) bool {
	var api *APIError
	return errors.As(err, &api) && api.Code == -1013
}
