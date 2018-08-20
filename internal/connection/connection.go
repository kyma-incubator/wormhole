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
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package connection

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/kyma-incubator/wormhole/internal/http2error"
	"github.com/kyma-incubator/wormhole/internal/streamio"
)

// ServeHijack hijacks the connection from ResponseWriter, writes the response
// and proxies data between targetConn and hijacked connection.
func ServeHijack(w http.ResponseWriter, writ io.Writer, read io.Reader, bufferPool *sync.Pool) error {
	_, ok := w.(http.CloseNotifier)
	if !ok {
		return errors.New("ResponseWriter does not implement CloseNotifier")
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return errors.New("ResponseWriter does not implement Hijacker")
	}
	clientConn, bufReader, err := hijacker.Hijack()
	if err != nil {
		return fmt.Errorf("failed to hijack: %v", err)
	}
	defer clientConn.Close()

	// bufReader may contain unprocessed buffered data from the client.
	if bufReader != nil {
		if n := bufReader.Reader.Buffered(); n > 0 {
			rbuf, err := bufReader.Reader.Peek(n)
			if err != nil {
				return err
			}
			writ.Write(rbuf)
		}
	}

	// Since we hijacked the connection, we can't write and flush headers via w.
	// Let's handcraft the response and send it manually.
	res := &http.Response{StatusCode: http.StatusOK,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}
	res.Header.Set("Server", "wormhole-connector")

	err = res.Write(clientConn)
	if err != nil {
		return fmt.Errorf("failed to send response to client: %v", err)
	}

	err = streamio.DualStream(writ, clientConn, clientConn, read, bufferPool)
	if err != nil && !http2error.IsClientDisconnect(err) {
		return err
	}

	return nil
}
