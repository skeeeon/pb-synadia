package synadia

import (
	"fmt"
	"net/http"

	"github.com/synadia-io/control-plane-sdk-go/syncp"
)

// SubjectImportInput bundles the fields pb-synadia hooks push to Synadia
// for a subject import create or update. Mirrors the user-facing fields on
// nats_account_imports — fields with no Synadia equivalent (allow_trace,
// description) are not present here.
type SubjectImportInput struct {
	Name         string
	Subject      string
	Type         string // "stream" or "service"
	Account      string // exporting account's public NKey
	Token        string // activation JWT, required only when the export sets TokenReq
	LocalSubject string
	Share        bool
}

// SubjectImportResult holds the fields pb-synadia caches back onto the PB
// record after a successful create/update.
type SubjectImportResult struct {
	ID      string
	Name    string
	Subject string
}

// CreateSubjectImport creates a subject import on the given Synadia account.
func (c *Client) CreateSubjectImport(synadiaAccountID string, in SubjectImportInput) (SubjectImportResult, error) {
	req := syncp.SubjectImportCreateRequest{
		JwtSettings: buildImport(in),
	}
	resp, _, err := c.api.AccountAPI.CreateSubjectImport(c.ctx, synadiaAccountID).
		SubjectImportCreateRequest(req).
		Execute()
	if err != nil {
		return SubjectImportResult{}, fmt.Errorf("create subject import %q on account %q: %w",
			in.Name, synadiaAccountID, err)
	}
	return subjectImportViewToResult(resp), nil
}

// UpdateSubjectImport patches an existing subject import.
func (c *Client) UpdateSubjectImport(synadiaImportID string, in SubjectImportInput) (SubjectImportResult, error) {
	req := syncp.SubjectImportUpdateRequest{
		JwtSettings: buildImportPatch(in),
	}
	resp, _, err := c.api.SubjectImportAPI.UpdateSubjectImport(c.ctx, synadiaImportID).
		SubjectImportUpdateRequest(req).
		Execute()
	if err != nil {
		return SubjectImportResult{}, fmt.Errorf("update subject import %q: %w", synadiaImportID, err)
	}
	return subjectImportViewToResult(resp), nil
}

// DeleteSubjectImport removes a subject import. Caller should use IsNotFound
// on the returned response+error to treat already-gone as success.
func (c *Client) DeleteSubjectImport(synadiaImportID string) (*http.Response, error) {
	resp, err := c.api.SubjectImportAPI.DeleteSubjectImport(c.ctx, synadiaImportID).Execute()
	if err != nil {
		return resp, fmt.Errorf("delete subject import %q: %w", synadiaImportID, err)
	}
	return resp, nil
}

func buildImport(in SubjectImportInput) syncp.Import {
	imp := syncp.Import{
		Name:    syncp.Ptr(in.Name),
		Subject: syncp.Ptr(in.Subject),
		Type:    exportTypePtr(in.Type),
		Account: syncp.Ptr(in.Account),
		Share:   syncp.Ptr(in.Share),
	}
	if in.Token != "" {
		imp.Token = syncp.Ptr(in.Token)
	}
	if in.LocalSubject != "" {
		imp.LocalSubject = syncp.Ptr(in.LocalSubject)
	}
	return imp
}

func buildImportPatch(in SubjectImportInput) *syncp.ImportPatch {
	patch := &syncp.ImportPatch{
		Name:    syncp.Ptr(in.Name),
		Subject: syncp.Ptr(in.Subject),
		Type:    exportTypePtr(in.Type),
		Account: syncp.Ptr(in.Account),
		Share:   syncp.Ptr(in.Share),
	}
	if in.Token != "" {
		nb := syncp.NewNullable(in.Token)
		patch.Token = &nb
	}
	if in.LocalSubject != "" {
		nb := syncp.NewNullable(in.LocalSubject)
		patch.LocalSubject = &nb
	}
	return patch
}

func subjectImportViewToResult(resp *syncp.SubjectImportViewResponse) SubjectImportResult {
	if resp == nil {
		return SubjectImportResult{}
	}
	return SubjectImportResult{
		ID:      resp.Id,
		Name:    resp.Name,
		Subject: resp.RemoteSubject,
	}
}
