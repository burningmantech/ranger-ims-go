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

package store

import (
	"context"
	"database/sql"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
)

const createIncident = `-- name: CreateIncident :one
insert into INCIDENT(EVENT, NUMBER, STATE, CREATED, PRIORITY)
values (?, (select ifnull(max(inc.NUMBER),0) + 1 as NEXT_ID
            from INCIDENT inc
            where inc.EVENT = ?),
        ?, ?, ?)
returning (NUMBER)
`

type CreateIncidentParams struct {
	Event    int32
	State    imsdb.IncidentState
	Created  float64
	Priority int8
}

func (l DBQ) CreateIncident(ctx context.Context, db imsdb.DBTX, arg CreateIncidentParams) (int32, error) {
	row := db.QueryRowContext(ctx, createIncident,
		arg.Event,
		arg.Event,
		arg.State,
		arg.Created,
		arg.Priority,
	)
	var incidentNumber int32
	err := row.Scan(&incidentNumber)
	return incidentNumber, err
}

const createFieldReport = `-- name: CreateFieldReport :one
insert into FIELD_REPORT (
    EVENT, NUMBER, CREATED, SUMMARY, INCIDENT_NUMBER
)
values (?, (select ifnull(max(fr.NUMBER),0) + 1 as NEXT_ID
            from FIELD_REPORT fr
            where fr.EVENT = ?),
        ?, ?, ?)
returning (NUMBER)
`

type CreateFieldReportParams struct {
	Event          int32
	Created        float64
	Summary        sql.NullString
	IncidentNumber sql.NullInt32
}

func (l DBQ) CreateFieldReport(ctx context.Context, db imsdb.DBTX, arg CreateFieldReportParams) (int32, error) {
	row := db.QueryRowContext(ctx, createFieldReport,
		arg.Event,
		arg.Event,
		arg.Created,
		arg.Summary,
		arg.IncidentNumber,
	)
	var fieldReportNumber int32
	err := row.Scan(&fieldReportNumber)
	return fieldReportNumber, err
}
