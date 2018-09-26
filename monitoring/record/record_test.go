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

package record

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/metrics"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/tag"

	"go.opencensus.io/stats"
	octag "go.opencensus.io/tag"
)

const (
	project1   = "project-1"
	project2   = "project-2"
	region1    = "us-central1"
	database1  = "instance-1"
	instance1  = "project-1:us-central1:instance-1"
	instance2  = "project-1:instance-1"
	instance3  = "project-1:us-central:1invalid\tinstance"
	invalidStr = "invalid\tvalue"
)

var (
	bgCtx        = context.Background()
	termCodeTag1 = octag.Tag{tag.TermCodeKey, "OK"}
	termCodeTag2 = octag.Tag{tag.TermCodeKey, "dial error"}
	apiMethodTag = octag.Tag{tag.APIMethodKey, "Instances.Get()"}
	respCodeTag  = octag.Tag{tag.RespCodeKey, "200"}
)

// recordData saves data passed from one call of recordFunc.
type recordData struct {
	val  float64
	name string
	tags map[octag.Key]string
}

// recData contains all data passed to recordMeasurement.
var recData []recordData

// mockRecordMs is the mock recordMeasurement that saves all passed data to recData.
func mockRecordFunc(ctx context.Context, ms ...stats.Measurement) {
	tags := map[octag.Key]string{}
	putTag := func(key octag.Key, val string) {
		tags[key] = val
	}
	octag.DecodeEach(octag.Encode(octag.FromContext(ctx)), putTag)
	recData = append(recData, recordData{ms[0].Value(), ms[0].Measure().Name(), tags})
}

func init() {
	recordFunc = mockRecordFunc
}

// TestRecordAll tests all record tests in this file.
func TestRecordAll(t *testing.T) {
	if err := metrics.Initialize(); err != nil {
		t.Fatalf("initializing metrics failed: %v", err)
	}

	testData := []struct {
		name string
		test func(*testing.T)
	}{
		{"GetContext", testGetContext},
		{"RecordSuccess", testRecordSuccess},
		{"InstanceWithNoRegion", testInstanceWithNoRegion},
		{"IPType", testIPType},
		{"RecordIgnore", testRecordIgnore},
		{"RecordError", testRecordError},
		{"GetContextTagOverride", testGetContextTagOverride},
		{"RecordTagOverride", testRecordTagOverride},
	}

	for _, data := range testData {
		recData = nil
		t.Run(data.name, data.test)
	}
}

// testGetContext tests behavior of GetContext.
func testGetContext(t *testing.T) {
	if _, err := GetContext(bgCtx, instance1); err != nil {
		t.Errorf("GetContext(bgCtx, %s) returned error: %v, want success", instance1, err)
	}
	_, err := GetContext(bgCtx, instance3)
	if err == nil {
		t.Errorf("GetContext(bgCtx, %s) succeeded, want error", instance3)
	}
}

// testRecordSuccess tests recording each metric succeeds.
func testRecordSuccess(t *testing.T) {
	ctx, err := GetContext(bgCtx, instance1)
	if err != nil {
		t.Fatalf("GetContext() with instance %s failed: %v", instance1, err)
	}

	for i, err := range []error{
		Count(ctx, metrics.AdminAPIReqCount, apiMethodTag, respCodeTag),
		Int(ctx, metrics.ActiveConnNum, 5),
		Duration(ctx, metrics.ConnLatency, time.Second),
		Count(ctx, metrics.ConnReqCount),
		Count(ctx, metrics.TermCodeCount, termCodeTag1),
		Int(ctx, metrics.RcvBytes, 10),
		Int(ctx, metrics.SentBytes, 20),
		Count(ctx, metrics.TLSCfgRefreshThrottleCount),
		Str(ctx, metrics.Version, "version 1.5"),
	} {
		if err != nil {
			t.Errorf("%d-th record call failed: %v", i+1, err)
		}
	}

	wantRecData := []recordData{{
		1,
		"cloudsql.googleapis.com/database/proxy/client/admin_api_request_count",
		map[octag.Key]string{
			tag.ProjectKey:   project1,
			tag.RegionKey:    region1,
			tag.DatabaseKey:  database1,
			tag.APIMethodKey: "Instances.Get()",
			tag.RespCodeKey:  "200",
		}}, {
		5,
		"cloudsql.googleapis.com/database/proxy/client/connection/active",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
			tag.IPTypeKey:   "UNKNOWN",
		}}, {
		1000,
		"cloudsql.googleapis.com/database/proxy/client/connection/latencies",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
			tag.IPTypeKey:   "UNKNOWN",
		}}, {
		1,
		"cloudsql.googleapis.com/database/proxy/client/connection/request_count",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
		}}, {
		1,
		"cloudsql.googleapis.com/database/proxy/client/connection/termination_code_count",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
			tag.IPTypeKey:   "UNKNOWN",
			tag.TermCodeKey: "OK",
		}}, {
		10,
		"cloudsql.googleapis.com/database/proxy/client/received_bytes_count",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
			tag.IPTypeKey:   "UNKNOWN",
		}}, {
		20,
		"cloudsql.googleapis.com/database/proxy/client/sent_bytes_count",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
			tag.IPTypeKey:   "UNKNOWN",
		}}, {
		1,
		"cloudsql.googleapis.com/database/proxy/client/throttled_TLS_config_refresh_count",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
		}}, {
		0,
		"cloudsql.googleapis.com/database/proxy/client/version",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
			tag.StrValKey:   "version 1.5",
		}},
	}

	if err := checkRecData(wantRecData); err != nil {
		t.Error(err)
	}
}

// testInstanceWithNoRegion tests the behavior when instance without region is given.
func testInstanceWithNoRegion(t *testing.T) {
	ctx, err := GetContext(bgCtx, instance2)
	if err != nil {
		t.Fatalf("GetContext() with instance %s failed: %v", instance2, err)
	}

	if err := Count(ctx, metrics.ConnReqCount); err != nil {
		t.Errorf("recording failed: %v", err)
	}
	wantRecData := []recordData{{
		1,
		"cloudsql.googleapis.com/database/proxy/client/connection/request_count",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   "UNSPECIFIED",
			tag.DatabaseKey: database1,
		}},
	}

	if err := checkRecData(wantRecData); err != nil {
		t.Error(err)
	}
}

// testSetIPType tests the operation of IP type.
func testIPType(t *testing.T) {
	ctx, err := GetContext(bgCtx, instance1)
	if err != nil {
		t.Fatalf("GetContext() with instance %s failed: %v", instance1, err)
	}

	if err := Int(ctx, metrics.RcvBytes, 1); err != nil {
		t.Errorf("first record call failed: %v", err)
	}
	if ctx, err = octag.New(ctx, octag.Update(tag.IPTypeKey, "PRIMARY")); err != nil {
		t.Errorf("setting IP type failed: %v", err)
	}
	if err := Int(ctx, metrics.RcvBytes, 1); err != nil {
		t.Errorf("second record call failed: %v", err)
	}

	wantRecData := []recordData{{
		1,
		"cloudsql.googleapis.com/database/proxy/client/received_bytes_count",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
			tag.IPTypeKey:   "UNKNOWN",
		}}, {
		1,
		"cloudsql.googleapis.com/database/proxy/client/received_bytes_count",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
			tag.IPTypeKey:   "PRIMARY",
		}},
	}

	if err := checkRecData(wantRecData); err != nil {
		t.Error(err)
	}
}

// testRecordIgnore tests that record functions are no-op for contexts not propagated from
// GetContext.
func testRecordIgnore(t *testing.T) {
	for i, err := range []error{
		Count(bgCtx, metrics.ConnReqCount),
		Count(bgCtx, metrics.SentBytes),
	} {
		if err != nil {
			t.Errorf("%d-th record call failed: %v", i+1, err)
		}
	}

	if err := checkRecData(nil); err != nil {
		t.Error(err)
	}
}

// testRecordError checks errors of record functions.
func testRecordError(t *testing.T) {
	ctx, err := GetContext(bgCtx, instance1)
	if err != nil {
		t.Fatalf("GetContext() with instance %s failed: %v", instance1, err)
	}

	dataList := []struct {
		err          error
		desc         string
		errStrPrefix string
	}{
		{Count(ctx, metrics.SentBytes), "recording invalid count metric", "invalid metric"},
		{Int(ctx, metrics.ConnReqCount, 1), "recording invalid int metric", "invalid metric"},
		{Duration(ctx, metrics.ConnReqCount, time.Second), "recording invalid duration metric", "invalid metric"},
		{Str(ctx, metrics.ActiveConnNum, "1"), "recording invalid string metric", "invalid metric"},
		{Count(ctx, metrics.AdminAPIReqCount, apiMethodTag, octag.Tag{tag.RespCodeKey, invalidStr}), "recording with invalid value", "creating context with input tags failed"},
		{Int(ctx, metrics.SentBytes, 1, apiMethodTag), "recording invalid tag", "invalid tag for the metric provided"},
		{Count(ctx, metrics.TermCodeCount), "recording with missing tag", "metric expects a tag with following key, but not provided"},
		{Str(ctx, metrics.Version, invalidStr), "recording invalid string value", "setting string value failed"},
	}

	for _, data := range dataList {
		if data.err == nil {
			t.Errorf("%s: expected error, but got nil", data.desc)
			continue
		}
		if !strings.HasPrefix(data.err.Error(), data.errStrPrefix) {
			t.Errorf("%s: error got: %q, want: prefixed by %q", data.desc, data.err, data.errStrPrefix)
		}
	}

	if err := checkRecData(nil); err != nil {
		t.Error(err)
	}
}

// testGetContextTagOverride tests tag overriding behavior of GetContext.
func testGetContextTagOverride(t *testing.T) {
	proj2Ctx, err := octag.New(bgCtx, octag.Insert(tag.ProjectKey, project2))
	if err != nil {
		t.Fatalf("setting project tag to bgCtx failed: %v", err)
	}
	ctx, err := GetContext(proj2Ctx, instance1)
	if err != nil {
		t.Fatalf("GetContext() with instance %s failed: %v", instance1, err)
	}

	if err := Count(ctx, metrics.ConnReqCount); err != nil {
		t.Errorf("recording failed: %v", err)
	}

	wantRecData := []recordData{{
		1,
		"cloudsql.googleapis.com/database/proxy/client/connection/request_count",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
		}},
	}
	if err := checkRecData(wantRecData); err != nil {
		t.Error(err)
	}
}

// testRecordTagOverride tests tag overriding behavior of record functions.
func testRecordTagOverride(t *testing.T) {
	ctx, err := GetContext(bgCtx, instance1)
	if err != nil {
		t.Fatalf("GetContext() with instance %s failed: %v", instance1, err)
	}

	for i, err := range []error{
		Count(ctx, metrics.ConnReqCount, octag.Tag{tag.ProjectKey, project2}),
		Count(ctx, metrics.TermCodeCount, termCodeTag1, termCodeTag2),
		Str(ctx, metrics.Version, "value1", octag.Tag{tag.StrValKey, "value2"}),
	} {
		if err != nil {
			t.Errorf("%d-th record call failed: %v", i+1, err)
		}
	}

	wantRecData := []recordData{{
		1,
		"cloudsql.googleapis.com/database/proxy/client/connection/request_count",
		map[octag.Key]string{
			tag.ProjectKey:  project2,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
		}}, {
		1,
		"cloudsql.googleapis.com/database/proxy/client/connection/termination_code_count",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
			tag.IPTypeKey:   "UNKNOWN",
			tag.TermCodeKey: "dial error",
		}}, {
		0,
		"cloudsql.googleapis.com/database/proxy/client/version",
		map[octag.Key]string{
			tag.ProjectKey:  project1,
			tag.RegionKey:   region1,
			tag.DatabaseKey: database1,
			tag.StrValKey:   "value1",
		}},
	}
	if err := checkRecData(wantRecData); err != nil {
		t.Error(err)
	}
}

// checkRecData checks the content of recData.
func checkRecData(wantRecData []recordData) error {
	recLen, wantRecLen := len(recData), len(wantRecData)
	if recLen != wantRecLen {
		return fmt.Errorf("numer of record data: %d, want: %d", recLen, wantRecLen)
	}

	var errs []error
	for i := 0; i < recLen; i++ {
		rec, wantRec := recData[i], wantRecData[i]
		prefix := fmt.Sprintf("%d-th record data mismatch", i+1)
		if rec.val != wantRec.val {
			errs = append(errs, fmt.Errorf("%s: val: %v, want: %v", prefix, rec.val, wantRec.val))
		}
		if rec.name != wantRec.name {
			errs = append(errs, fmt.Errorf("%s: name: %v, want: %v", prefix, rec.name, wantRec.name))
		}
		if !reflect.DeepEqual(rec.tags, wantRec.tags) {
			errs = append(errs, fmt.Errorf("%s: tags: %#v, want: %#v", prefix, rec.tags, wantRec.tags))
		}
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return fmt.Errorf("multiple errors: %q", errs)
	}
}
