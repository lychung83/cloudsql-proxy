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

// Package util contains utility functions for use throughout the Cloud SQL Proxy.
package util

import (
	"fmt"
	"strings"
)

// Instance represents fully qualified instance name. It should be initialized by NewInstance.
type Instance struct {
	Project, Region, Database string
}

// NewInstance makes an Instance object by splitting a fully qualified instance name into its
// project, region, and instance name components. project, region, instance names are all required.
// project name may optionally preceded by organization name.
//
// Examples:
//    "proj:region:my-db" -> Instance{"proj", "region", "my-db"}, true
//    "google.com:project:region:instance" ->
//        Instance{"google.com:project", "region", "instance"}, true
//    "google.com:missing:part" -> Instance{}, false
func NewInstance(name string) (Instance, error) {
	err := fmt.Errorf("invalid instance name: must be in the form `project:region:instance-name`; invalid name was %q", name)
	spl := strings.Split(name, ":")
	switch len(spl) {
	case 3:
		if strings.ContainsRune(spl[0], '.') {
			return Instance{}, err
		}
		return Instance{spl[0], spl[1], spl[2]}, nil
	case 4:
		if !strings.ContainsRune(spl[0], '.') {
			return Instance{}, err
		}
		return Instance{fmt.Sprintf("%s:%s", spl[0], spl[1]), spl[2], spl[3]}, nil
	default:
		return Instance{}, err
	}
}

func (inst Instance) String() string {
	return fmt.Sprintf("%s:%s:%s", inst.Project, inst.Region, inst.Database)
}
