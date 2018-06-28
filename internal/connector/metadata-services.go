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

	"github.com/gorilla/mux"
	"github.com/kinvolk/wormhole-connector/openapi"
	log "github.com/sirupsen/logrus"
)

// DeleteServiceByServiceId
func (wc *WormholeConnector) deleteService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := getLogger(ctx)
	wc.doDeleteService(ctx, w, r, l)
}

func (wc *WormholeConnector) doDeleteService(ctx context.Context, w http.ResponseWriter, r *http.Request, logger *log.Entry) {
	logger.Println("doing doDeleteService")

	serviceMetadataApi := wc.getApiClient(r).ServiceMetadataApi
	if serviceMetadataApi == nil {
		logger.Fatalln("unable to find serviceMetadataApi")
		return
	}

	vars := mux.Vars(r)
	serviceId := vars["serviceId"]
	if serviceId == "" {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, fmt.Sprintf("empty serviceId"))
		return
	}

	_, err := serviceMetadataApi.DeleteServiceByServiceId(ctx, serviceId)
	if err != nil {
		logger.Fatalf("unable to delete service by ID %v: %v", serviceId, err)
		return
	}

	io.WriteString(w, fmt.Sprintf("deleted service by ID %v", serviceId))
	logger.Println("deleted service by ID %v", serviceId)
}

// GetServices
func (wc *WormholeConnector) getServices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := getLogger(ctx)
	wc.doGetServices(ctx, w, r, l)
}

func (wc *WormholeConnector) doGetServices(ctx context.Context, w http.ResponseWriter, r *http.Request, logger *log.Entry) {
	logger.Println("doing doGetServices")

	serviceMetadataApi := wc.getApiClient(r).ServiceMetadataApi
	if serviceMetadataApi == nil {
		logger.Fatalln("unable to find serviceMetadataApi")
		return
	}

	services, _, err := serviceMetadataApi.GetServices(ctx)
	if err != nil {
		logger.Fatalf("unable to get services: %v", err)
		return
	}

	io.WriteString(w, fmt.Sprintf("%v", services))
	logger.Println("got services %v", services)
}

// GetServiceByServiceId
func (wc *WormholeConnector) getServiceById(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := getLogger(ctx)
	wc.doGetServiceById(ctx, w, r, l)
}

func (wc *WormholeConnector) doGetServiceById(ctx context.Context, w http.ResponseWriter, r *http.Request, logger *log.Entry) {
	logger.Println("doing doGetServiceById")

	serviceMetadataApi := wc.getApiClient(r).ServiceMetadataApi
	if serviceMetadataApi == nil {
		logger.Fatalln("unable to find serviceMetadataApi")
		return
	}

	vars := mux.Vars(r)
	serviceId := vars["serviceId"]
	if serviceId == "" {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, fmt.Sprintf("empty serviceId"))
		return
	}

	service, _, err := serviceMetadataApi.GetServiceByServiceId(ctx, serviceId)
	if err != nil {
		logger.Fatalf("unable to get service: %v", err)
		return
	}

	io.WriteString(w, fmt.Sprintf("got service by ID %v: %v", serviceId, service))
	logger.Println("got service by ID %v: %v", serviceId, service)
}

// RegisterService
func (wc *WormholeConnector) registerService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := getLogger(ctx)
	wc.doRegisterService(ctx, w, r, l)
}

func (wc *WormholeConnector) doRegisterService(ctx context.Context, w http.ResponseWriter, r *http.Request, logger *log.Entry) {
	logger.Println("doing doRegisterService")

	serviceMetadataApi := wc.getApiClient(r).ServiceMetadataApi
	if serviceMetadataApi == nil {
		logger.Fatalln("unable to find serviceMetadataApi")
		return
	}

	// TODO: fill the service details
	serviceDetails := openapi.ServiceDetails{
		Name: "test service",
	}

	serviceId, _, err := serviceMetadataApi.RegisterService(ctx, serviceDetails)
	if err != nil {
		logger.Fatalf("unable to register service by ID %v: %v", serviceId, err)
		return
	}

	io.WriteString(w, fmt.Sprintf("register service by ID %v: %v", serviceId, serviceDetails))
	logger.Println("register service by ID %v: %v", serviceId, serviceDetails)
}

// UpdateService
func (wc *WormholeConnector) updateService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := getLogger(ctx)
	wc.doUpdateService(ctx, w, r, l)
}

func (wc *WormholeConnector) doUpdateService(ctx context.Context, w http.ResponseWriter, r *http.Request, logger *log.Entry) {
	logger.Println("doing doUpdateService")

	serviceMetadataApi := wc.getApiClient(r).ServiceMetadataApi
	if serviceMetadataApi == nil {
		logger.Fatalln("unable to find serviceMetadataApi")
		return
	}

	vars := mux.Vars(r)
	serviceId := vars["serviceId"]
	if serviceId == "" {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, fmt.Sprintf("empty serviceId"))
		return
	}

	// TODO: fill the service details
	serviceDetails := openapi.ServiceDetails{
		Name: "test service",
	}

	_, err := serviceMetadataApi.UpdateService(ctx, serviceId, serviceDetails)
	if err != nil {
		logger.Fatalf("unable to update service by ID %v: %v", serviceId, err)
		return
	}

	io.WriteString(w, fmt.Sprintf("updated service by ID %v: %v", serviceId, serviceDetails))
	logger.Println("updated service by ID %v: %v", serviceId, serviceDetails)
}
