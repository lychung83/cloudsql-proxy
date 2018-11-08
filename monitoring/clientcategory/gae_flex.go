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

// +build appenginevm

package clientcategory

// We use the presence of build tag appenginevm to tell whether the app is running under GAE Flex
// environment or not. For similar uses see
// https://github.com/golang/oauth2/blob/d2e6202438beef2727060aa7cabdd924d92ebfd9/google/appengineflex_hook.go
func init() {
	onGAEFlex = true
}
