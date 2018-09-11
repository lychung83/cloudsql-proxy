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

	"google.golang.org/api/support/bundler"
	mpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

// projectData contain per-project data in exporter. It should be created by newProjectData()
type projectData struct {
	parent    *Exporter
	projectID string
	// We make bundler for each project because call to monitoring API can be grouped only in
	// project level
	bndler *bundler.Bundler
}

// uploadRowData is called by bundler to upload row data, and report any error happened meanwhile.
func (pd *projectData) uploadRowData(bundle interface{}) {
	exp := pd.parent
	rds := bundle.([]*RowData)

	// uploadTs contains TimeSeries objects that needs to be uploaded.
	var uploadTs []*mpb.TimeSeries = nil
	// uploadRds contains RowData objects corresponds to uploadTs. It's used for error reporting
	// when upload operation fails.
	var uploadRds []*RowData = nil

	for _, rd := range rds {
		ts, err := exp.makeTS(rd)
		if err != nil {
			exp.opts.OnError(err, rd)
			continue
		}
		// Time series created. We update both uploadTs and uploadRds.
		uploadTs = append(uploadTs, ts)
		uploadRds = append(uploadRds, rd)
		if len(uploadTs) == exp.opts.BundleCountThreshold {
			pd.uploadTimeSeries(uploadTs, uploadRds)
			uploadTs = nil
			uploadRds = nil
		}
	}
	// Upload any remaining time series.
	if len(uploadTs) != 0 {
		pd.uploadTimeSeries(uploadTs, uploadRds)
	}
}

// uploadTimeSeries uploads timeSeries. ts and rds must contain matching data, and ts must not be
// empty. When uploading fails, this function calls exporter's OnError() directly, not propagating
// errors to the caller.
func (pd *projectData) uploadTimeSeries(ts []*mpb.TimeSeries, rds []*RowData) {
	exp := pd.parent
	req := &mpb.CreateTimeSeriesRequest{
		Name:       fmt.Sprintf("projects/%s", pd.projectID),
		TimeSeries: ts,
	}
	if err := createTimeSeries(exp.client, exp.ctx, req); err != nil {
		newErr := fmt.Errorf("monitoring API call to create time series failed for project %s: %v", pd.projectID, err)
		// We pass all row data not successfully uploaded.
		exp.opts.OnError(newErr, rds...)
	}
}
