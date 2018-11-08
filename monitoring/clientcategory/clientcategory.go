// Copyright 2018 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package clientcategory provides facilities to get the category of client that runs
// cloudsql-proxy.
package clientcategory

import (
	"net/http"
	"os"
	"sync"
)

var (
	onGAEFlex = false
	onGCE     bool
	onGCEOnce sync.Once
)

// Get returns the client category.
func Get() string {
	switch {
	case OnGAEFlex():
		return "GAE Flex"
	case OnGKE():
		return "GKE"
	// Since GKE and GAE Flex are run on GCE, we conclude that we're on GCE after excluding
	// GAE Flex and GKE.
	case OnGCE():
		return "GCE"
	// If all other check fails, we conclude that we're running On-Premise.
	default:
		return "On-Premise"
	}
}

// OnGAEFlex checks whether the client is running on GAE flex or not.
func OnGAEFlex() bool {
	return onGAEFlex
}

// OnGKE checks whether the client is running on GKE or not. We use the presence of kubernetes
// specific env variable to tell whether we're in GKE or not. For simlar uses, see
// https://github.com/kubernetes/client-go/blob/ca74aedde1ee2ea0ae6a39c3fb58409c1f04840f/rest/config.go#L320-L323
func OnGKE() bool {
	return os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != ""
}

// OnGCE checks whether the client is running on GCE or not. The return value is not mutually
// exclusive to those of OnGAEFlex and OnGKE.
func OnGCE() bool {
	onGCEOnce.Do(initOnGCE)
	return onGCE
}

// initOnGCE implements OnGCE. See https://github.com/GoogleCloudPlatform/gcloud-golang/issues/194
func initOnGCE() {
	res, err := http.Get("http://metadata.google.internal")
	if err != nil {
		onGCE = false
	} else {
		onGCE = res.Header.Get("Metadata-Flavor") == "Google"
	}
}
