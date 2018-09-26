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

// Package recutil contains utility features for use throughout the Cloud SQL Proxy monitored metric
// recording.
package recutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/metrics"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/monitoring"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/record"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/tag"

	octag "go.opencensus.io/tag"
	"google.golang.org/api/googleapi"
)

// Following strings are tag values that appears multiple times.
const (
	// AdminAPIErr is the connection termination code due to admin API call error.
	AdminAPIErr = "admin API call error"
	// OK is the connection termination code for successful termination.
	OK = "OK"
	// InstancesGet is the API mehod name of Instances.Get.
	InstancesGet = "Instances.Get"
)

// Version is the version of cloudsql-proxy. Users may set it and use it, but it is intended to set
// only once at the beginning of the proxy client execution.
var Version string

var (
	// failedInstances saves the list of instances that failed on GetCtx.
	failedInstances = make(map[string]bool)
	// failedIntancesMu protects access to failedInstances
	failedInstancesMu sync.Mutex
)

// GetCtx is a wrapper of record.GetContext. This function does following:
// 1. Always returns a valid context that can be used. While doing that, it also checks whether
//    monitoring is used or not, and whether this function failed for same instance before to avoid
//    unnecessary error logs.
// 2. Returned error is suitable for logging.
func GetCtx(ctx context.Context, instance string) (newCtx context.Context, err error) {
	// When monitoring is not used, trying to call record.GetContext can generate unnecessary
	// error logs. So return nil error with doRecord is false.
	if !monitoring.Initialized() {
		return ctx, nil
	}
	failedInstancesMu.Lock()
	defer failedInstancesMu.Unlock()
	if failedInstances[instance] {
		return ctx, nil
	}

	newCtx, err = record.GetContext(ctx, instance)
	if err != nil {
		failedInstances[instance] = true
		return ctx, fmt.Errorf("monitoring error on instance %s: getting context for recording failed, and no further recording will be attempted: %v", instance, err)
	}
	return newCtx, nil
}

// UpdateIPType updates ipType to the context. Returned context is valid for use even on error
// return and returned error is suitable for logging.
func UpdateIPType(ctx context.Context, instance, ipType string) (context.Context, error) {
	newCtx, err := octag.New(ctx, octag.Update(tag.IPTypeKey, ipType))
	if err != nil {
		return ctx, fmt.Errorf("monitoring error on instance %s: setting IP address type %s failed: %v", instance, ipType, err)
	}
	return newCtx, nil
}

// Err generates an error suitable to log for failed record operation.
func Err(instance string, metric *metrics.Metric, err error) error {
	return fmt.Errorf("monitoring error on instance %s: recording metric %v failed: %v", instance, metric, err)
}

// RespCode returns the response code of an admin API call from its error.
func RespCode(err error) string {
	gErr, ok := err.(*googleapi.Error)
	if ok {
		return fmt.Sprintf("%d", gErr.Code)
	}
	if err != nil {
		return "UNKNOWN"
	}
	// Success is 200.
	return "200"
}
