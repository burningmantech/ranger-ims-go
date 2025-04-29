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

package api

import (
	"encoding/json"
	"github.com/launchdarkly/eventsource"
	"log/slog"
	"strconv"
	"sync/atomic"
)

const EventSourceChannel = "imsevents"

type IMSEventData struct {
	// EventName an IMS Event Name, e.g. "2025"
	EventName string `json:"event_name,omitzero"`
	Comment   string `json:"comment,omitzero"`

	// Exactly one of IncidentNumber, FieldReportNumber, or InitialEvent must be set,
	// as this indicates the type of IMS SSE.

	IncidentNumber    int32 `json:"incident_number,omitzero"`
	FieldReportNumber int32 `json:"field_report_number,omitzero"`
	InitialEvent      bool  `json:"initial_event,omitzero"`
}

type IMSEvent struct {
	EventID   int64
	EventData IMSEventData
}

func (e IMSEvent) Id() string {
	return strconv.FormatInt(e.EventID, 10)
}

func (e IMSEvent) Event() string {
	if e.EventData.IncidentNumber > 0 {
		return "Incident"
	}
	if e.EventData.FieldReportNumber > 0 {
		return "FieldReport"
	}
	if e.EventData.InitialEvent {
		return "InitialEvent"
	}
	return "UnknownEvent"
}

func (e IMSEvent) Data() string {
	b, err := json.Marshal(e.EventData)
	if err != nil {
		slog.Error("Error converting IMSEvent to JSON", "EventData", e.EventData, "err", err)
	}
	return string(b)
}

type EventSourcerer struct {
	Server    *eventsource.Server
	IdCounter atomic.Int64
}

func NewEventSourcerer() *EventSourcerer {
	es := &EventSourcerer{
		Server:    eventsource.NewServer(),
		IdCounter: atomic.Int64{},
	}
	es.Server.Register(EventSourceChannel, es)
	es.Server.ReplayAll = true
	return es
}

func (es *EventSourcerer) notifyFieldReportUpdate(eventName string, frNumber int32) {
	if frNumber == 0 {
		return
	}
	es.Server.Publish([]string{EventSourceChannel}, IMSEvent{
		EventID: es.IdCounter.Add(1),
		EventData: IMSEventData{
			EventName:         eventName,
			FieldReportNumber: frNumber,
		},
	})
}

func (es *EventSourcerer) notifyIncidentUpdate(eventName string, incidentNumber int32) {
	if incidentNumber == 0 {
		return
	}
	es.Server.Publish([]string{EventSourceChannel}, IMSEvent{
		EventID: es.IdCounter.Add(1),
		EventData: IMSEventData{
			EventName:      eventName,
			IncidentNumber: incidentNumber,
		},
	})
}

func (es *EventSourcerer) Replay(channel, id string) chan eventsource.Event {
	if channel != EventSourceChannel {
		return nil
	}
	out := make(chan eventsource.Event, 1)
	out <- IMSEvent{
		EventID: es.IdCounter.Load(),
		EventData: IMSEventData{
			InitialEvent: true,
			Comment:      "The most recent SSE ID is provided in this message",
		},
	}
	close(out)
	return out
}
