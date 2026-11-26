//
// See the file COPYRIGHT for copyright information.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package template

import "strings"

// clubhouseURL is the base URL of the Clubhouse server that goes with the deployment.
func clubhouseURL(deployment string) string {
	switch strings.ToLower(deployment) {
	case "training":
		return "https://ranger-clubhouse-training.burningman.org"
	case "staging":
		return "https://ranger-clubhouse-staging.burningman.org"
	default:
		return "https://ranger-clubhouse.burningman.org"
	}
}
