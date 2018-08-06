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

package tunnel

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

type RequestSchema int

const (
	RequestHTTP RequestSchema = iota
	RequestHTTPS
)

type Endpoint int

const (
	ConnectorEndpoint = iota
	DispatcherEndpoint
)

func Create(client *http.Client, host, endpointAddr string, schema RequestSchema, endpoint Endpoint) (io.Writer, *http.Response, error) {
	pr, pw := io.Pipe()
	tunnelReq, err := http.NewRequest(http.MethodConnect, endpointAddr, ioutil.NopCloser(pr))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to prepare request: %v", err)
	}

	// set Host in the tunnel request to the host requested by the client
	tunnelReq.Host = host

	switch endpoint {
	case ConnectorEndpoint:
		tunnelReq.Header.Set("X-Wormhole-Connector", "true")
	case DispatcherEndpoint:
		tunnelReq.Header.Set("X-Wormhole-Dispatcher", "true")
	}

	if schema == RequestHTTP {
		// set protocol so the other end of the tunnel knows what port to dial
		tunnelReq.Header.Set("X-Forwarded-Proto", "http")
	}

	// establish tunnel, if a tunnel is already established, the same HTTP/2
	// connection will be used
	tunnelRes, err := client.Do(tunnelReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to do request: %v", err)
	}

	return pw, tunnelRes, nil
}
