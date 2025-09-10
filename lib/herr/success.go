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

package herr

import "net/http"

// WriteOKResponse writes a status 200 (OK) HTTP response with a text/plain body.
func WriteOKResponse(w http.ResponseWriter, text string) {
	http.Error(w, text, http.StatusOK)
}

// WriteNoContentResponse writes a status 204 (No Content) HTTP response with a text/plain body.
func WriteNoContentResponse(w http.ResponseWriter, text string) {
	http.Error(w, text, http.StatusNoContent)
}

// WriteCreatedResponse writes a status 201 (Created) HTTP response with a text/plain body.
func WriteCreatedResponse(w http.ResponseWriter, text string) {
	http.Error(w, text, http.StatusCreated)
}
