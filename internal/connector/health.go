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
	"fmt"
	"io"
	"net/http"

	log "github.com/sirupsen/logrus"
)

// GetHealth
func (wc *WormholeConnector) getHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := getLogger(ctx)
	wc.doGetHealth(ctx, w, r, l)
}

func (wc *WormholeConnector) doGetHealth(ctx context.Context, w http.ResponseWriter, r *http.Request, logger *log.Entry) {
	logger.Println("doing doGetHealth")

	healthApi := wc.getApiClient(r).HealthApi
	if healthApi == nil {
		logger.Fatalln("unable to find healthApi")
		return
	}

	response, err := healthApi.GetHealth(ctx)
	if err != nil {
		logger.Fatalf("unable to get health: %v\nresponse: %v", err, response)
		return
	}

	io.WriteString(w, fmt.Sprintf("%v", response))
	logger.Println("got health %v", healthApi)
}
