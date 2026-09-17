package client

import "fmt"

// TOTPEnrollment mirrors apicp's POST /v1/accounts/{id}/totp response -
// Secret/OTPAuthURL are only ever returned by that one call (PLAN.md §8
// phase 10). GET only ever reports whether enrollment exists, never the
// secret - see EnableTOTP's own doc comment.
type TOTPEnrollment struct {
	AccountID  string `json:"-"`
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauth_url"`
}

// EnableTOTP implements POST /v1/accounts/{id}/totp - generates a fresh
// secret every call, which is also how an admin resets a lost/
// misconfigured authenticator (there's no separate "reset" endpoint).
func (c *Client) EnableTOTP(accountID string) (*TOTPEnrollment, error) {
	var e TOTPEnrollment
	if err := c.Post(fmt.Sprintf("/v1/accounts/%s/totp", accountID), nil, &e); err != nil {
		return nil, err
	}
	e.AccountID = accountID
	return &e, nil
}

// GetTOTPStatus implements GET /v1/accounts/{id}/totp.
func (c *Client) GetTOTPStatus(accountID string) (bool, error) {
	var out struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.Get(fmt.Sprintf("/v1/accounts/%s/totp", accountID), &out); err != nil {
		return false, err
	}
	return out.Enabled, nil
}

func (c *Client) DisableTOTP(accountID string) error {
	return c.Delete(fmt.Sprintf("/v1/accounts/%s/totp", accountID))
}
