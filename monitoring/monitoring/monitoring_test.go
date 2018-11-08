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
	"reflect"
	"testing"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/exporter/stackdriver"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/tag"
	"go.opencensus.io/stats/view"
	octag "go.opencensus.io/tag"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

// TestGetProjectID tests the operation of getProjectID.
func TestGetProjectID(t *testing.T) {
	dataList := []struct {
		metricName string
		tags       []octag.Tag
		wantProjID string
		wantErr    bool
		// wantIgnore tells whether getProjectID() should ignore the given row data or not.
		wantIgnore bool
	}{{
		metricName: "invalid_metric",
		tags:       []octag.Tag{{tag.ProjectKey, "project-1"}},
		wantProjID: "",
		wantErr:    true,
		wantIgnore: true,
	}, {
		metricName: "cloudsql.googleapis.com/database/proxy/client/metric_1",
		tags:       []octag.Tag{{tag.ProjectKey, "project-1"}},
		wantProjID: "project-1",
		wantErr:    false,
		wantIgnore: false,
	}, {
		metricName: "cloudsql.googleapis.com/database/proxy/client/metric_1",
		tags:       nil,
		wantProjID: "",
		wantErr:    true,
		wantIgnore: false,
	}}

	for _, data := range dataList {
		rd := &stackdriver.RowData{
			View: &view.View{Name: data.metricName},
			Row:  &view.Row{Tags: data.tags},
		}
		projID, err := getProjectID(rd)
		if (err != nil) != data.wantErr {
			t.Errorf("getProjectID(%+v) returned error: %v, wantErr: %v", rd, err, data.wantErr)
			continue
		}
		gotIgnore := err == stackdriver.ErrRowDataNotApplicable
		if gotIgnore != data.wantIgnore {
			t.Errorf("getProjectID(%+v) gotIgnore: %v, wantIgnore: %v", rd, gotIgnore, data.wantIgnore)
			continue
		}
		if projID != data.wantProjID {
			t.Errorf("getProjectID(%+v) = %s, want %s", rd, projID, data.wantProjID)
		}
	}
}

// TestMakeResource tests the operation of makeResource.
func TestMakeResource(t *testing.T) {
	dataList := []struct {
		tags    []octag.Tag
		wantRsc *mrpb.MonitoredResource
		wantErr bool
	}{{
		tags: []octag.Tag{
			{tag.ProjectKey, "project-1"},
			{tag.RegionKey, "us-central1"},
			{tag.DatabaseKey, "instance-1"},
		},
		wantRsc: &mrpb.MonitoredResource{
			Type: "cloudsqlproxy",
			Labels: map[string]string{
				projectName:  "project-1",
				regionName:   "us-central1",
				databaseName: "instance-1",
				"uuid":       "9767319e-7726-42d8-9e28-5c8ddb2d54c2",
			},
		},
		wantErr: false,
	}, {
		tags:    nil,
		wantRsc: nil,
		wantErr: true,
	}}

	oldUUIDVal := uuidVal
	uuidVal = "9767319e-7726-42d8-9e28-5c8ddb2d54c2"
	defer func() {
		uuidVal = oldUUIDVal
	}()

	for _, data := range dataList {
		rd := &stackdriver.RowData{Row: &view.Row{Tags: data.tags}}
		rsc, err := makeResource(rd)
		if (err != nil) != data.wantErr {
			t.Errorf("makeResource(%+v) returned error: %v, wantErr: %v", rd, err, data.wantErr)
			continue
		}
		if !reflect.DeepEqual(rsc, data.wantRsc) {
			t.Errorf("makeResource(%+v) = %+v, want %+v", rd, rsc, data.wantRsc)
		}
	}
}

// TestIsValueString tests the operation of isValueString.
func TestIsValueString(t *testing.T) {
	testData := []struct {
		tags    []octag.Tag
		wantStr string
		wantOK  bool
	}{{
		tags: []octag.Tag{
			{tag.ProjectKey, "project-1"},
			{tag.RegionKey, "us-central1"},
			{tag.DatabaseKey, "instance-1"},
		},
		wantStr: "",
		wantOK:  false,
	}, {
		tags: []octag.Tag{
			{tag.ProjectKey, "project-1"},
			{tag.RegionKey, "us-central1"},
			{tag.DatabaseKey, "instance-1"},
			{tag.StrValKey, "version 1"},
		},
		wantStr: "version 1",
		wantOK:  true,
	}}

	for _, data := range testData {
		rd := &stackdriver.RowData{Row: &view.Row{Tags: data.tags}}
		str, ok, err := isValueString(rd)
		if str != data.wantStr || ok != data.wantOK || err != nil {
			t.Errorf("isValueString(%+v) = (%s, %t, %v), want (%s, %t, nil)", rd, str, ok, err, data.wantStr, data.wantOK)
		}
	}
}
