// Copyright 2015 Google Inc. All Rights Reserved.
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

package util

import "testing"

func TestNewInstance(t *testing.T) {
	table := []struct {
		in, wantProj, wantRegion, wantInstance string
		wantErr                                bool
	}{
		{"proj:region:my-db", "proj", "region", "my-db", false},
		{"google.com:project:region:instance", "google.com:project", "region", "instance", false},
		{"google.com:missing:part", "", "", "", true},
		{"proj:my-db", "", "", "", true},
		{"googlecom:project:region:instance", "", "", "", true},
		{"google:com:project:region:instance", "", "", "", true},
	}

	for _, test := range table {
		inst, err := NewInstance(test.in)
		gotProj, gotRegion, gotInstance := inst.Project, inst.Region, inst.Database
		if gotProj != test.wantProj {
			t.Errorf("NewInstance(%q): got %v for project, want %v", test.in, gotProj, test.wantProj)
		}
		if gotRegion != test.wantRegion {
			t.Errorf("NewInstance(%q): got %v for region, want %v", test.in, gotRegion, test.wantRegion)
		}
		if gotInstance != test.wantInstance {
			t.Errorf("NewInstance(%q): got %v for instance, want %v", test.in, gotInstance, test.wantInstance)
		}
		if (err != nil) != test.wantErr {
			t.Errorf("NewInstance(%q) returned err: %v, wantErr %t", test.in, err, test.wantErr)
		}
	}
}

func TestInstanceString(t *testing.T) {
	table := []struct {
		project, region, database, wantStr string
	}{
		{"proj", "region", "my-db", "proj:region:my-db"},
		{"google.com:project", "region", "instance", "google.com:project:region:instance"},
	}

	for _, test := range table {
		inst := Instance{test.project, test.region, test.database}
		gotStr := inst.String()
		if gotStr != test.wantStr {
			t.Errorf("string of %#v: got %s, want %s", inst, gotStr, test.wantStr)
		}
	}
}
