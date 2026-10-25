package provider

import (
	"fmt"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// unknownFieldDetail explains apicpd's 422 for a field it does not know
// (apicp v0.7.0 on refuses unknown fields on the backup settings and
// database endpoints instead of ignoring them). The provider's argument has
// the field's name. ok is false for any other error.
func unknownFieldDetail(err error) (string, bool) {
	field, ok := client.UnknownField(err)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("This apicpd does not support %q, so it refused the request and changed nothing. "+
		"Upgrade apicpd to a version that knows it, or remove the %s argument from the configuration. (%v)", field, field, err), true
}

// errorDetail is err's text for a diagnostic, with unknownFieldDetail's
// explanation when it applies.
func errorDetail(err error) string {
	if d, ok := unknownFieldDetail(err); ok {
		return d
	}
	return err.Error()
}
