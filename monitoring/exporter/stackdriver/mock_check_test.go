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

package stackdriver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	monitoring "cloud.google.com/go/monitoring/apiv3"
	gax "github.com/googleapis/gax-go"
	"google.golang.org/api/option"
	"google.golang.org/api/support/bundler"
	mpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

// This file defines various mocks for testing, and checking functions for mocked data. We mock
// metric client and bundler because their actions involves RPC calls or non-deterministic behavior.

// Following data are used to store various data generated by exporters' activity. They are used by
// each test to verify intended behavior. Each test should call testDataInit() to clear these data.
var (
	// errStorage records all errors and associated RowData objects reported by exporter.
	errStorage []errRowData
	// projDataMap is a copy of projDataMap used by each tests.
	projDataMap map[string]*projectData
	// projRds saves all RowData objects passed to addToBundler call by project ID. Since a
	// value of a map is not addressable, we save the pointer to the slice.
	projRds map[string]*[]*RowData
	// timeSeriesReqs saves all incoming requests for creating time series.
	timeSeriesReqs []*mpb.CreateTimeSeriesRequest
	// timeSeriesResults holds predefined error values to be returned by mockCreateTimeSerie()
	// calls. Each errors in timeSeriesResults are returned per each mockCreateTimeSeries()
	// call. If all errors in timeSeriesResults are used, all other mockCreateTimeSeries calls
	// will return nil.
	timeSeriesResults []error
)

func init() {
	// For testing convenience, we reduce maximum time series that metric client accepts.
	MaxTimeSeriesPerUpload = 3

	// Mock functions.
	newMetricClient = mockNewMetricClient
	createTimeSeries = mockCreateTimeSeries
	newBundler = mockNewBundler
	addToBundler = mockAddToBundler
}

// testDataInit() initializes all data needed for each test. This function must be called at the
// beginning of each test.
func testDataInit() {
	projDataMap = nil
	projRds = map[string]*[]*RowData{}
	timeSeriesReqs = nil
	timeSeriesResults = nil
	errStorage = nil
}

// Mocked functions.

func mockNewMetricClient(_ context.Context, _ ...option.ClientOption) (*monitoring.MetricClient, error) {
	return nil, nil
}

func mockCreateTimeSeries(_ *monitoring.MetricClient, _ context.Context, req *mpb.CreateTimeSeriesRequest, _ ...gax.CallOption) error {
	timeSeriesReqs = append(timeSeriesReqs, req)
	// Check timeSeriesResults and if not empty, return the first error from it.
	if len(timeSeriesResults) == 0 {
		return nil
	}
	err := timeSeriesResults[0]
	// Delete the returning error.
	timeSeriesResults = timeSeriesResults[1:]
	return err
}

func mockNewBundler(_ interface{}, _ func(interface{})) *bundler.Bundler {
	// We do not return nil but create an empty Bundler object because
	// 1. Exporter.newProjectData() is setting fields of Bundler.
	// 2. mockAddToBundler needs to get the project ID of the bundler. To do that we need
	//    different address for each bundler.
	return &bundler.Bundler{}
}

func mockAddToBundler(bndler *bundler.Bundler, item interface{}, _ int) error {
	// Get the project ID of the bndler by inspecting projDataMap.
	var projID string
	projIDfound := false
	for tempProjID, pd := range projDataMap {
		if pd.bndler == bndler {
			projID = tempProjID
			projIDfound = true
			break
		}
	}
	if !projIDfound {
		return unrecognizedDataError
	}

	rds, ok := projRds[projID]
	if !ok {
		// For new project ID, create the actual slice and save its pointer.
		var rdsSlice []*RowData
		rds = &rdsSlice
		projRds[projID] = rds
	}
	*rds = append(*rds, item.(*RowData))
	return nil
}

// newTest*() functions create exporters and project data used for testing. Each test should call
// One of these functions once and only once, and never call NewExporter() directly.

// newTestExp creates an exporter which saves error to errStorage. Caller should not set
// opts.OnError.
func newTestExp(t *testing.T, opts *Options) *Exporter {
	opts.OnError = testOnError
	exp, err := NewExporter(ctx, opts)
	if err != nil {
		t.Fatalf("creating exporter failed: %v", err)
	}
	// Expose projDataMap so that mockAddToBundler() can use it.
	projDataMap = exp.projDataMap
	return exp
}

// newTestProjData creates a projectData object to test behavior of projectData.uploadRowData. Other
// uses are not recommended. As newTestExp, all errors are saved to errStorage.
func newTestProjData(t *testing.T, opts *Options) *projectData {
	return newTestExp(t, opts).newProjectData(project1)
}

// We define a storage for all errors happened in export operation.

type errRowData struct {
	err error
	rds []*RowData
}

// testOnError records any incoming error and accompanying RowData array. This function is passed to
// the exporter to record errors.
func testOnError(err error, rds ...*RowData) {
	errStorage = append(errStorage, errRowData{err, rds})
}

// checkMetricClient checks all recorded requests to the metric client. We only compare int64
// values of the time series. To make this work, we assigned different int64 values for all valid
// rows in the test.
func checkMetricClient(t *testing.T, wantReqsValues [][]int64) {
	reqsLen, wantReqsLen := len(timeSeriesReqs), len(wantReqsValues)
	if reqsLen != wantReqsLen {
		t.Errorf("number of requests got: %d, want %d", reqsLen, wantReqsLen)
		return
	}
	for i := 0; i < reqsLen; i++ {
		prefix := fmt.Sprintf("%d-th request mismatch", i+1)
		tsArr := timeSeriesReqs[i].TimeSeries
		wantTsValues := wantReqsValues[i]
		tsArrLen, wantTsArrLen := len(tsArr), len(wantTsValues)
		if tsArrLen != wantTsArrLen {
			t.Errorf("%s: number of time series got: %d, want: %d", prefix, tsArrLen, wantTsArrLen)
			continue
		}
		for j := 0; j < tsArrLen; j++ {
			// This is how monitoring API stores the int64 value.
			tsVal := tsArr[j].Points[0].Value.Value.(*mpb.TypedValue_Int64Value).Int64Value
			wantTsVal := wantTsValues[j]
			if tsVal != wantTsVal {
				t.Errorf("%s: Value got: %d, want: %d", prefix, tsVal, wantTsVal)
			}
		}
	}
}

// errRowDataCheck contains data for checking content of error storage.
type errRowDataCheck struct {
	errPrefix, errSuffix string
	rds                  []*RowData
}

// checkErrStorage checks content of error storage. For returned errors, we check prefix and suffix.
func checkErrStorage(t *testing.T, wantErrRdCheck []errRowDataCheck) {
	gotLen, wantLen := len(errStorage), len(wantErrRdCheck)
	if gotLen != wantLen {
		t.Errorf("number of reported errors: %d, want: %d", gotLen, wantLen)
		return
	}
	for i := 0; i < gotLen; i++ {
		prefix := fmt.Sprintf("%d-th reported error mismatch", i+1)
		errRd, wantErrRd := errStorage[i], wantErrRdCheck[i]
		errStr := errRd.err.Error()
		if errPrefix := wantErrRd.errPrefix; !strings.HasPrefix(errStr, errPrefix) {
			t.Errorf("%s: error got: %q, want: prefixed by %q", prefix, errStr, errPrefix)
		}
		if errSuffix := wantErrRd.errSuffix; !strings.HasSuffix(errStr, errSuffix) {
			t.Errorf("%s: error got: %q, want: suffiexd by %q", prefix, errStr, errSuffix)
		}
		if err := checkRowDataArr(errRd.rds, wantErrRd.rds); err != nil {
			t.Errorf("%s: RowData array mismatch: %v", prefix, err)
		}
	}
}

func checkRowDataArr(rds, wantRds []*RowData) error {
	rdLen, wantRdLen := len(rds), len(wantRds)
	if rdLen != wantRdLen {
		return fmt.Errorf("number row data got: %d, want: %d", rdLen, wantRdLen)
	}
	for i := 0; i < rdLen; i++ {
		if err := checkRowData(rds[i], wantRds[i]); err != nil {
			return fmt.Errorf("%d-th row data mismatch: %v", i+1, err)
		}
	}
	return nil
}

func checkRowData(rd, wantRd *RowData) error {
	if rd.View != wantRd.View {
		return fmt.Errorf("View got: %s, want: %s", rd.View.Name, wantRd.View.Name)
	}
	if rd.Start != wantRd.Start {
		return fmt.Errorf("Start got: %v, want: %v", rd.Start, wantRd.Start)
	}
	if rd.End != wantRd.End {
		return fmt.Errorf("End got: %v, want: %v", rd.End, wantRd.End)
	}
	if rd.Row != wantRd.Row {
		return fmt.Errorf("Row got: %v, want: %v", rd.Row, wantRd.Row)
	}
	return nil
}

// checkProjData checks all data passed to the bundler by bundler.Add().
func checkProjData(t *testing.T, wantProjData map[string][]*RowData) {
	wantProj := map[string]bool{}
	for proj := range wantProjData {
		wantProj[proj] = true
	}
	for proj := range projRds {
		if !wantProj[proj] {
			t.Errorf("project in exporter's project data not wanted: %s", proj)
		}
	}

	for proj, wantRds := range wantProjData {
		rds, ok := projRds[proj]
		if !ok {
			t.Errorf("wanted project not found in exporter's project data: %v", proj)
			continue
		}
		if err := checkRowDataArr(*rds, wantRds); err != nil {
			t.Errorf("RowData array mismatch for project %s: %v", proj, err)
		}
	}
}

// checkLabels checks data in labels.
func checkLabels(t *testing.T, prefix string, labels, wantLabels map[string]string) {
	for labelName, value := range labels {
		wantValue, ok := wantLabels[labelName]
		if !ok {
			t.Errorf("%s: label name in time series not wanted: %s", prefix, labelName)
			continue
		}
		if value != wantValue {
			t.Errorf("%s: value for label name %s got: %s, want: %s", prefix, labelName, value, wantValue)
		}
	}
	for wantLabelName := range wantLabels {
		if _, ok := labels[wantLabelName]; !ok {
			t.Errorf("%s: wanted label name not found in time series: %s", prefix, wantLabelName)
		}
	}
}
