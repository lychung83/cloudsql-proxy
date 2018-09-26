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

// Package monitoring provides overall facilities to manage monitoring service for cloudsql-proxy.
// To use monitoring service, cloudsql-proxy should call Init() before any metric is collected.
// When exiting the program, cloudsql-proxy should call Close().
//
// NOTE: we usually assume that monitoring is the only of user of opencensus of cloudsql-proxy. If
// not, be aware that this package sets opencensus report interval by calling
// view.SetReportingPeriod(), and changing this interval may affect the operation of this package in
// unexpected ways.
package monitoring

import (
	"context"
	"fmt"
	"strings"
	"time"

	// TODO(lawrencechung): use exporter provided by opencensus when they implement exporter
	// that meets cloudsql-proxy's requirement.
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/exporter/stackdriver"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/metrics"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/tag"

	"go.opencensus.io/stats/view"
	"google.golang.org/api/option"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

const (
	// DefaultReportDelay is the default interval between opencensus report.
	DefaultReportDelay = 30 * time.Second
	// DefaultErrLogDelay is the default interval between error logging.
	DefaultErrLogDelay = time.Minute
)

var (
	// projectName is the name of ProjectKey.
	projectName = tag.ProjectKey.Name()
	// regionName is the name of RegionKey.
	regionName = tag.RegionKey.Name()
	// databaseName is the name of DatabaseKey.
	databaseName = tag.DatabaseKey.Name()
	// strValName is the name of StrValKey.
	strValName = tag.StrValKey.Name()
)

var (
	// initialized tells whether monitroing service is successfully intialized or not.
	initialized bool
	// exp is the stackdriver exporter used to upload metrics.
	exp *stackdriver.Exporter
	// errHandler handles errors reported from exp.
	errHandler *errorHandler
	// uuid is the unique identifier of the cloudsql-proxy.
	uuid string
)

// Options provide various parameters for initializing monitoring services.
type Options struct {
	// ReportDelay is the interval between opencensus report. If this value is less or equal to
	// 0, then DefaultReportDelay is used.
	ReportDelay time.Duration
	// ErrLogDelay is the interval between error logging. If this value is less or equal to 0,
	// then DefaultErrLogDelay is used.
	ErrLogDelay time.Duration
	// ClientOptions is used to tune monitoring.Client.
	ClientOptions []option.ClientOption

	// Following fields are fixed values for labels and monitored resoruces. All of them must
	// pass checkStrValue().

	// ClientCategory denotes the category of runtime environment of cloudsql-proxy.
	ClientCategory string
	// UUID is a unique identifier of the execution of cloudsql-proxy.
	UUID string
}

// Initialize initializes monitoring service. It's generally a programming error when it fails.
// This function should not be called more than once, and if called, it should be called at the
// beginning of the program before all other monitroing operation.
func Initialize(ctx context.Context, opts Options) error {
	uuid = opts.UUID

	if err := metrics.Initialize(); err != nil {
		return fmt.Errorf("initializing the metrics failed: %v", err)
	}

	errHandler = newErrorHandler(opts.ErrLogDelay)

	// We define options for the exporter.
	// NOTE: we do not set BundleDelayThreshold. It is generally not a good idea to store
	// values in the bundle for long time since, there are some cases that stackdriver does not
	// accept data point of same value those only differs in timestamp.
	expOpts := stackdriver.Options{
		ClientOptions: opts.ClientOptions,
		// Since every data point in bundle corresponds to a time series when uploading them
		// to stackdriver, MaxTimeSeriesPerUpload is a natural bound for bundle cap.
		BundleCountThreshold: stackdriver.MaxTimeSeriesPerUpload,
		GetProjectID:         getProjectID,
		OnError:              errHandler.expErrHandler,
		MakeResource:         makeResource,
		IsValueString:        isValueString,
		DefaultLabels:        map[string]string{"client_category": opts.ClientCategory},
		// Tags used only by getProjectID and makeResource, and tag used to store string
		// metric value are not part of metric labels seen by stackdriver, so we do not
		// export them.
		UnexportedLabels: []string{projectName, regionName, databaseName, strValName},
	}

	var err error
	if exp, err = stackdriver.NewExporter(ctx, expOpts); err != nil {
		return fmt.Errorf("creating exporter failed: %v", err)
	}

	// All variables are defined successfully. Start monitoring.
	go errHandler.run()
	reportDelay := opts.ReportDelay
	if reportDelay <= 0 {
		reportDelay = DefaultReportDelay
	}
	view.SetReportingPeriod(reportDelay)
	view.RegisterExporter(exp)

	initialized = true
	return nil
}

// Initialized tells whether monitoring service is initialized or not.
func Initialized() bool {
	return initialized
}

// Close closes monitoring service. Once it's called, no other monitoring activity should be made.
// This function should not be called when Initialize is not called or returned error and this
// function is intended to be called at the end of program execution.
func Close() error {
	// TODO(lawrencechung): as soon as opencensus implements 'flush' feature,
	// ( see https://github.com/census-instrumentation/opencensus-go/issues/862 ) call it here
	// to make sure that all remaining data in opencensus are exported.
	view.UnregisterExporter(exp)
	err := exp.Close()
	if err != nil {
		err = fmt.Errorf("closing exporter failed: %v", err)
	}
	// Quit error handler and report any remaining errors.
	errHandler.stop()
	initialized = false
	return err
}

// getProjectID checks whether rd belongs to the monitored metric and if so, get the project ID of
// it. This function is used by the stackdriver exporter to classify row data passed to it.
func getProjectID(rd *stackdriver.RowData) (string, error) {
	// Filter all metrics that are not defined by monitoring package.
	if !strings.HasPrefix(rd.View.Name, "cloudsql.googleapis.com/database/proxy/client/") {
		return "", stackdriver.ErrRowDataNotApplicable
	}
	// Check the project tag to get the project ID.
	for _, tg := range rd.Row.Tags {
		if tg.Key == tag.ProjectKey {
			return tg.Value, nil
		}
	}
	return "", fmt.Errorf("no project tag found on the row data")
}

// makeResource creates proxy monitored resource by inspecting tags of the row data. This function
// is used by the stackdriver exporter to dynamically generate monitored resource.
// TODO(lawrencechung): Make sure that this is the monitored resource we define.
func makeResource(rd *stackdriver.RowData) (*mrpb.MonitoredResource, error) {
	resLabels := map[string]string{"uuid": uuid}
	for _, tg := range rd.Row.Tags {
		switch key := tg.Key; key {
		case tag.ProjectKey, tag.RegionKey, tag.DatabaseKey:
			resLabels[key.Name()] = tg.Value
		}
	}

	for _, keyName := range []string{projectName, regionName, databaseName} {
		if _, ok := resLabels[keyName]; !ok {
			return nil, fmt.Errorf("row data does not have expected key: %s", keyName)
		}
	}

	res := &mrpb.MonitoredResource{
		Type:   "cloudsqlproxy",
		Labels: resLabels,
	}
	return res, nil
}

// isValueString checks whether metric value type is string or not. Since opencensus does not
// support string as metric value, we use tag StrValKey as an indicator and storage for it.
func isValueString(rd *stackdriver.RowData) (string, bool, error) {
	for _, tg := range rd.Row.Tags {
		if tg.Key == tag.StrValKey {
			return tg.Value, true, nil
		}
	}
	return "", false, nil
}
