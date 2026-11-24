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

package directory_test

import (
	"testing"
	"time"

	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/stretchr/testify/require"
)

func TestOnDuty_ClubhouseHasATimesheet(t *testing.T) {
	t.Parallel()
	store := directory.NewUserStore(directory.NewClubhouseSource(nil), 0)
	require.True(t, store.OnDutyAvailable())
}

func TestOnDuty_IMSDirectoryHasNoTimesheet(t *testing.T) {
	t.Parallel()
	store := directory.NewUserStore(directory.NewIMSSource(nil), 0)
	require.False(t, store.OnDutyAvailable())
	onDuty, err := store.OnDuty(t.Context(), time.Now())
	require.NoError(t, err)
	require.Empty(t, onDuty)
}
