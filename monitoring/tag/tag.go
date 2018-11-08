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

// Package tag defines opencensus tags used by cloudsqlproxy monitoring.
package tag

import octag "go.opencensus.io/tag"

var (
	// ProjectKey is the key for Project ID of an instance.
	ProjectKey, _ = octag.NewKey("project_id")
	// RegionKey is the key for the region name of an instance.
	RegionKey, _ = octag.NewKey("region")
	// DatabaseKey is the key for the database ID of an instance.
	DatabaseKey, _ = octag.NewKey("database_id")
	// IPTypeKey is the key for IP address type use for a connection.
	IPTypeKey, _ = octag.NewKey("ip_type")
	// APIMethodKey is the key for the name of API method.
	APIMethodKey, _ = octag.NewKey("api_method")
	// RespCodeKey is the key for the response code of an API request.
	RespCodeKey, _ = octag.NewKey("response_code")
	// StrValKey is the key for the string metric value, whose tag value is storing string type
	// metric value which is not supported by opencensus.
	StrValKey, _ = octag.NewKey("string_metric_value")
	// TermCodeKey is the key for the termination code of a connection.
	TermCodeKey, _ = octag.NewKey("termination_code")

	// MandatoryKeys is the list of keys required by all metrics.
	MandatoryKeys = []octag.Key{ProjectKey, RegionKey, DatabaseKey}
)
