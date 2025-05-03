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

package api_test

import (
	"bytes"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/api"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

type exampleAction struct {
	output *bytes.Buffer
}

func (e exampleAction) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintln(e.output, "      in the action")
}

func firstAdapter(output *bytes.Buffer) api.Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(output, "firstAdapter before")
			next.ServeHTTP(w, r)
			fmt.Fprintln(output, "firstAdapter after")
		})
	}
}

func secondAdapter(output *bytes.Buffer) api.Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(output, "  secondAdapter before")
			next.ServeHTTP(w, r)
			fmt.Fprintln(output, "  secondAdapter after")
		})
	}
}

func thirdAdapter(output *bytes.Buffer) api.Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(output, "    thirdAdapter before")
			next.ServeHTTP(w, r)
			fmt.Fprintln(output, "    thirdAdapter after")
		})
	}
}

// TestAdapt demonstrates how the Adapter pattern works.
func TestAdapt(t *testing.T) {
	t.Parallel()
	b := bytes.Buffer{}
	api.Adapt(
		exampleAction{output: &b},
		firstAdapter(&b),
		secondAdapter(&b),
		thirdAdapter(&b),
	).ServeHTTP(nil, nil)
	require.Equal(t, ""+
		"firstAdapter before\n"+
		"  secondAdapter before\n"+
		"    thirdAdapter before\n"+
		"      in the action\n"+
		"    thirdAdapter after\n"+
		"  secondAdapter after\n"+
		"firstAdapter after\n",
		b.String(),
	)
}
