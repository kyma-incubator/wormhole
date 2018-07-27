package connector

import "strings"

func http2isClientDisconnect(err error) bool {
	if err == nil {
		return false
	}

	str := err.Error()
	// the connection is closed on the other end
	if strings.Contains(str, "use of closed network connection") {
		return true
	}

	// the client sent a RST_STREAM
	if strings.Contains(str, "stream ID") && strings.Contains(str, "; CANCEL") {
		return true
	}

	// client disconnected
	if strings.Contains(str, "client disconnected") {
		return true
	}

	return false
}
