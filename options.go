package pbsynadia

import (
	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
)

// DefaultOptions returns sensible defaults for pb-synadia configuration.
//
// SystemID and APIToken must be set explicitly — there are no defaults.
func DefaultOptions() Options {
	return Options{
		SynadiaBaseURL: pbtypes.DefaultSynadiaBaseURL,

		UserCollectionName:    pbtypes.DefaultUserCollectionName,
		RoleCollectionName:    pbtypes.DefaultRoleCollectionName,
		AccountCollectionName: pbtypes.DefaultAccountCollectionName,
		ExportCollectionName:  pbtypes.DefaultExportCollectionName,
		ImportCollectionName:  pbtypes.DefaultImportCollectionName,

		DefaultPublishPermissions:   pbtypes.DefaultPublishPermissions,
		DefaultSubscribePermissions: pbtypes.DefaultSubscribePermissions,

		LogToConsole: true,

		EventFilter: nil,
	}
}

// applyDefaultOptions fills in default values for any missing options.
func applyDefaultOptions(options Options) Options {
	defaults := DefaultOptions()

	if options.SynadiaBaseURL == "" {
		options.SynadiaBaseURL = defaults.SynadiaBaseURL
	}
	if options.UserCollectionName == "" {
		options.UserCollectionName = defaults.UserCollectionName
	}
	if options.RoleCollectionName == "" {
		options.RoleCollectionName = defaults.RoleCollectionName
	}
	if options.AccountCollectionName == "" {
		options.AccountCollectionName = defaults.AccountCollectionName
	}
	if options.ExportCollectionName == "" {
		options.ExportCollectionName = defaults.ExportCollectionName
	}
	if options.ImportCollectionName == "" {
		options.ImportCollectionName = defaults.ImportCollectionName
	}
	if len(options.DefaultPublishPermissions) == 0 {
		options.DefaultPublishPermissions = defaults.DefaultPublishPermissions
	}
	if len(options.DefaultSubscribePermissions) == 0 {
		options.DefaultSubscribePermissions = defaults.DefaultSubscribePermissions
	}

	return options
}
