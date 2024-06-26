// Copyright 2022 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package cloudpb

import (
	"fmt"
	"strings"
)

const (
	// ExternalStorageAuthImplicit is used by ExternalStorage instances to
	// indicate access via a node's "implicit" authorization (e.g. machine acct).
	ExternalStorageAuthImplicit = "implicit"

	// ExternalStorageAuthSpecified is used by ExternalStorage instances to
	// indicate access is via explicitly provided credentials.
	ExternalStorageAuthSpecified = "specified"
)

// AccessIsWithExplicitAuth returns true if the external storage config carries
// its own explicit credentials to use for access (or does not require them), as
// opposed to using something about the node to gain implicit access, such as a
// VM's machine account, network access, file system, etc.
func (m *ExternalStorage) AccessIsWithExplicitAuth() bool {
	switch m.Provider {
	case ExternalStorageProvider_s3:
		// custom endpoints could be a network resource only accessible via this
		// node's network context and thus have an element of implicit auth.
		if m.S3Config.Endpoint != "" {
			return false
		}
		return m.S3Config.Auth != ExternalStorageAuthImplicit
	case ExternalStorageProvider_gs:
		return m.GoogleCloudConfig.Auth == ExternalStorageAuthSpecified
	case ExternalStorageProvider_azure:
		return m.AzureConfig.Auth == AzureAuth_LEGACY || m.AzureConfig.Auth == AzureAuth_EXPLICIT
	case ExternalStorageProvider_userfile:
		// userfile always checks the user performing the action has grants on the
		// table used.
		return true
	case ExternalStorageProvider_null:
		return true
	case ExternalStorageProvider_http:
		// Arbitrary network endpoints may be accessible only via the node and thus
		// make use of its implicit access to them.
		return false
	case ExternalStorageProvider_nodelocal:
		// The node's local filesystem is obviously accessed implicitly as the node.
		return false
	case ExternalStorageProvider_external:
		// External Connections have a `USAGE` privilege that determines if a user
		// has the appropriate privileges to use the underlying resource.
		return true
	default:
		return false
	}
}

const assumeRoleProviderExternalIDParam = "external_id"

// EncodeAsString returns the string representation of the provider to be used
// in URIs.
func (p ExternalStorage_AssumeRoleProvider) EncodeAsString() string {
	if p.Role == "" {
		return ""
	}

	if p.ExternalID == "" {
		return p.Role
	}

	return fmt.Sprintf("%s;%s=%s", p.Role, assumeRoleProviderExternalIDParam, p.ExternalID)

}

// DecodeRoleProviderString decodes a string into an
// ExternalStorage_AssumeRoleProvider.
func DecodeRoleProviderString(roleProviderString string) (p ExternalStorage_AssumeRoleProvider) {
	parts := strings.Split(roleProviderString, ";")
	p.Role = parts[0]

	for pidx := 1; pidx < len(parts); pidx++ {
		key, value, _ := strings.Cut(parts[pidx], "=")
		if key == assumeRoleProviderExternalIDParam {
			p.ExternalID = value
		}
	}
	return p
}

// ParseRoleString parses a comma separated string of roles into a list of
// intermediate delegate roles and the final assumed role.
func ParseRoleString(roleString string) (string, []string) {
	roleProvider, delegateRoleProviders := ParseRoleProvidersString(roleString)

	delegateRoles := make([]string, len(delegateRoleProviders))
	for i := range delegateRoleProviders {
		delegateRoles[i] = delegateRoleProviders[i].Role
	}
	return roleProvider.Role, delegateRoles
}

// ParseRoleProvidersString parses a comma separated list of role provider
// strings.
//
// Each role provider string is in the format: "<role>;external_id=<id>"
// The external ID portion of the format, including the ';', is optional and can
// be omitted if there is no external ID needed when assuming the role.
func ParseRoleProvidersString(
	roleProvidersString string,
) (
	assumeRole ExternalStorage_AssumeRoleProvider,
	delegateRoles []ExternalStorage_AssumeRoleProvider,
) {
	if roleProvidersString == "" {
		return assumeRole, delegateRoles
	}

	roleProviders := strings.Split(roleProvidersString, ",")
	delegateRoles = make([]ExternalStorage_AssumeRoleProvider, len(roleProviders)-1)

	assumeRole = DecodeRoleProviderString(roleProviders[len(roleProviders)-1])
	for i := 0; i < len(roleProviders)-1; i++ {
		delegateRoles[i] = DecodeRoleProviderString(roleProviders[i])
	}
	return assumeRole, delegateRoles
}
