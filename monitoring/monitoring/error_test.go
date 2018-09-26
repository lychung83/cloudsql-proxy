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

package monitoring

import (
	"fmt"
	"testing"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/logging"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/exporter/stackdriver"
)

// This file contains tests of facilities in error.go

// errLog saves all logged errors.
var errLog []string

func init() {
	logging.Errorf = func(format string, args ...interface{}) {
		errLog = append(errLog, fmt.Sprintf(format, args...))
	}
}

// TestAllError tests all error tests in this file.
func TestAllError(t *testing.T) {
	testData := []struct {
		name string
		test func(*testing.T)
	}{
		{"EmptyErrorReport", testEmptyErrorReport},
		{"ErrorReport", testErrorReport},
		{"ErrorReportTimestamp", testErrorReportTimestamp},
	}

	for _, data := range testData {
		errLog = nil
		t.Run(data.name, data.test)
	}
}

// testEmptyErrorReport tests that no error is logged by empty errLogData object.
func testEmptyErrorReport(t *testing.T) {
	data := &errLogData{start: start}
	data.report(now)
	if errLogLen := len(errLog); errLogLen != 0 {
		t.Errorf("want no error logs, but got: %v", errLog)
	}
}

// testErrorReport tests format of error report.
func testErrorReport(t *testing.T) {
	eh := newErrorHandler(0)
	go eh.run()

	// Generate some errors.
	eh.expErrHandler(err1, &stackdriver.RowData{View: view1, Row: row1})
	eh.expErrHandler(err2, &stackdriver.RowData{View: view2, Row: row2}, &stackdriver.RowData{View: view1, Row: row2})
	eh.expErrHandler(err1, &stackdriver.RowData{View: view1, Row: row2}, &stackdriver.RowData{View: view2, Row: row2})
	eh.stop()

	wantErrLog := []string{
		"stackdriver monitoring error: beginning of the report",
		// This line includes timestamp. This part is dynamically generated, so we do not
		// test here.
		"",
		"stackdriver monitoring error: 3 errors are reported and export of 5 data points failed to stackdriver",
		"stackdriver monitoring error: error messages are:",
		"stackdriver monitoring error:     (repeated 2 times) error message 1",
		"stackdriver monitoring error:     error message 2",
		"stackdriver monitoring error: affected instances are:",
		"stackdriver monitoring error:     project-1:us-central1:instance-1: export of 1 data points failed in this instance",
		"stackdriver monitoring error:     project-2:us-east1:instance-2: export of 4 data points failed in this instance",
		"stackdriver monitoring error: affected metrics are:",
		"stackdriver monitoring error:     cloudsql.googleapis.com/database/proxy/client/metric_1: export of 3 data points failed in this metric",
		"stackdriver monitoring error:     cloudsql.googleapis.com/database/proxy/client/metric_2: export of 2 data points failed in this metric",
		"stackdriver monitoring error: end of the report",
	}

	errLogLen, wantErrLogLen := len(errLog), len(wantErrLog)
	if errLogLen != wantErrLogLen {
		t.Fatalf("number of error log lines got %d, want: %d", errLogLen, wantErrLogLen)
	}
	for i := 0; i < errLogLen; i++ {
		// Skip timestemp.
		if i == 1 {
			continue
		}
		msg, wantMsg := errLog[i], wantErrLog[i]
		if msg != wantMsg {
			t.Errorf("%d-th error log line got %q, want: %q", i+1, msg, wantMsg)
		}
	}
}

// testErrorReportTimestamp tests timestamp format of the error report.
func testErrorReportTimestamp(t *testing.T) {
	data := &errLogData{
		start:     start,
		errNum:    1,
		rowNum:    1,
		errMsgs:   map[string]int{errMsg1: 1},
		instances: map[string]int{fmt.Sprintf("%s:%s:%s", project1, region1, database1): 1},
		metrics:   map[string]int{metric1Name: 1},
	}
	data.report(now)

	if len(errLog) < 2 {
		t.Fatalf("error log does not contain time stamp")
	}
	timestamp := errLog[1]
	wantTimestamp := fmt.Sprintf("stackdriver monitoring error: span of the report: from %s to %s", startStr, nowStr)
	if timestamp != wantTimestamp {
		t.Errorf("timestamp got: %q, want: %q", timestamp, wantTimestamp)
	}
}
