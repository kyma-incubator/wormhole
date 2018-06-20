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

package connector

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/http2"
)

type WormholeConnector struct {
	server http.Server
}

type WormholeConnectorConfig struct {
	KymaServer string
	Timeout    time.Duration
}

func NewWormholeConnector(config WormholeConnectorConfig) *WormholeConnector {
	var srv http.Server

	srv.Addr = config.KymaServer
	srv.IdleTimeout = config.Timeout
	http2.ConfigureServer(&srv, &http2.Server{})

	mux := http.NewServeMux()
	srv.Handler = addLogger(mux)

	registerHandlers(mux)

	return &WormholeConnector{
		server: srv,
	}
}

func addLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), "logger", log.WithFields(log.Fields{
			"request_id": uuid.NewV4()}))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func registerHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/", doStuff)
}

func (w *WormholeConnector) ListenAndServeTLS(cert, key string) {
	go func() {
		if err := w.server.ListenAndServeTLS(cert, key); err != nil {
			if err != http.ErrServerClosed {
				log.Fatal(err)
			}
		}
	}()
}

func (w *WormholeConnector) Shutdown(ctx context.Context) {
	w.server.Shutdown(ctx)
}

func doStuff(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := getLogger(ctx)
	stuff(w, r, l)
}

func getLogger(ctx context.Context) *log.Entry {
	logger, ok := ctx.Value("logger").(*log.Entry)
	if !ok {
		return nil
	}
	return logger
}

func stuff(w http.ResponseWriter, r *http.Request, logger *log.Entry) {
	logger.Println("doing stuff")
	io.WriteString(w, "THIS IS A RESPONSE")
}
