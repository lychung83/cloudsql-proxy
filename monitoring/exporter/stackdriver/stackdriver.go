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

// Package stackdriver provides an exporter that uploads data from opencensus to stackdriver
// metrics of multiple GCP projects.
//
// General assumptions or requirements when using this exporter.
// 1. The basic unit of data is a view.Data with only a single view.Row. We define it as a separate
//    type called RowData.
// 2. We can inspect each RowData to tell whether this RowData is applicable for this exporter.
// 3. For RowData that is applicable to this exporter, we require that
// 3.1. Any view associated to RowData corresponds to a stackdriver metric, and it is already
//      defined for all GCP projects.
// 3.2. RowData has correcponding GCP projects, and we can determine its project ID.
// 3.3. After trimming labels and tags, configuration of all view data matches that of corresponding
//      stackdriver metric
package stackdriver

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"google.golang.org/api/option"
	"google.golang.org/api/support/bundler"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	mpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

// Exporter is the exporter that can be registered to opencensus. An Exporter object must be
// created by NewExporter().
type Exporter struct {
	// TODO(lawrencechung): If possible, find a way to not storing ctx in the struct.
	ctx    context.Context
	client *monitoring.MetricClient
	opts   Options

	// mu protects access to projDataMap
	mu sync.Mutex
	// per-project data of exporter
	projDataMap map[string]*projectData
}

// Options designates various parameters used by stats exporter. Default value of fields in Options
// are valid for use.
type Options struct {
	// ClientOptions designates options for creating metric client, especially credentials for
	// RPC calls.
	ClientOptions []option.ClientOption

	// Options for bundles amortizing export requests. Note that a bundle is created for each
	// project. When not provided, default values in bundle package are used.

	// BundleDelayThreshold determines the max amount of time the exporter can wait before
	// uploading data to the stackdriver.
	BundleDelayThreshold time.Duration
	// BundleCountThreshold determines how many RowData objects can be buffered before batch
	// uploading them to the backend.
	BundleCountThreshold int

	// Callback functions provided by user.

	// GetProjectID is used to filter whether given row data can be applicable to this exporter
	// and if so, it also determines the projectID of given row data. If
	// RowDataNotApplicableError is returned, then the row data is not applicable to this
	// exporter, and it will be silently ignored. Though not recommended, other errors can be
	// returned, and in that case the error is reported to callers via OnError and the row data
	// will not be uploaded to stackdriver. When GetProjectID is not set, for any row data with
	// tag key name "project_id" (it's defined as ProjectKeyName), the value of the tag will be
	// it's project ID. All other row data will be silently ignored.
	GetProjectID func(*RowData) (projectID string, err error)
	// OnError is used to report any error happened while exporting view data fails. Whenever
	// this function is called, it's guaranteed that at least one row data is also passed to
	// OnError. Row data passed to OnError must not be modified and OnError must be
	// non-blocking. When OnError is not set, all errors happened on exporting are ignored.
	OnError func(error, ...*RowData)
	// MakeResource creates monitored resource from RowData. It is guaranteed that only RowData
	// that passes GetProjectID will be given to this function. Though not recommended, error
	// can be returned, and in that case the error is reported to callers via OnError and the
	// row data will not be uploaded to stackdriver. When MakeResource is not set, global
	// resource is used for all RowData objects.
	MakeResource func(rd *RowData) (*mrpb.MonitoredResource, error)

	// Options concerning labels.

	// DefaultLabels store default value of some labels. Labels in DefaultLabels need not be
	// specified in tags of view data. Default labels and tags of view may have overlapping
	// label keys. In this case, values in tag are used. Default labels are used for labels
	// those are constant throughout export operation, like version number of the calling
	// program.
	DefaultLabels map[string]string
	// UnexportedLabels contains key of labels that will not be exported stackdriver. Typical
	// uses of unexported labels will be either that marks project ID, or that's used only for
	// constructing resource.
	UnexportedLabels []string
}

// ProjectKeyName is used by defaultGetProjectID to get the project ID of a given row data.
const ProjectKeyName = "project_id"

// Default values for options. Their semantics are described in Options.

func defaultGetProjectID(rd *RowData) (string, error) {
	for _, tag := range rd.Row.Tags {
		if tag.Key.Name() == ProjectKeyName {
			return tag.Value, nil
		}
	}
	return "", RowDataNotApplicableError
}

func defaultOnError(err error, rds ...*RowData) {}

func defaultMakeResource(rd *RowData) (*mrpb.MonitoredResource, error) {
	return &mrpb.MonitoredResource{Type: "global"}, nil
}

// Following functions are wrapper of functions those will be mocked by tests. Only tests can modify
// these functions.
var (
	newMetricClient  = monitoring.NewMetricClient
	createTimeSeries = (*monitoring.MetricClient).CreateTimeSeries
	newBundler       = bundler.NewBundler
	addToBundler     = (*bundler.Bundler).Add
)

// NewExporter creates an Exporter object. Once a call to NewExporter is made, any fields in opts
// must not be modified at all. ctx will also be used throughout entire exporter operation when
// making RPC call.
func NewExporter(ctx context.Context, opts Options) (*Exporter, error) {
	client, err := newMetricClient(ctx, opts.ClientOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create a metric client: %v", err)
	}

	e := &Exporter{
		ctx:         ctx,
		client:      client,
		opts:        opts,
		projDataMap: make(map[string]*projectData),
	}

	if e.opts.GetProjectID == nil {
		e.opts.GetProjectID = defaultGetProjectID
	}
	if e.opts.OnError == nil {
		e.opts.OnError = defaultOnError
	}
	if e.opts.MakeResource == nil {
		e.opts.MakeResource = defaultMakeResource
	}

	return e, nil
}

// RowData represents a single row in view data. This is our unit of computation. We use a single
// row instead of view data because a view data consists of multiple rows, and each row may belong
// to different projects.
type RowData struct {
	View       *view.View
	Start, End time.Time
	Row        *view.Row
}

// ExportView is the method called by opencensus to export view data. It constructs RowData out of
// view.Data objects.
func (e *Exporter) ExportView(vd *view.Data) {
	for _, row := range vd.Rows {
		rd := &RowData{
			View:  vd.View,
			Start: vd.Start,
			End:   vd.End,
			Row:   row,
		}
		e.exportRowData(rd)
	}
}

// RowDataNotApplicableError is used to tell that given row data is not applicable to the exporter.
// See GetProjectID of Options for more detail.
var RowDataNotApplicableError = errors.New("row data is not applicable to the exporter, so it will be ignored")

// exportRowData exports a single row data.
func (e *Exporter) exportRowData(rd *RowData) {
	projID, err := e.opts.GetProjectID(rd)
	if err != nil {
		// We ignore non-applicable RowData.
		if err != RowDataNotApplicableError {
			newErr := fmt.Errorf("failed to get project ID on row data with view %s: %v", rd.View.Name, err)
			e.opts.OnError(newErr, rd)
		}
		return
	}
	pd := e.getProjectData(projID)
	switch err := addToBundler(pd.bndler, rd, 1); err {
	case nil:
	case bundler.ErrOversizedItem:
		go pd.uploadRowData(rd)
	default:
		newErr := fmt.Errorf("failed to add row data with view %s to bundle for project %s: %v", rd.View.Name, projID, err)
		e.opts.OnError(newErr, rd)
	}
}

func (e *Exporter) getProjectData(projectID string) *projectData {
	e.mu.Lock()
	defer e.mu.Unlock()
	if pd, ok := e.projDataMap[projectID]; ok {
		return pd
	}

	pd := e.newProjectData(projectID)
	e.projDataMap[projectID] = pd
	return pd
}

func (e *Exporter) newProjectData(projectID string) *projectData {
	pd := &projectData{
		parent:    e,
		projectID: projectID,
	}

	pd.bndler = newBundler((*RowData)(nil), pd.uploadRowData)
	// Set options for bundler if they are provided by users.
	if 0 < e.opts.BundleDelayThreshold {
		pd.bndler.DelayThreshold = e.opts.BundleDelayThreshold
	}
	if 0 < e.opts.BundleCountThreshold {
		pd.bndler.BundleCountThreshold = e.opts.BundleCountThreshold
	}
	return pd
}

// Close flushes and closes the exporter. Close must be called after the exporter is unregistered
// and no further calls to ExportView() are made. Once Close() is returned no further access to the
// exporter is allowed in any way.
func (e *Exporter) Close() error {
	e.mu.Lock()
	for _, pd := range e.projDataMap {
		pd.bndler.Flush()
	}
	e.mu.Unlock()

	if err := e.client.Close(); err != nil {
		return fmt.Errorf("failed to close the metric client: %v", err)
	}
	return nil
}

// makeTS constructs a time series from a row data.
func (e *Exporter) makeTS(rd *RowData) (*mpb.TimeSeries, error) {
	pt := newPoint(rd.View, rd.Row, rd.Start, rd.End)
	if pt.Value == nil {
		return nil, fmt.Errorf("inconsistent data found in view %s", rd.View.Name)
	}
	resource, err := e.opts.MakeResource(rd)
	if err != nil {
		return nil, fmt.Errorf("failed to construct resource of view %s: %v", rd.View.Name, err)
	}
	ts := &mpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   rd.View.Name,
			Labels: e.makeLabels(rd.Row.Tags),
		},
		Resource: resource,
		Points:   []*mpb.Point{pt},
	}
	return ts, nil
}

// makeLables constructs label that's ready for being uploaded to stackdriver.
func (e *Exporter) makeLabels(tags []tag.Tag) map[string]string {
	opts := e.opts
	labels := make(map[string]string, len(opts.DefaultLabels)+len(tags))
	for key, val := range opts.DefaultLabels {
		labels[key] = val
	}
	// If there's overlap When combining exporter's default label and tags, values in tags win.
	for _, tag := range tags {
		labels[tag.Key.Name()] = tag.Value
	}
	// Some labels are not for exporting.
	for _, key := range opts.UnexportedLabels {
		delete(labels, key)
	}
	return labels
}
