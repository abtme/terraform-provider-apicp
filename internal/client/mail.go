package client

import "fmt"

// MailDomain mirrors apicp's internal/mail.MailDomain JSON shape.
type MailDomain struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	UnixUser string `json:"unix_user"`
	Protocols
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Protocols mirrors apicp's mail.Protocols: which login protocols are
// switched off (false = allowed). Always sent in full on create/update.
type Protocols struct {
	DisableIMAP bool `json:"disable_imap"`
	DisablePOP3 bool `json:"disable_pop3"`
}

type mailDomainCreateRequest struct {
	Name string `json:"name"`
	Protocols
}

func (c *Client) CreateMailDomain(name string, p Protocols) (*MailDomain, error) {
	var d MailDomain
	if err := c.Post("/v1/mail/domains", mailDomainCreateRequest{Name: name, Protocols: p}, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// SetMailDomainProtocols implements PATCH /v1/mail/domains/{id}.
func (c *Client) SetMailDomainProtocols(id string, p Protocols) (*MailDomain, error) {
	var d MailDomain
	if err := c.Patch("/v1/mail/domains/"+id, p, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) GetMailDomain(id string) (*MailDomain, error) {
	var d MailDomain
	if err := c.Get("/v1/mail/domains/"+id, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) DeleteMailDomain(id string) error {
	return c.Delete("/v1/mail/domains/" + id)
}

// Mailbox mirrors apicp's internal/mail.Mailbox JSON shape, plus the
// one-time Password field createMailboxResponse adds on create/reset.
type Mailbox struct {
	ID           string `json:"id"`
	MailDomainID string `json:"mail_domain_id"`
	LocalPart    string `json:"local_part"`
	Email        string `json:"email"`
	Protocols
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
	Password string `json:"password,omitempty"` // only populated by Create/ResetMailboxPassword's response
}

type mailboxCreateRequest struct {
	LocalPart string `json:"local_part"`
	Protocols
}

func (c *Client) CreateMailbox(mailDomainID, localPart string, p Protocols) (*Mailbox, error) {
	var m Mailbox
	path := fmt.Sprintf("/v1/mail/domains/%s/mailboxes", mailDomainID)
	if err := c.Post(path, mailboxCreateRequest{LocalPart: localPart, Protocols: p}, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) GetMailbox(mailDomainID, id string) (*Mailbox, error) {
	var m Mailbox
	path := fmt.Sprintf("/v1/mail/domains/%s/mailboxes/%s", mailDomainID, id)
	if err := c.Get(path, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// SetMailboxProtocols implements PATCH .../mailboxes/{id} with the
// protocol switches set - which changes only them (no password reset).
func (c *Client) SetMailboxProtocols(mailDomainID, id string, p Protocols) (*Mailbox, error) {
	var m Mailbox
	path := fmt.Sprintf("/v1/mail/domains/%s/mailboxes/%s", mailDomainID, id)
	if err := c.Patch(path, p, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ResetMailboxPassword implements PATCH .../mailboxes/{id} with an empty
// body, which apicp treats as a password reset.
func (c *Client) ResetMailboxPassword(mailDomainID, id string) (*Mailbox, error) {
	var m Mailbox
	path := fmt.Sprintf("/v1/mail/domains/%s/mailboxes/%s", mailDomainID, id)
	if err := c.Patch(path, struct{}{}, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) DeleteMailbox(mailDomainID, id string) error {
	return c.Delete(fmt.Sprintf("/v1/mail/domains/%s/mailboxes/%s", mailDomainID, id))
}
