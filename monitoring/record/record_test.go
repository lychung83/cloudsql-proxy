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
	"log"
	"os"
	"reflect"
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
	ipTypeTag    = octag.Tag{tag.IPTypeKey, "UNKNOWN"}
	termCodeTag1 = octag.Tag{tag.TermCodeKey, "OK"}
	termCodeTag2 = octag.Tag{tag.TermCodeKey, "dial error"}
	apiMethodTag = octag.Tag{tag.APIMethodKey, "Instances.Get()"}
	respCodeTag  = octag.Tag{tag.RespCodeKey, "200"}
)

type testData struct {
	desc string
	f    func() error
}

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

func TestMain(m *testing.M) {
	if err := metrics.Initialize(); err != nil {
		log.Fatalf("initializing metrics failed: %v", err)
	}

	oldRecordFunc := recordFunc
	recordFunc = mockRecordFunc
	defer func() {
		recordFunc = oldRecordFunc
	}()

	os.Exit(m.Run())
}

// TestAddTags tests behavior of AddTags.
func TestAddTags(t *testing.T) {
	if _, err := AddTags(bgCtx, instance1); err != nil {
		t.Errorf("AddTags(bgCtx, %s) returned error: %v, want success", instance1, err)
	}
	_, err := AddTags(bgCtx, instance3)
	if err == nil {
		t.Errorf("AddTags(bgCtx, %s) succeeded, want error", instance3)
	}
}

// TestRecordSuccess tests recording each metric succeeds.
func TestRecordSuccess(t *testing.T) {
	recData = nil
	ctx, err := AddTags(bgCtx, instance1)
	if err != nil {
		t.Fatalf("AddTags() with instance %s failed: %v", instance1, err)
	}

	for _, test := range []testData{
		{
			"Increment admin api count",
			func() error { return Increment(ctx, metrics.AdminAPIReqCount, apiMethodTag, respCodeTag) },
		},
		{
			"Set active connection number",
			func() error { return SetVal(ctx, metrics.ActiveConnNum, 5, ipTypeTag) },
		},
		{
			"Set connection latency",
			func() error { return SetDuration(ctx, metrics.ConnLatency, time.Second, ipTypeTag) },
		},
		{
			"Increment connection request count",
			func() error { return Increment(ctx, metrics.ConnReqCount) },
		},
		{
			"Increment termination code count",
			func() error { return Increment(ctx, metrics.TermCodeCount, termCodeTag1, ipTypeTag) },
		},
		{
			"Set received bytes count",
			func() error { return SetVal(ctx, metrics.RcvBytes, 10, ipTypeTag) },
		},
		{
			"Set sent bytes count",
			func() error { return SetVal(ctx, metrics.SentBytes, 20, ipTypeTag) },
		},
		{
			"Increment TLS config refresh throttle count",
			func() error { return Increment(ctx, metrics.TLSCfgRefreshThrottleCount) },
		},
		{
			"Set verion string",
			func() error { return SetStr(ctx, metrics.Version, "version 1.5") },
		},
	} {
		if err := test.f(); err != nil {
			t.Errorf("%s: failed with error: %v", test.desc, err)
		}
	}

	wantRecData := []recordData{
		{
			1,
			metrics.AdminAPIReqCount.String(),
			map[octag.Key]string{
				tag.ProjectKey:   project1,
				tag.RegionKey:    region1,
				tag.DatabaseKey:  database1,
				tag.APIMethodKey: "Instances.Get()",
				tag.RespCodeKey:  "200",
			},
		},
		{
			5,
			metrics.ActiveConnNum.String(),
			map[octag.Key]string{
				tag.ProjectKey:  project1,
				tag.RegionKey:   region1,
				tag.DatabaseKey: database1,
				tag.IPTypeKey:   "UNKNOWN",
			},
		},
		{
			1000,
			metrics.ConnLatency.String(),
			map[octag.Key]string{
				tag.ProjectKey:  project1,
				tag.RegionKey:   region1,
				tag.DatabaseKey: database1,
				tag.IPTypeKey:   "UNKNOWN",
			},
		},
		{
			1,
			metrics.ConnReqCount.String(),
			map[octag.Key]string{
				tag.ProjectKey:  project1,
				tag.RegionKey:   region1,
				tag.DatabaseKey: database1,
			},
		},
		{
			1,
			metrics.TermCodeCount.String(),
			map[octag.Key]string{
				tag.ProjectKey:  project1,
				tag.RegionKey:   region1,
				tag.DatabaseKey: database1,
				tag.IPTypeKey:   "UNKNOWN",
				tag.TermCodeKey: "OK",
			},
		},
		{
			10,
			metrics.RcvBytes.String(),
			map[octag.Key]string{
				tag.ProjectKey:  project1,
				tag.RegionKey:   region1,
				tag.DatabaseKey: database1,
				tag.IPTypeKey:   "UNKNOWN",
			},
		},
		{
			20,
			metrics.SentBytes.String(),
			map[octag.Key]string{
				tag.ProjectKey:  project1,
				tag.RegionKey:   region1,
				tag.DatabaseKey: database1,
				tag.IPTypeKey:   "UNKNOWN",
			},
		},
		{
			1,
			metrics.TLSCfgRefreshThrottleCount.String(),
			map[octag.Key]string{
				tag.ProjectKey:  project1,
				tag.RegionKey:   region1,
				tag.DatabaseKey: database1,
			},
		},
		{
			0,
			"cloudsql.googleapis.com/database/proxy/client/version",
			map[octag.Key]string{
				tag.ProjectKey:  project1,
				tag.RegionKey:   region1,
				tag.DatabaseKey: database1,
				tag.StrValKey:   "version 1.5",
			},
		},
	}

	if err := checkRecData(wantRecData); err != nil {
		t.Error(err)
	}
}

// TestInstanceWithNoRegion tests the behavior when instance without region is given.
func TestInstanceWithNoRegion(t *testing.T) {
	recData = nil
	ctx, err := AddTags(bgCtx, instance2)
	if err != nil {
		t.Fatalf("AddTags() with instance %s failed: %v", instance2, err)
	}

	if err := Increment(ctx, metrics.ConnReqCount); err != nil {
		t.Errorf("incremeting connection request count for instance %s failed: %v", instance2, err)
	}
	wantRecData := []recordData{
		{
			1,
			metrics.ConnReqCount.String(),
			map[octag.Key]string{
				tag.ProjectKey:  project1,
				tag.RegionKey:   "UNSPECIFIED",
				tag.DatabaseKey: database1,
			},
		},
	}

	if err := checkRecData(wantRecData); err != nil {
		t.Error(err)
	}
}

// TestRecordIgnore tests that record functions are no-op for contexts not propagated from
// AddTags.
func TestRecordIgnore(t *testing.T) {
	recData = nil
	for _, test := range []testData{
		{
			"Incrementing connection request count on background context",
			func() error { return Increment(bgCtx, metrics.ConnReqCount) },
		},
		{
			"Incrementing sent bytes count on backgroudn context",
			func() error { return Increment(bgCtx, metrics.SentBytes) },
		},
	} {
		if err := test.f(); err != nil {
			t.Errorf("%s: failed with error: %v", test.desc, err)
		}
	}

	if err := checkRecData(nil); err != nil {
		t.Error(err)
	}
}

// TestRecordError checks errors of record functions.
func TestRecordError(t *testing.T) {
	recData = nil
	ctx, err := AddTags(bgCtx, instance1)
	if err != nil {
		t.Fatalf("AddTags() with instance %s failed: %v", instance1, err)
	}

	for _, test := range []testData{
		{
			"recording invalid count metric",
			func() error { return Increment(ctx, metrics.SentBytes) },
		},
		{
			"recording invalid int metric",
			func() error { return SetVal(ctx, metrics.ConnReqCount, 1) },
		},
		{
			"recording invalid duration metric",
			func() error { return SetDuration(ctx, metrics.ConnReqCount, time.Second) },
		},
		{
			"recording invalid string metric",
			func() error { return SetStr(ctx, metrics.ActiveConnNum, "1") },
		},
		{
			"recording with invalid value",
			func() error {
				return Increment(ctx, metrics.AdminAPIReqCount, apiMethodTag, octag.Tag{tag.RespCodeKey, invalidStr})
			},
		},
		{
			"recording unexpected tag",
			func() error { return SetVal(ctx, metrics.SentBytes, 1, apiMethodTag, ipTypeTag) },
		},
		{
			"recording with missing tag",
			func() error { return Increment(ctx, metrics.TermCodeCount, ipTypeTag) },
		},
		{
			"recording invalid string value",
			func() error { return SetStr(ctx, metrics.Version, invalidStr) },
		},
	} {
		if test.f() == nil {
			t.Errorf("%s: expected error, but got nil", test.desc)
		}
	}

	if err := checkRecData(nil); err != nil {
		t.Error(err)
	}
}

// TestAddTagsOverride tests tag overriding behavior of AddTags.
func TestAddTagsOverride(t *testing.T) {
	recData = nil
	proj2Ctx, err := octag.New(bgCtx, octag.Insert(tag.ProjectKey, project2))
	if err != nil {
		t.Fatalf("setting project tag to bgCtx failed: %v", err)
	}
	ctx, err := AddTags(proj2Ctx, instance1)
	if err != nil {
		t.Fatalf("AddTags() with instance %s failed: %v", instance1, err)
	}

	if err := Increment(ctx, metrics.ConnReqCount); err != nil {
		t.Errorf("recording failed: %v", err)
	}

	wantRecData := []recordData{
		{
			1,
			metrics.ConnReqCount.String(),
			map[octag.Key]string{
				tag.ProjectKey:  project1,
				tag.RegionKey:   region1,
				tag.DatabaseKey: database1,
			},
		},
	}
	if err := checkRecData(wantRecData); err != nil {
		t.Error(err)
	}
}

// TestRecordTagOverride tests tag overriding behavior of record functions.
func TestRecordTagOverride(t *testing.T) {
	recData = nil
	ctx, err := AddTags(bgCtx, instance1)
	if err != nil {
		t.Fatalf("AddTags() with instance %s failed: %v", instance1, err)
	}

	for _, test := range []testData{
		{
			"overriding project ID",
			func() error { return Increment(ctx, metrics.ConnReqCount, octag.Tag{tag.ProjectKey, project2}) },
		},
		{
			"overriding termination code",
			func() error {
				return Increment(ctx, metrics.TermCodeCount, termCodeTag1, ipTypeTag, termCodeTag2)
			},
		},
	} {
		if err := test.f(); err != nil {
			t.Errorf("%s: failed with error: %v", test.desc, err)
		}
	}

	wantRecData := []recordData{
		{
			1,
			metrics.ConnReqCount.String(),
			map[octag.Key]string{
				tag.ProjectKey:  project2,
				tag.RegionKey:   region1,
				tag.DatabaseKey: database1,
			},
		},
		{
			1,
			metrics.TermCodeCount.String(),
			map[octag.Key]string{
				tag.ProjectKey:  project1,
				tag.RegionKey:   region1,
				tag.DatabaseKey: database1,
				tag.IPTypeKey:   "UNKNOWN",
				tag.TermCodeKey: "dial error",
			},
		},
	}
	if err := checkRecData(wantRecData); err != nil {
		t.Error(err)
	}
}

// checkRecData checks the content of recData.
func checkRecData(wantRecData []recordData) error {
	if !reflect.DeepEqual(recData, wantRecData) {
		return fmt.Errorf("recData = %+v, want %+v", recData, wantRecData)
	}
	return nil
}
