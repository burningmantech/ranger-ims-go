// Code generated by templ - DO NOT EDIT.

// templ: version: v0.3.924
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

//lint:file-ignore SA4006 This context is only used if a nested component is present.

import "github.com/a-h/templ"
import templruntime "github.com/a-h/templ/runtime"

func Incident(deployment, versionName, versionRef, eventName string) templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var1 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var1 == nil {
			templ_7745c5c3_Var1 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 1, "<!doctype html><html lang=\"en\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = Head("Incident Details | "+eventName, "incident.js", false).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 2, "<body><div class=\"container-fluid\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = Header(deployment).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = Nav(eventName).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = LoadingOverlay().Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 3, "<div id=\"error_info\" class=\"hidden text-danger\"><p id=\"error_text\"></p></div><!-- Help modal for incident page --><div class=\"modal no-print\" id=\"helpModal\" tabindex=\"-1\" aria-labelledby=\"helpModalLabel\" aria-hidden=\"true\"><div class=\"modal-dialog\"><div class=\"modal-content\"><div class=\"modal-header\"><p class=\"modal-title fs-5\" id=\"helpModalLabel\">Keyboard shortcuts</p><button type=\"button\" class=\"btn-close\" data-bs-dismiss=\"modal\" aria-label=\"Close\"></button></div><div class=\"modal-body\"><code>n</code>: create (n)ew Incident <br><code>a</code>: jump to (a)dd new report text<br><code>h</code>: toggle showing system-generated (h)istory <br></div></div></div></div><div class=\"modal no-print\" id=\"startTimeModal\" tabindex=\"-1\" aria-labelledby=\"startTimeModalLabel\" aria-hidden=\"true\"><div class=\"modal-dialog\"><div class=\"modal-content\"><div class=\"modal-header\"><p class=\"modal-title fs-5\" id=\"startTimeModalLabel\">Override start time</p><button type=\"button\" class=\"btn-close\" data-bs-dismiss=\"modal\" aria-label=\"Close\"></button></div><div class=\"modal-body row\"><p>Approximately when did the incident start?</p><div class=\"input-group mb-3\"><label for=\"override_start_date\" class=\"control-label input-group-text\">Started</label> <input id=\"override_start_date\" aria-label=\"Start Date\" type=\"date\" class=\"form-control form-control-sm\"> <input id=\"override_start_time\" aria-label=\"Start Time\" type=\"time\" class=\"form-control form-control-sm\"> <span id=\"override_start_tz\" class=\"input-group-text control-label\" title=\"This date and time must be edited in your computer's local time zone.\"></span></div><p>It's normally fine to leave this value as the default, which is the time that the incident was created in IMS. This override is intended for cases in which the incident began hours or days before it was logged in IMS.</p></div></div></div></div><!-- Incident type info --><div class=\"modal no-print\" id=\"incidentTypeInfoModal\" tabindex=\"-1\" aria-labelledby=\"incidentTypeInfoModalLabel\" aria-hidden=\"true\"><div class=\"modal-dialog\"><div class=\"modal-content\"><div class=\"modal-header\"><p class=\"modal-title fs-5\" id=\"incidentTypeInfoModalLabel\">Incident Types</p><button type=\"button\" class=\"btn-close\" data-bs-dismiss=\"modal\" aria-label=\"Close\"></button></div><div class=\"modal-body\"><ul id=\"incident-type-info\" class=\"list-group list-group-small\"></ul></div></div></div></div><!-- Incident number, state, started --><div class=\"row py-1\"><div class=\"col-sm-2 py-1\"><div class=\"input-group flex-nowrap\"><label class=\"control-label input-group-text\">IMS #</label> <span id=\"incident_number\" aria-label=\"IMS #\" class=\"form-control form-control-static\"></span></div></div><div class=\"col-sm-4 py-1\"><div class=\"input-group\"><label for=\"incident_state\" class=\"control-label input-group-text\">State</label> <select id=\"incident_state\" class=\"form-control form-select form-select-sm auto-width\" onchange=\"editState()\"><option value=\"new\">New</option> <option value=\"on_hold\">On Hold</option> <option value=\"dispatched\">Dispatched</option> <option value=\"on_scene\">On Scene</option> <option value=\"closed\">Closed</option></select></div></div><div class=\"col-sm-6 py-1\"><div class=\"input-group\"><label class=\"control-label input-group-text\" for=\"started_datetime\">Started</label> <span class=\"form-control form-control-static align-middle d-flex justify-content-between\"><span id=\"started_datetime\"></span> <span id=\"override_started_button\" class=\"show-pointer no-print\" title=\"Override Start Time\"><svg fill=\"currentColor\" class=\"bi align-middle\"><use href=\"#pencil-square\"></use></svg></span></span></div></div></div><!-- Summary --><div class=\"row\"><div class=\"input-group\"><label for=\"incident_summary\" class=\"input-group-text control-label\">Summary</label> <input id=\"incident_summary\" class=\"form-control form-control-sm\" type=\"text\" inputmode=\"latin-prose\" onchange=\"editIncidentSummary()\"></div></div><!-- Attached Rangers, incident types --><div class=\"row\"><div class=\"col-sm-6 py-2\"><div class=\"card\"><label class=\"control-label card-header\">Rangers <a href=\"https://ranger-clubhouse.burningman.org/reports/on-duty\" target=\"_blank\" class=\"link-body-emphasis float-end no-print\" title=\"On-Duty Report\">On-Duty <svg fill=\"currentColor\" class=\"bi\"><use href=\"#box-arrow-up-right\"></use></svg></a></label><ul id=\"incident_rangers_list\" class=\"list-group list-group-flush list-group-small card-body\"><li class=\"list-group-item ps-3 hidden\"><button class=\"badge btn btn-danger remove-badge float-end\" onclick=\"removeRanger(this)\">X</button></li></ul><div class=\"flex-input-container card-footer no-print\"><label for=\"ranger_add\" class=\"control-label\">Add:</label> <input type=\"text\" id=\"ranger_add\" aria-label=\"Add Ranger Handle\" list=\"ranger_handles\" class=\"form-control form-control-sm auto-width\" onchange=\"addRanger()\"> <datalist id=\"ranger_handles\"><option value=\"\"></option></datalist></div></div></div><div class=\"col-sm-6 py-2\"><div class=\"card\"><label class=\"control-label card-header\">Incident Types <a href=\"#\" class=\"link-body-emphasis float-end no-print\" id=\"show-incident-type-info\" title=\"Show Incident Type descriptions\">Info <svg fill=\"currentColor\" class=\"bi\"><use href=\"#question-circle\"></use></svg></a></label><ul id=\"incident_types_list\" class=\"list-group list-group-flush list-group-small card-body\"><li class=\"list-group-item ps-3 hidden\"><button class=\"badge btn btn-danger remove-badge float-end\" onclick=\"removeIncidentType(this)\">X</button></li></ul><div class=\"card-footer flex-input-container no-print\"><label class=\"control-label\">Add:</label> <input type=\"text\" id=\"incident_type_add\" aria-label=\"Add Incident Type\" list=\"incident_types\" class=\"form-control form-control-sm auto-width\" onchange=\"addIncidentType()\"> <datalist id=\"incident_types\"><option value=\"\"></option></datalist></div></div></div></div><!-- Location --><div class=\"row py-1\"><div class=\"col-sm-12\"><div class=\"card\"><label class=\"control-label card-header\">Location</label><div class=\"card-body\"><form class=\"form-horizontal\"><div class=\"input-group row align-items-center\"><label for=\"incident_location_name\" class=\"col-sm-2 col-form-label control-label\">Name:</label><div class=\"col-sm-10\"><input id=\"incident_location_name\" class=\"form-control form-control-sm\" type=\"text\" inputmode=\"latin-prose\" placeholder=\"Name of location\" aria-label=\"Location name\" onchange=\"editLocationName()\"></div></div><div class=\"input-group row align-items-center\"><span class=\"col-sm-2 col-form-label control-label\">Address:</span><div id=\"incident_address\" class=\"col-sm-10\"><select id=\"incident_location_address_radial_hour\" class=\"form-control form-select auto-width\" aria-label=\"Incident location address radial hour\" onchange=\"editLocationAddressRadialHour()\"><option value=\"\"></option></select> ")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var2 string
		templ_7745c5c3_Var2, templ_7745c5c3_Err = templ.JoinStringErrs(":")
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `web/template/incident.templ`, Line: 253, Col: 22}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var2))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 4, " <select id=\"incident_location_address_radial_minute\" class=\"form-control form-control-sm form-select form-select-sm  auto-width\" aria-label=\"Incident location address radial minute\" onchange=\"editLocationAddressRadialMinute()\"><option value=\"\"></option></select> ")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var3 string
		templ_7745c5c3_Var3, templ_7745c5c3_Err = templ.JoinStringErrs("@")
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `web/template/incident.templ`, Line: 262, Col: 22}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var3))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 5, " <select id=\"incident_location_address_concentric\" class=\"form-control form-select form-select-sm auto-width\" aria-label=\"Incident location address concentric street\" onchange=\"editLocationAddressConcentric()\"><option value=\"\"></option></select></div></div><div class=\"input-group row align-items-center\"><label for=\"incident_location_description\" class=\"col-sm-2 col-form-label control-label\">Details:</label><div class=\"col-sm-10\"><input id=\"incident_location_description\" class=\"form-control form-control-sm\" type=\"text\" inputmode=\"latin-prose\" aria-label=\"Additional location description\" placeholder=\"Other identifying info\" onchange=\"editLocationDescription()\"></div></div></form></div></div></div></div><!-- Attached field reports --><div class=\"row py-2\"><div class=\"col-sm-12\"><div class=\"card\"><label class=\"control-label card-header\">Attached Field Reports</label><ul id=\"attached_field_reports\" class=\"list-group list-group-small card-body list-group-flush\"><li class=\"list-group-item ps-3 hidden\"><button class=\"badge btn btn-danger remove-badge float-end\" onclick=\"detachFieldReport(this)\">X</button></li></ul><div id=\"attached_field_report_add_container\" class=\"flex-input-container card-footer no-print\"><label class=\"control-label\">Add:</label> <select id=\"attached_field_report_add\" class=\"form-control form-control-sm\" onchange=\"attachFieldReport()\"><option value=\"\"></option></select></div></div></div></div><!-- Incident details --><div class=\"row\"><div class=\"col-sm-12\"><div class=\"card\"><label class=\"control-label card-header\">Entries from Incident and attached Field Reports</label><div id=\"report_entry_well\" class=\"card-body\"><label class=\"control-label\"><input id=\"history_checkbox\" type=\"checkbox\" onchange=\"toggleShowHistory()\"> Show history and stricken</label><div id=\"report_entries\"></div></div><div class=\"card-footer\"><textarea id=\"report_entry_add\" aria-label=\"New report entry text\" class=\"form-control no-print\" placeholder=\"Press &quot;a&quot; to add report text\" onchange=\"reportEntryEdited()\"></textarea><div class=\"d-flex justify-content-between\"><button id=\"report_entry_submit\" aria-label=\"Submit report entry\" type=\"submit\" class=\"btn btn-default btn-sm btn-block my-1 disabled no-print float-start\" onclick=\"submitReportEntry()\">Add Entry (Control ⏎)</button><!-- File attachment --><label class=\"input-group-text hidden\" for=\"attach_file_input\">Attach file</label> <input type=\"file\" class=\"form-control hidden\" id=\"attach_file_input\" name=\"filename\" onchange=\"attachFile();\"> <input id=\"attach_file\" type=\"button\" class=\"btn btn-default btn-sm btn-block btn-secondary my-1 form-control-lite no-print hidden\" value=\"Attach file\" onclick=\"document.getElementById('attach_file_input').click();\"></div></div></div></div></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = Footer(versionName, versionRef).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 6, "</div></body></html>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return nil
	})
}

var _ = templruntime.GeneratedTemplate
