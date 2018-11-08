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

// Package recutil contains utility functions and constants for use throughout the Cloud SQL Proxy
// monitored metric recording.
package recutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/metrics"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/monitoring"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/record"
	"google.golang.org/api/googleapi"
)

// Following strings are tag values that appears multiple times.
const (

	// InstancesGet is the admin API mehod name of Instances.Get.
	InstancesGet = "Instances.Get"
	// SslCertsCreateEphemeral is the admin API method name of SslCerts.CreateEphemeral.
	SslCertsCreateEphemeral = "SslCerts.CreateEphemeral"
	// Unknown used for describe unknown tag values for recording.
	Unknown = "UNKNOWN"
)

var (
	// failedInstances saves the list of instances that failed on AddTags.
	failedInstances = make(map[string]bool)
	// failedIntancesMu protects access to failedInstances
	failedInstancesMu sync.Mutex

	// Version is the version of cloudsql-proxy. Users may set it and use it, but it is expected
	// to be set only once at the beginning of the proxy client execution.
	Version = "NO_VERSION_SET"
)

// AddTags is a wrapper of record.AddTags. This function does following:
// 1. Always returns a valid context that can be used. While doing that, it also checks whether
//    monitoring is used or not, and whether this function failed for same instance before to avoid
//    unnecessary error logs.
// 2. Returned error is suitable for logging.
func AddTags(ctx context.Context, instance string) (newCtx context.Context, err error) {
	// When monitoring is not used, trying to call record.AddTags can generate unnecessary
	// error logs. So return nil error with doRecord is false.
	if !monitoring.Initialized() {
		return ctx, nil
	}
	failedInstancesMu.Lock()
	defer failedInstancesMu.Unlock()
	if failedInstances[instance] {
		return ctx, nil
	}

	newCtx, err = record.AddTags(ctx, instance)
	if err != nil {
		failedInstances[instance] = true
		return ctx, fmt.Errorf("monitoring error on instance %s: getting context for recording failed, and no further recording will be attempted: %v", instance, err)
	}
	return newCtx, nil
}

// Err generates an error suitable to log for failed record operation.
func Err(instance string, metric *metrics.Metric, err error) error {
	return fmt.Errorf("monitoring error on instance %s: recording metric %v failed: %v", instance, metric, err)
}

// RespCode returns the response code of an admin API call from its error.
func RespCode(err error) string {
	if gErr, ok := err.(*googleapi.Error); ok {
		return fmt.Sprintf("%d", gErr.Code)
	}
	if err != nil {
		return Unknown
	}
	// Success is 200.
	return "200"
}
