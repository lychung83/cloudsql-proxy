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
	"fmt"
	"testing"

	"go.opencensus.io/stats/view"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

// This file contains actual tests.

// TestAll runs all tests defined in this file.
func TestAll(t *testing.T) {
	testData := []struct {
		name string
		test func(t *testing.T)
	}{
		{"ProjectClassifyNoError", testProjectClassifyNoError},
		{"ProjectClassifyError", testProjectClassifyError},
		{"DefaultProjectClassify", testDefaultProjectClassify},
		{"UploadNoError", testUploadNoError},
		{"UploadTimeSeriesMakeError", testUploadTimeSeriesMakeError},
		{"UploadWithMetricClientError", testUploadWithMetricClientError},
		{"MakeResource", testMakeResource},
		{"MakeLabel", testMakeLabel},
	}

	for _, data := range testData {
		run := func(t *testing.T) {
			testDataInit()
			data.test(t)
		}
		t.Run(data.name, run)
	}
}

// testProjectClassifyNoError tests that exporter can recognize and distribute incoming data by
// its project.
func testProjectClassifyNoError(t *testing.T) {
	viewData1 := &view.Data{
		View:  view1,
		Start: startTime1,
		End:   endTime1,
		Rows:  []*view.Row{view1row1, view1row2},
	}
	viewData2 := &view.Data{
		View:  view2,
		Start: startTime2,
		End:   endTime2,
		Rows:  []*view.Row{view2row1},
	}

	getProjectID := func(rd *RowData) (string, error) {
		switch rd.Row {
		case view1row1, view2row1:
			return project1, nil
		case view1row2:
			return project2, nil
		default:
			return "", unrecognizedDataError
		}
	}

	exp := newTestExp(t, &Options{GetProjectID: getProjectID})
	exp.ExportView(viewData1)
	exp.ExportView(viewData2)

	wantRowData := map[string][]*RowData{
		project1: []*RowData{
			{view1, startTime1, endTime1, view1row1},
			{view2, startTime2, endTime2, view2row1},
		},
		project2: []*RowData{
			{view1, startTime1, endTime1, view1row2},
		},
	}
	checkErrStorage(t, nil)
	checkProjData(t, wantRowData)
}

// testProjectClassifyError tests that exporter can properly handle errors while classifying
// incoming data by its project.
func testProjectClassifyError(t *testing.T) {
	viewData1 := &view.Data{
		View:  view1,
		Start: startTime1,
		End:   endTime1,
		Rows:  []*view.Row{view1row1, view1row2},
	}
	viewData2 := &view.Data{
		View:  view2,
		Start: startTime2,
		End:   endTime2,
		Rows:  []*view.Row{view2row1, view2row2},
	}

	getProjectID := func(rd *RowData) (string, error) {
		switch rd.Row {
		case view1row1, view2row2:
			return project1, nil
		case view1row2:
			return "", RowDataNotApplicableError
		case view2row1:
			return "", invalidDataError
		default:
			return "", unrecognizedDataError
		}
	}

	exp := newTestExp(t, &Options{GetProjectID: getProjectID})
	exp.ExportView(viewData1)
	exp.ExportView(viewData2)

	wantErrRdCheck := []errRowDataCheck{
		{
			errPrefix: "failed to get project ID",
			errSuffix: invalidDataErrStr,
			rds:       []*RowData{{view2, startTime2, endTime2, view2row1}},
		},
	}
	wantRowData := map[string][]*RowData{
		project1: []*RowData{
			{view1, startTime1, endTime1, view1row1},
			{view2, startTime2, endTime2, view2row2},
		},
	}
	checkErrStorage(t, wantErrRdCheck)
	checkProjData(t, wantRowData)
}

// testDefaultProjectClassify tests that defaultGetProjectID classifies RowData by tag with key name
// "project_id".
func testDefaultProjectClassify(t *testing.T) {
	viewData1 := &view.Data{
		View:  view1,
		Start: startTime1,
		End:   endTime1,
		Rows:  []*view.Row{view1row1},
	}
	viewData2 := &view.Data{
		View:  view3,
		Start: startTime2,
		End:   endTime2,
		Rows:  []*view.Row{view3row1, view3row2},
	}
	viewData3 := &view.Data{
		View:  view2,
		Start: startTime3,
		End:   endTime3,
		Rows:  []*view.Row{view2row1, view2row2},
	}
	viewData4 := &view.Data{
		View:  view3,
		Start: startTime4,
		End:   endTime4,
		Rows:  []*view.Row{view3row3},
	}

	exp := newTestExp(t, &Options{})
	exp.ExportView(viewData1)
	exp.ExportView(viewData2)
	exp.ExportView(viewData3)
	exp.ExportView(viewData4)

	checkErrStorage(t, nil)
	// RowData in viewData1 and viewData3 has no project ID tag, so ignored.
	wantRowData := map[string][]*RowData{
		project1: []*RowData{
			{view3, startTime2, endTime2, view3row1},
			{view3, startTime4, endTime4, view3row3},
		},
		project2: []*RowData{
			{view3, startTime2, endTime2, view3row2},
		},
	}
	checkProjData(t, wantRowData)
}

// testUploadNoError tests that all RowData objects passed to uploadRowData() are grouped by
// slice of length MaxTimeSeriesPerUpload, and passed to createTimeSeries().
func testUploadNoError(t *testing.T) {
	pd := newTestProjData(t, &Options{})
	rd := []*RowData{
		{view1, startTime1, endTime1, view1row1},
		{view1, startTime1, endTime1, view1row2},
		{view1, startTime1, endTime1, view1row3},
		{view2, startTime2, endTime2, view2row1},
		{view2, startTime2, endTime2, view2row2},
	}
	pd.uploadRowData(rd)

	checkErrStorage(t, nil)
	wantClData := [][]int64{
		{1, 2, 3},
		{4, 5},
	}
	checkMetricClient(t, wantClData)
}

// testUploadTimeSeriesMakeError tests that errors while creating time series are properly handled.
func testUploadTimeSeriesMakeError(t *testing.T) {
	makeResource := func(rd *RowData) (*mrpb.MonitoredResource, error) {
		if rd.Row == view1row2 {
			return nil, invalidDataError
		}
		return defaultMakeResource(rd)
	}
	pd := newTestProjData(t, &Options{MakeResource: makeResource})
	rd := []*RowData{
		{view1, startTime1, endTime1, view1row1},
		{view1, startTime1, endTime1, view1row2},
		{view1, startTime1, endTime1, view1row3},
		// This row data is invalid, so it will trigger inconsistent data error.
		{view2, startTime2, endTime2, invalidRow},
		{view2, startTime2, endTime2, view2row1},
		{view2, startTime2, endTime2, view2row2},
	}
	pd.uploadRowData(rd)

	wantErrRdCheck := []errRowDataCheck{
		{
			errPrefix: "failed to construct resource",
			errSuffix: invalidDataErrStr,
			rds:       []*RowData{{view1, startTime1, endTime1, view1row2}},
		}, {
			errPrefix: "inconsistent data found in view",
			errSuffix: metric2name,
			rds:       []*RowData{{view2, startTime2, endTime2, invalidRow}},
		},
	}
	checkErrStorage(t, wantErrRdCheck)

	wantClData := [][]int64{
		{1, 3, 4},
		{5},
	}
	checkMetricClient(t, wantClData)
}

// testUploadTimeSeriesMakeError tests that exporter can handle error on metric client's time
// series create RPC call.
func testUploadWithMetricClientError(t *testing.T) {
	pd := newTestProjData(t, &Options{})
	timeSeriesResults = append(timeSeriesResults, invalidDataError)
	rd := []*RowData{
		{view1, startTime1, endTime1, view1row1},
		{view1, startTime1, endTime1, view1row2},
		{view1, startTime1, endTime1, view1row3},
		{view2, startTime2, endTime2, view2row1},
		{view2, startTime2, endTime2, view2row2},
	}
	pd.uploadRowData(rd)

	wantErrRdCheck := []errRowDataCheck{
		{
			errPrefix: "RPC call to create time series failed",
			errSuffix: invalidDataErrStr,
			rds: []*RowData{
				{view1, startTime1, endTime1, view1row1},
				{view1, startTime1, endTime1, view1row2},
				{view1, startTime1, endTime1, view1row3},
			},
		},
	}
	checkErrStorage(t, wantErrRdCheck)

	wantClData := [][]int64{
		{1, 2, 3},
		{4, 5},
	}
	checkMetricClient(t, wantClData)
}

// testMakeResource tests that exporter can create monitored resource dynamically.
func testMakeResource(t *testing.T) {
	makeResource := func(rd *RowData) (*mrpb.MonitoredResource, error) {
		switch rd.Row {
		case view1row1:
			return resource1, nil
		case view1row2:
			return resource2, nil
		default:
			return nil, unrecognizedDataError
		}
	}
	pd := newTestProjData(t, &Options{MakeResource: makeResource})
	rd := []*RowData{
		{view1, startTime1, endTime1, view1row1},
		{view1, startTime1, endTime1, view1row2},
	}
	pd.uploadRowData(rd)
	checkErrStorage(t, nil)
	checkMetricClient(t, [][]int64{{1, 2}})

	tsArr := timeSeriesReqs[0].TimeSeries
	for i, wantResource := range []*mrpb.MonitoredResource{resource1, resource2} {
		if resource := tsArr[i].Resource; resource != wantResource {
			t.Errorf("%d-th time series resource got: %#v, want: %#v", i+1, resource, wantResource)
		}
	}
}

// testMakeLabel tests that exporter can correctly handle label manipulation process, including
// merging default label with tags, and removing unexported labels.
func testMakeLabel(t *testing.T) {
	opts := &Options{
		DefaultLabels: map[string]string{
			label1name: value7,
			label4name: value8,
		},
		UnexportedLabels: []string{label3name, label5name},
	}
	pd := newTestProjData(t, opts)
	rd := []*RowData{
		{view1, startTime1, endTime1, view1row1},
		{view2, startTime2, endTime2, view2row1},
	}
	pd.uploadRowData(rd)
	checkErrStorage(t, nil)
	checkMetricClient(t, [][]int64{{1, 4}})

	wantLabels1 := map[string]string{
		label1name: value7,
		label4name: value8,
	}
	wantLabels2 := map[string]string{
		// Default value for key1 is suppressed, and value defined in tag of view2row1 is
		// used.
		label1name: value1,
		label2name: value2,
		label4name: value8,
	}
	tsArr := timeSeriesReqs[0].TimeSeries
	for i, wantLabels := range []map[string]string{wantLabels1, wantLabels2} {
		prefix := fmt.Sprintf("%d-th time series labels mismatch", i+1)
		checkLabels(t, prefix, tsArr[i].Metric.Labels, wantLabels)
	}
}
