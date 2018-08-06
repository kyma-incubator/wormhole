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

package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"

	"github.com/cloudflare/cfssl/log"
)

func GenerateTLSConfig(trustCAFile string, insecure bool) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: insecure,
	}

	if insecure || trustCAFile == "" {
		return tlsConfig, nil
	}

	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	certs, err := ioutil.ReadFile(trustCAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to append %q to RootCAs: %v", trustCAFile, err)
	}

	if ok := rootCAs.AppendCertsFromPEM(certs); !ok {
		log.Infof("no certs appended, using system certs only")
	}

	tlsConfig.RootCAs = rootCAs

	return tlsConfig, nil
}
