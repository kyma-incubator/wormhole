// Copyright © 2018 The wormhole-connector authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,

package http2error

import "strings"

// IsClientDisconnect returns whether an error is a client side disconnect.
//
// If the client side is closed or the client cancels, that doesn't
// mean there's an error
func IsClientDisconnect(err error) bool {
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
