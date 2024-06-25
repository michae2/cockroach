// Copyright 2021 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package uris

import (
	"net/url"
	"path"
	"strings"
)

const (
	// AuthParam is the query parameter for the cluster settings named
	// key in a URI.
	AuthParam = "AUTH"
	// AuthParamImplicit is the query parameter for the implicit authentication
	// mode in a URI.
	AuthParamImplicit = "implicit"
	// AuthParamSpecified is the query parameter for the specified authentication
	// mode in a URI.
	AuthParamSpecified = "specified"
	// LocalityURLParam is the parameter name used when specifying a locality tag
	// in a locality aware backup/restore.
	LocalityURLParam = "COCKROACH_LOCALITY"

	RedactionMarker = "redacted"
)

// redactedQueryParams is the set of query parameter names registered by the
// external storage providers that should be redacted from external storage URIs
// whenever they are displayed to a user.
var redactedQueryParams = map[string]struct{}{}

// GetPrefixBeforeWildcard gets the prefix of a path that does not contain glob-
// style matchers, up to the last path segment before the first one which has a
// glob character.
func GetPrefixBeforeWildcard(p string) string {
	globIndex := strings.IndexAny(p, "*?[")
	if globIndex < 0 {
		return p
	}
	return path.Dir(p[:globIndex])
}

// SanitizeExternalStorageURI returns the external storage URI with with some
// secrets redacted, for use when showing these URIs in the UI, to provide some
// protection from shoulder-surfing. The param is still present -- just
// redacted -- to make it clearer that that value is indeed persisted interally.
// extraParams which should be scrubbed -- for params beyond those that the
// various cloud-storage URIs supported by this package know about -- can be
// passed allowing this function to be used to scrub other URIs too (such as
// non-cloudstorage changefeed sinks).n
func SanitizeExternalStorageURI(path string, extraParams []string) (string, error) {
	uri, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	if uri.Scheme == "experimental-workload" || uri.Scheme == "workload" || uri.Scheme == "null" {
		return path, nil
	}

	if uri.User != nil {
		if _, passwordSet := uri.User.Password(); passwordSet {
			uri.User = url.UserPassword(uri.User.Username(), RedactionMarker)
		}
	}

	params := uri.Query()
	for param := range params {
		if _, ok := redactedQueryParams[param]; ok {
			params.Set(param, RedactionMarker)
		} else {
			for _, p := range extraParams {
				if param == p {
					params.Set(param, RedactionMarker)
				}
			}
		}
	}

	uri.RawQuery = params.Encode()
	return uri.String(), nil
}

// RedactKMSURI redacts the Master Key ID and the ExternalStorage secret
// credentials.
func RedactKMSURI(kmsURI string) (string, error) {
	sanitizedKMSURI, err := SanitizeExternalStorageURI(kmsURI, nil)
	if err != nil {
		return "", err
	}

	// Redact the path which contains the KMS Master Key identifier.
	uri, err := url.ParseRequestURI(sanitizedKMSURI)
	if err != nil {
		return "", err
	}
	uri.Path = "/redacted"
	return uri.String(), nil
}

// JoinPathPreservingTrailingSlash wraps path.Join but preserves the trailing
// slash if there was one in the suffix.
//
// This is particularly important when the joined path is used as a prefix for
// listing. When listing, the suffix *after the listed prefix* of each file name
// is what is returned and, importantly, what is used when grouping using a
// delimiter. E.g. when using `/` as a delimiter to find what might be called the
// immediate children in a directory, we pass that directory's path *with a
// trailing slash* as the prefix, so that the children do not start with a slash
// and get grouped into nothing. Thus it is important that if we use path.Join
// to construct the prefix, we always preserve the trailing slash.
func JoinPathPreservingTrailingSlash(prefix, suffix string) string {
	out := path.Join(prefix, suffix)
	// path.Clean removes trailing slashes, so put it back if needed.
	if strings.HasSuffix(suffix, "/") {
		out += "/"
	}
	return out
}

// ConsumeURL is a helper struct which for "consuming" URL query
// parameters from the underlying URL.
type ConsumeURL struct {
	*url.URL
	q url.Values
}

// ConsumeParam returns the value of the parameter p from the underlying URL,
// and deletes the parameter from the URL.
func (u *ConsumeURL) ConsumeParam(p string) string {
	if u.q == nil {
		u.q = u.Query()
	}
	v := u.q.Get(p)
	u.q.Del(p)
	return v
}

// RemainingQueryParams returns the query parameters that have not been consumed
// from the underlying URL.
func (u *ConsumeURL) RemainingQueryParams() (res []string) {
	if u.q == nil {
		u.q = u.Query()
	}
	for p := range u.q {
		// The `COCKROACH_LOCALITY` parameter is supported for all External Storage
		// implementations and is not used when creating the External Storage, but
		// instead during backup/restore resolution. So, this parameter is not
		// "consumed" by the individual External Storage implementations in their
		// parse functions and so it will always show up in this method. We should
		// consider this param invisible when validating that all the passed in
		// query parameters are supported for an External Storage URI.
		if p == LocalityURLParam {
			continue
		}
		res = append(res, p)
	}
	return
}

func RegisterRedactedParams(params map[string]struct{}) {
	for param := range params {
		redactedQueryParams[param] = struct{}{}
	}
}
