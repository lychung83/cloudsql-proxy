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
	"reflect"
	"testing"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/exporter/stackdriver"

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
		metricName: metric3Name,
		tags:       tags1,
		wantProjID: "",
		wantErr:    true,
		wantIgnore: true,
	}, {
		metricName: metric1Name,
		tags:       tags1,
		wantProjID: project1,
		wantErr:    false,
		wantIgnore: false,
	}, {
		metricName: metric1Name,
		tags:       tags3,
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

		reportPrefix := fmt.Sprintf("calling getProjectID() with a row data with metric name %s and tags %v", data.metricName, data.tags)
		if projID != data.wantProjID {
			t.Errorf("%s returned %s, want %s", reportPrefix, projID, data.wantProjID)
		}
		if (err != nil) != data.wantErr {
			t.Errorf("%s returned error: %v, wantErr: %v", reportPrefix, err, data.wantErr)
		}
		gotIgnore := err == stackdriver.ErrRowDataNotApplicable
		if gotIgnore != data.wantIgnore {
			t.Errorf("%s gotIgnore: %v, wantIgnore: %v", reportPrefix, gotIgnore, data.wantIgnore)
		}
	}
}

// TestMakeResource tests the operation of makeResource.
func TestMakeResource(t *testing.T) {
	monRsc := &mrpb.MonitoredResource{
		Type: "cloudsqlproxy",
		Labels: map[string]string{
			projectName:  project1,
			regionName:   region1,
			databaseName: database1,
			"uuid":       uuidVal,
		},
	}

	dataList := []struct {
		tags    []octag.Tag
		wantRsc *mrpb.MonitoredResource
		wantErr bool
	}{{
		tags:    tags1,
		wantRsc: monRsc,
		wantErr: false,
	}, {
		tags:    tags3,
		wantRsc: nil,
		wantErr: true,
	}}

	for _, data := range dataList {
		rd := &stackdriver.RowData{Row: &view.Row{Tags: data.tags}}
		rsc, err := makeResource(rd)

		reportPrefix := fmt.Sprintf("calling makeResrouce() with a row data with tags %v", data.tags)
		if !reflect.DeepEqual(rsc, data.wantRsc) {
			t.Errorf("%s returned %#v, want %#v", reportPrefix, rsc, data.wantRsc)
		}
		if (err != nil) != data.wantErr {
			t.Errorf("%s returned error: %v, wantErr: %v", reportPrefix, err, data.wantErr)
		}
	}
}
