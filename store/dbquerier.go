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
	"fmt"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"log/slog"
	"time"
)

// DBQ combines the SQL database and the Querier for the IMS datastore.
type DBQ struct {
	*sql.DB
	q imsdb.Querier
}

func NewDBQ(sqlDB *sql.DB, querier imsdb.Querier) *DBQ {
	return &DBQ{
		DB: sqlDB,
		q:  querier,
	}
}

func logQuery(queryName string, start time.Time, err error) {
	durationMS := float64(time.Since(start).Microseconds()) / 1000.0
	slog.Debug("Ran IMS SQL: "+queryName,
		"duration", fmt.Sprintf("%.3fms", durationMS),
		"err", err,
	)
}

// TODO: possibly do this sort of caching
//  where DBQ has a "eventsCache *cache.InMemory[[]imsdb.EventsRow]" field
// func (l DBQ) Events(ctx context.Context, db imsdb.DBTX) ([]imsdb.EventsRow, error) {
//	rows, err := l.eventsCache.Get(ctx)
//	return orNil(rows), err
//}
//
// func orNil[S ~*[]E, E any](sl S) []E {
//	if sl == nil {
//		return nil
//	}
//	return *sl
//}

// Force DBQ to implement the imsdb.Querier interface.
var _ imsdb.Querier = (*DBQ)(nil)

func (l DBQ) AddEventAccess(ctx context.Context, db imsdb.DBTX, arg imsdb.AddEventAccessParams) (int64, error) {
	start := time.Now()
	id, err := l.q.AddEventAccess(ctx, db, arg)
	logQuery("AddEventAccess", start, err)
	return id, err
}

func (l DBQ) AttachFieldReportToIncident(ctx context.Context, db imsdb.DBTX, arg imsdb.AttachFieldReportToIncidentParams) error {
	start := time.Now()
	err := l.q.AttachFieldReportToIncident(ctx, db, arg)
	logQuery("AttachFieldReportToIncident", start, err)
	return err
}

func (l DBQ) AttachRangerHandleToIncident(ctx context.Context, db imsdb.DBTX, arg imsdb.AttachRangerHandleToIncidentParams) error {
	start := time.Now()
	err := l.q.AttachRangerHandleToIncident(ctx, db, arg)
	logQuery("AttachRangerHandleToIncident", start, err)
	return err
}

func (l DBQ) AttachReportEntryToFieldReport(ctx context.Context, db imsdb.DBTX, arg imsdb.AttachReportEntryToFieldReportParams) error {
	start := time.Now()
	err := l.q.AttachReportEntryToFieldReport(ctx, db, arg)
	logQuery("AttachReportEntryToFieldReport", start, err)
	return err
}

func (l DBQ) AttachReportEntryToIncident(ctx context.Context, db imsdb.DBTX, arg imsdb.AttachReportEntryToIncidentParams) error {
	start := time.Now()
	err := l.q.AttachReportEntryToIncident(ctx, db, arg)
	logQuery("AttachReportEntryToIncident", start, err)
	return err
}

func (l DBQ) ClearEventAccessForExpression(ctx context.Context, db imsdb.DBTX, arg imsdb.ClearEventAccessForExpressionParams) error {
	start := time.Now()
	err := l.q.ClearEventAccessForExpression(ctx, db, arg)
	logQuery("ClearEventAccessForExpression", start, err)
	return err
}

func (l DBQ) ClearEventAccessForMode(ctx context.Context, db imsdb.DBTX, arg imsdb.ClearEventAccessForModeParams) error {
	start := time.Now()
	err := l.q.ClearEventAccessForMode(ctx, db, arg)
	logQuery("ClearEventAccessForMode", start, err)
	return err
}

func (l DBQ) ConcentricStreets(ctx context.Context, db imsdb.DBTX, event int32) ([]imsdb.ConcentricStreetsRow, error) {
	start := time.Now()
	streets, err := l.q.ConcentricStreets(ctx, db, event)
	logQuery("ConcentricStreets", start, err)
	return streets, err
}

func (l DBQ) CreateConcentricStreet(ctx context.Context, db imsdb.DBTX, arg imsdb.CreateConcentricStreetParams) error {
	start := time.Now()
	err := l.q.CreateConcentricStreet(ctx, db, arg)
	logQuery("CreateConcentricStreet", start, err)
	return err
}

func (l DBQ) CreateEvent(ctx context.Context, db imsdb.DBTX, name string) (int64, error) {
	start := time.Now()
	event, err := l.q.CreateEvent(ctx, db, name)
	logQuery("CreateEvent", start, err)
	return event, err
}

func (l DBQ) CreateFieldReport(ctx context.Context, db imsdb.DBTX, arg imsdb.CreateFieldReportParams) error {
	start := time.Now()
	err := l.q.CreateFieldReport(ctx, db, arg)
	logQuery("CreateFieldReport", start, err)
	return err
}

func (l DBQ) CreateIncident(ctx context.Context, db imsdb.DBTX, arg imsdb.CreateIncidentParams) (int64, error) {
	start := time.Now()
	incident, err := l.q.CreateIncident(ctx, db, arg)
	logQuery("CreateIncident", start, err)
	return incident, err
}

func (l DBQ) CreateIncidentType(ctx context.Context, db imsdb.DBTX, arg imsdb.CreateIncidentTypeParams) (int64, error) {
	start := time.Now()
	id, err := l.q.CreateIncidentType(ctx, db, arg)
	logQuery("CreateIncidentType", start, err)
	return id, err
}

func (l DBQ) CreateReportEntry(ctx context.Context, db imsdb.DBTX, arg imsdb.CreateReportEntryParams) (int64, error) {
	start := time.Now()
	entry, err := l.q.CreateReportEntry(ctx, db, arg)
	logQuery("CreateReportEntry", start, err)
	return entry, err
}

func (l DBQ) DetachRangerHandleFromIncident(ctx context.Context, db imsdb.DBTX, arg imsdb.DetachRangerHandleFromIncidentParams) error {
	start := time.Now()
	err := l.q.DetachRangerHandleFromIncident(ctx, db, arg)
	logQuery("DetachRangerHandleFromIncident", start, err)
	return err
}

func (l DBQ) EventAccess(ctx context.Context, db imsdb.DBTX, event int32) ([]imsdb.EventAccessRow, error) {
	start := time.Now()
	items, err := l.q.EventAccess(ctx, db, event)
	logQuery("EventAccess", start, err)
	return items, err
}

func (l DBQ) EventAccessAll(ctx context.Context, db imsdb.DBTX) ([]imsdb.EventAccessAllRow, error) {
	start := time.Now()
	all, err := l.q.EventAccessAll(ctx, db)
	logQuery("EventAccessAll", start, err)
	return all, err
}

func (l DBQ) Events(ctx context.Context, db imsdb.DBTX) ([]imsdb.EventsRow, error) {
	start := time.Now()
	events, err := l.q.Events(ctx, db)
	logQuery("Events", start, err)
	return events, err
}

func (l DBQ) FieldReport(ctx context.Context, db imsdb.DBTX, arg imsdb.FieldReportParams) (imsdb.FieldReportRow, error) {
	start := time.Now()
	report, err := l.q.FieldReport(ctx, db, arg)
	logQuery("FieldReport", start, err)
	return report, err
}

func (l DBQ) FieldReport_ReportEntries(ctx context.Context, db imsdb.DBTX, arg imsdb.FieldReport_ReportEntriesParams) ([]imsdb.FieldReport_ReportEntriesRow, error) {
	start := time.Now()
	entries, err := l.q.FieldReport_ReportEntries(ctx, db, arg)
	logQuery("FieldReport_ReportEntries", start, err)
	return entries, err
}

func (l DBQ) FieldReports(ctx context.Context, db imsdb.DBTX, event int32) ([]imsdb.FieldReportsRow, error) {
	start := time.Now()
	reports, err := l.q.FieldReports(ctx, db, event)
	logQuery("FieldReports", start, err)
	return reports, err
}

func (l DBQ) FieldReports_ReportEntries(ctx context.Context, db imsdb.DBTX, arg imsdb.FieldReports_ReportEntriesParams) ([]imsdb.FieldReports_ReportEntriesRow, error) {
	start := time.Now()
	entries, err := l.q.FieldReports_ReportEntries(ctx, db, arg)
	logQuery("FieldReports_ReportEntries", start, err)
	return entries, err
}

func (l DBQ) UpdateIncidentType(ctx context.Context, db imsdb.DBTX, arg imsdb.UpdateIncidentTypeParams) error {
	start := time.Now()
	err := l.q.UpdateIncidentType(ctx, db, arg)
	logQuery("UpdateIncidentType", start, err)
	return err
}

func (l DBQ) Incident(ctx context.Context, db imsdb.DBTX, arg imsdb.IncidentParams) (imsdb.IncidentRow, error) {
	start := time.Now()
	incident, err := l.q.Incident(ctx, db, arg)
	logQuery("Incident", start, err)
	return incident, err
}

func (l DBQ) IncidentTypes(ctx context.Context, db imsdb.DBTX) ([]imsdb.IncidentTypesRow, error) {
	start := time.Now()
	types, err := l.q.IncidentTypes(ctx, db)
	logQuery("IncidentTypes", start, err)
	return types, err
}

func (l DBQ) IncidentType(ctx context.Context, db imsdb.DBTX, id int32) (imsdb.IncidentTypeRow, error) {
	start := time.Now()
	iType, err := l.q.IncidentType(ctx, db, id)
	logQuery("IncidentType", start, err)
	return iType, err
}

func (l DBQ) Incident_ReportEntries(ctx context.Context, db imsdb.DBTX, arg imsdb.Incident_ReportEntriesParams) ([]imsdb.Incident_ReportEntriesRow, error) {
	start := time.Now()
	entries, err := l.q.Incident_ReportEntries(ctx, db, arg)
	logQuery("Incident_ReportEntries", start, err)
	return entries, err
}

func (l DBQ) Incidents(ctx context.Context, db imsdb.DBTX, event int32) ([]imsdb.IncidentsRow, error) {
	start := time.Now()
	incidents, err := l.q.Incidents(ctx, db, event)
	logQuery("Incidents", start, err)
	return incidents, err
}

func (l DBQ) Incidents_ReportEntries(ctx context.Context, db imsdb.DBTX, arg imsdb.Incidents_ReportEntriesParams) ([]imsdb.Incidents_ReportEntriesRow, error) {
	start := time.Now()
	entries, err := l.q.Incidents_ReportEntries(ctx, db, arg)
	logQuery("Incidents_ReportEntries", start, err)
	return entries, err
}

func (l DBQ) NextFieldReportNumber(ctx context.Context, db imsdb.DBTX, event int32) (int32, error) {
	start := time.Now()
	number, err := l.q.NextFieldReportNumber(ctx, db, event)
	logQuery("NextFieldReportNumber", start, err)
	return number, err
}

func (l DBQ) NextIncidentNumber(ctx context.Context, db imsdb.DBTX, event int32) (int32, error) {
	start := time.Now()
	number, err := l.q.NextIncidentNumber(ctx, db, event)
	logQuery("NextIncidentNumber", start, err)
	return number, err
}

func (l DBQ) QueryEventID(ctx context.Context, db imsdb.DBTX, name string) (imsdb.QueryEventIDRow, error) {
	start := time.Now()
	id, err := l.q.QueryEventID(ctx, db, name)
	logQuery("QueryEventID", start, err)
	return id, err
}

func (l DBQ) SchemaVersion(ctx context.Context, db imsdb.DBTX) (int16, error) {
	start := time.Now()
	version, err := l.q.SchemaVersion(ctx, db)
	logQuery("SchemaVersion", start, err)
	return version, err
}

func (l DBQ) SetFieldReportReportEntryStricken(ctx context.Context, db imsdb.DBTX, arg imsdb.SetFieldReportReportEntryStrickenParams) error {
	start := time.Now()
	err := l.q.SetFieldReportReportEntryStricken(ctx, db, arg)
	logQuery("SetFieldReportReportEntryStricken", start, err)
	return err
}

func (l DBQ) SetIncidentReportEntryStricken(ctx context.Context, db imsdb.DBTX, arg imsdb.SetIncidentReportEntryStrickenParams) error {
	start := time.Now()
	err := l.q.SetIncidentReportEntryStricken(ctx, db, arg)
	logQuery("SetIncidentReportEntryStricken", start, err)
	return err
}

func (l DBQ) UpdateFieldReport(ctx context.Context, db imsdb.DBTX, arg imsdb.UpdateFieldReportParams) error {
	start := time.Now()
	err := l.q.UpdateFieldReport(ctx, db, arg)
	logQuery("UpdateFieldReport", start, err)
	return err
}

func (l DBQ) UpdateIncident(ctx context.Context, db imsdb.DBTX, arg imsdb.UpdateIncidentParams) error {
	start := time.Now()
	err := l.q.UpdateIncident(ctx, db, arg)
	logQuery("UpdateIncident", start, err)
	return err
}

func (l DBQ) AttachIncidentTypeToIncident(ctx context.Context, db imsdb.DBTX, arg imsdb.AttachIncidentTypeToIncidentParams) error {
	start := time.Now()
	err := l.q.AttachIncidentTypeToIncident(ctx, db, arg)
	logQuery("AttachIncidentTypeToIncident", start, err)
	return err
}

func (l DBQ) DetachIncidentTypeFromIncident(ctx context.Context, db imsdb.DBTX, arg imsdb.DetachIncidentTypeFromIncidentParams) error {
	start := time.Now()
	err := l.q.DetachIncidentTypeFromIncident(ctx, db, arg)
	logQuery("DetachIncidentTypeFromIncident", start, err)
	return err
}
