package synadia

import (
	"fmt"
	"net/http"

	"github.com/synadia-io/control-plane-sdk-go/syncp"
)

// SubjectExportInput bundles the fields pb-synadia hooks push to Synadia
// for a subject export create or update. Mirrors the user-facing fields on
// nats_account_exports — fields that have no Synadia equivalent
// (allow_trace) are not present here.
type SubjectExportInput struct {
	Name                 string
	Subject              string
	Type                 string // "stream" or "service"
	TokenReq             bool
	ResponseType         string // "Singleton" | "Stream" | "Chunked" (service only)
	ResponseThreshold    int64  // ms, service only
	AccountTokenPosition int64
	Advertise            bool
	Description          string
}

// SubjectExportResult holds the fields pb-synadia caches back onto the PB
// record after a successful create/update.
type SubjectExportResult struct {
	ID      string
	Subject string
	Name    string
}

// CreateSubjectExport creates a subject export on the given Synadia account.
// MetricsEnabled / MetricsSamplingPercentage are not exposed today — defaulted
// to false / 0.
func (c *Client) CreateSubjectExport(synadiaAccountID string, in SubjectExportInput) (SubjectExportResult, error) {
	req := syncp.SubjectExportCreateRequest{
		JwtSettings:               buildExport(in),
		MetricsEnabled:            false,
		MetricsSamplingPercentage: 0,
	}
	resp, _, err := c.api.AccountAPI.CreateSubjectExport(c.ctx, synadiaAccountID).
		SubjectExportCreateRequest(req).
		Execute()
	if err != nil {
		return SubjectExportResult{}, fmt.Errorf("create subject export %q on account %q: %w",
			in.Name, synadiaAccountID, err)
	}
	return subjectExportViewToResult(resp), nil
}

// UpdateSubjectExport patches an existing subject export.
func (c *Client) UpdateSubjectExport(synadiaExportID string, in SubjectExportInput) (SubjectExportResult, error) {
	req := syncp.SubjectExportUpdateRequest{
		JwtSettings: buildExportPatch(in),
	}
	resp, _, err := c.api.SubjectExportAPI.UpdateSubjectExport(c.ctx, synadiaExportID).
		SubjectExportUpdateRequest(req).
		Execute()
	if err != nil {
		return SubjectExportResult{}, fmt.Errorf("update subject export %q: %w", synadiaExportID, err)
	}
	return subjectExportViewToResult(resp), nil
}

// DeleteSubjectExport removes a subject export. Caller should use IsNotFound
// on the returned response+error to treat already-gone as success.
func (c *Client) DeleteSubjectExport(synadiaExportID string) (*http.Response, error) {
	resp, err := c.api.SubjectExportAPI.DeleteSubjectExport(c.ctx, synadiaExportID).Execute()
	if err != nil {
		return resp, fmt.Errorf("delete subject export %q: %w", synadiaExportID, err)
	}
	return resp, nil
}

func buildExport(in SubjectExportInput) syncp.Export {
	exp := syncp.Export{
		Name:      syncp.Ptr(in.Name),
		Subject:   syncp.Ptr(in.Subject),
		Type:      exportTypePtr(in.Type),
		TokenReq:  syncp.Ptr(in.TokenReq),
		Advertise: syncp.Ptr(in.Advertise),
	}
	if in.AccountTokenPosition != 0 {
		exp.AccountTokenPosition = syncp.Ptr(in.AccountTokenPosition)
	}
	if in.Description != "" {
		exp.Info = syncp.Info{Description: syncp.Ptr(in.Description)}
	}
	if in.Type == "service" {
		if in.ResponseType != "" {
			exp.ResponseType = responseTypePtr(in.ResponseType)
		}
		if in.ResponseThreshold != 0 {
			exp.ResponseThreshold = syncp.Ptr(in.ResponseThreshold)
		}
	}
	return exp
}

func buildExportPatch(in SubjectExportInput) *syncp.ExportPatch {
	patch := &syncp.ExportPatch{
		Name:      syncp.Ptr(in.Name),
		Subject:   syncp.Ptr(in.Subject),
		Type:      exportTypePtr(in.Type),
		TokenReq:  syncp.Ptr(in.TokenReq),
		Advertise: syncp.Ptr(in.Advertise),
	}
	if in.Description != "" {
		nb := syncp.NewNullable(in.Description)
		patch.Description = &nb
	}
	if in.AccountTokenPosition != 0 {
		nb := syncp.NewNullable(in.AccountTokenPosition)
		patch.AccountTokenPosition = &nb
	}
	if in.Type == "service" {
		if in.ResponseType != "" {
			patch.ResponseType = responseTypePtr(in.ResponseType)
		}
		if in.ResponseThreshold != 0 {
			nb := syncp.NewNullable(in.ResponseThreshold)
			patch.ResponseThreshold = &nb
		}
	}
	return patch
}

func exportTypePtr(s string) *syncp.ExportType {
	var t syncp.ExportType
	switch s {
	case "service":
		t = syncp.EXPORTTYPE_SERVICE
	default:
		t = syncp.EXPORTTYPE_STREAM
	}
	return &t
}

func responseTypePtr(s string) *syncp.ResponseType {
	var t syncp.ResponseType
	switch s {
	case "Stream":
		t = syncp.RESPONSETYPE_STREAM
	case "Chunked":
		t = syncp.RESPONSETYPE_CHUNKED
	default:
		t = syncp.RESPONSETYPE_SINGLETON
	}
	return &t
}

func subjectExportViewToResult(resp *syncp.SubjectExportViewResponse) SubjectExportResult {
	if resp == nil {
		return SubjectExportResult{}
	}
	r := SubjectExportResult{
		ID:      resp.Id,
		Subject: resp.Subject,
	}
	if !resp.Name.IsNull() {
		r.Name = resp.Name.Val
	}
	return r
}

