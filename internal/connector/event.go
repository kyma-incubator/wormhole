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

	"github.com/kinvolk/wormhole-connector/openapi"
	log "github.com/sirupsen/logrus"
)

// PublishEvent
func (wc *WormholeConnector) publishEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := getLogger(ctx)
	wc.doPublishEvent(ctx, w, r, l)
}

func (wc *WormholeConnector) doPublishEvent(ctx context.Context, w http.ResponseWriter, r *http.Request, logger *log.Entry) {
	logger.Println("doing doPublishEvent")

	publishApi := wc.getApiClient(r).PublishApi
	if publishApi == nil {
		logger.Fatalln("unable to find publishApi")
		return
	}

	// TODO: fill the publish request
	publishRequest := openapi.PublishRequest{
		EventType: "",
	}

	publishResponse, httpResponse, err := publishApi.PublishEvent(ctx, publishRequest)
	if err != nil {
		logger.Fatalf("unable to get events: %v", err)
		return
	}

	io.WriteString(w, fmt.Sprintf("%v", httpResponse))
	logger.Println("published events, response: %v", publishResponse)
}
