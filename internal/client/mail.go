package client

import (
	"errors"
	"fmt"
)

// MailDomain mirrors apicp's internal/mail.MailDomain JSON shape.
type MailDomain struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	UnixUser string `json:"unix_user"`
	Protocols
	MaxEmailsPerHour int `json:"max_emails_per_hour"`
	// RelayID is the relay the domain's outgoing mail goes through ("" = the
	// server's default relay). apicpd before v0.8.0 does not return it.
	RelayID string `json:"relay_id,omitempty"`
	// Compression is how new mail of the domain is stored (off, zstd, lz4,
	// gz, bz2) and CompressionLevel its level (0 = the method's default);
	// nil on an apicpd from before stored-mail compression (v0.8.0), which
	// does not return them. CompressedMail: some stored mail may be compressed.
	Compression      *string `json:"compression,omitempty"`
	CompressionLevel int     `json:"compression_level"`
	CompressedMail   bool    `json:"compressed_mail"`
	Status           string  `json:"status"`
	Error            string  `json:"error,omitempty"`
}

// CompressionMethod is Compression with an old apicpd's missing field read as
// "off" (it has no compression).
func (d *MailDomain) CompressionMethod() string {
	if d.Compression == nil || *d.Compression == "" {
		return "off"
	}
	return *d.Compression
}

// ErrCompressionUnsupported is a compression setting against an apicpd that
// does not know it (before v0.8.0 it ignores the fields).
var ErrCompressionUnsupported = errors.New("this apicpd does not support stored-mail compression (compression needs apicp v0.8.0) - upgrade apicp, or leave compression off")

// Compression is a mail domain's compression setting to send; nil sends none.
type Compression struct {
	Method string `json:"compression"`
	Level  int    `json:"compression_level"`
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
	MaxEmailsPerHour int `json:"max_emails_per_hour"`
	// RelayID is only sent when set: an apicpd before v0.8.0 does not know it.
	RelayID string `json:"relay_id,omitempty"`
	*Compression
}

// CreateMailDomain implements POST /v1/mail/domains; relayID "" = the
// server's default relay (and the field is left out). comp is sent only when
// it is not off, so an apicpd from before compression (which ignores the
// fields) still creates the domain; asking it for compression is
// ErrCompressionUnsupported.
func (c *Client) CreateMailDomain(name string, p Protocols, maxEmailsPerHour int, relayID string, comp *Compression) (*MailDomain, error) {
	if comp != nil && comp.Method == "off" && comp.Level == 0 {
		comp = nil
	}
	var d MailDomain
	if err := c.Post("/v1/mail/domains", mailDomainCreateRequest{Name: name, Protocols: p, MaxEmailsPerHour: maxEmailsPerHour, RelayID: relayID, Compression: comp}, &d); err != nil {
		return nil, err
	}
	if comp != nil && d.Compression == nil {
		return &d, ErrCompressionUnsupported
	}
	return &d, nil
}

type mailDomainUpdateRequest struct {
	Protocols
	MaxEmailsPerHour int     `json:"max_emails_per_hour"`
	RelayID          *string `json:"relay_id,omitempty"`
	*Compression
}

// UpdateMailDomain implements PATCH /v1/mail/domains/{id}, sending every
// mutable field - relay_id only when relayID is not nil (a change of relay:
// "" = back to the default relay), so an update that does not touch the
// relay works on an apicpd before v0.8.0 too - and the compression setting
// when comp is not nil (only when it changes: apicp refuses a compression
// change while a compress- or decompress-existing job of the domain runs).
func (c *Client) UpdateMailDomain(id string, p Protocols, maxEmailsPerHour int, relayID *string, comp *Compression) (*MailDomain, error) {
	var d MailDomain
	if err := c.Patch("/v1/mail/domains/"+id, mailDomainUpdateRequest{Protocols: p, MaxEmailsPerHour: maxEmailsPerHour, RelayID: relayID, Compression: comp}, &d); err != nil {
		return nil, err
	}
	if comp != nil && d.Compression == nil && (comp.Method != "off" || comp.Level != 0) {
		return &d, ErrCompressionUnsupported
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
	// MaxEmailsPerHour is nil when apicpd predates per-mailbox sending
	// limits (it does not return the field at all) - see ErrMailboxTooOld.
	MaxEmailsPerHour *int64 `json:"max_emails_per_hour,omitempty"`
	Status           string `json:"status"`
	Error            string `json:"error,omitempty"`
	Password         string `json:"password,omitempty"` // only populated by Create/ResetMailboxPassword's response
}

// ErrMailboxTooOld is the error for a mailbox sending limit set against an
// apicpd that does not know the field (it ignores it silently).
var ErrMailboxTooOld = errors.New("this apicpd does not support per-mailbox sending limits (max_emails_per_hour needs the release after v0.6.18) - upgrade apicp, or leave max_emails_per_hour at 0")

type mailboxCreateRequest struct {
	LocalPart string `json:"local_part"`
	Protocols
	MaxEmailsPerHour int64 `json:"max_emails_per_hour,omitempty"`
}

func (c *Client) CreateMailbox(mailDomainID, localPart string, p Protocols, maxEmailsPerHour int64) (*Mailbox, error) {
	var m Mailbox
	path := fmt.Sprintf("/v1/mail/domains/%s/mailboxes", mailDomainID)
	if err := c.Post(path, mailboxCreateRequest{LocalPart: localPart, Protocols: p, MaxEmailsPerHour: maxEmailsPerHour}, &m); err != nil {
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

// mailboxUpdateRequest always carries both protocol switches: apicpd
// treats a PATCH body with no field it knows as a password reset, and one
// that predates max_emails_per_hour would see a limit-only body that way.
type mailboxUpdateRequest struct {
	Protocols
	MaxEmailsPerHour int64 `json:"max_emails_per_hour"`
}

// UpdateMailbox implements PATCH .../mailboxes/{id} with the protocol
// switches and the sending limit set - which changes only them (no
// password reset).
func (c *Client) UpdateMailbox(mailDomainID, id string, p Protocols, maxEmailsPerHour int64) (*Mailbox, error) {
	var m Mailbox
	path := fmt.Sprintf("/v1/mail/domains/%s/mailboxes/%s", mailDomainID, id)
	if err := c.Patch(path, mailboxUpdateRequest{Protocols: p, MaxEmailsPerHour: maxEmailsPerHour}, &m); err != nil {
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

// Forwarder mirrors apicp's internal/mail.Forwarder JSON shape. LocalPart
// "*" is the domain's catch-all.
type Forwarder struct {
	ID           string   `json:"id"`
	MailDomainID string   `json:"mail_domain_id"`
	LocalPart    string   `json:"local_part"`
	Email        string   `json:"email"`
	Destinations []string `json:"destinations"`
	Status       string   `json:"status"`
	Error        string   `json:"error,omitempty"`
}

type forwarderCreateRequest struct {
	LocalPart    string   `json:"local_part"`
	Destinations []string `json:"destinations"`
}

type forwarderUpdateRequest struct {
	Destinations []string `json:"destinations"`
}

func (c *Client) CreateForwarder(mailDomainID, localPart string, destinations []string) (*Forwarder, error) {
	var f Forwarder
	path := fmt.Sprintf("/v1/mail/domains/%s/forwarders", mailDomainID)
	if err := c.Post(path, forwarderCreateRequest{LocalPart: localPart, Destinations: destinations}, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func (c *Client) GetForwarder(mailDomainID, id string) (*Forwarder, error) {
	var f Forwarder
	path := fmt.Sprintf("/v1/mail/domains/%s/forwarders/%s", mailDomainID, id)
	if err := c.Get(path, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// UpdateForwarder implements PATCH .../forwarders/{id}, replacing the
// destination list.
func (c *Client) UpdateForwarder(mailDomainID, id string, destinations []string) (*Forwarder, error) {
	var f Forwarder
	path := fmt.Sprintf("/v1/mail/domains/%s/forwarders/%s", mailDomainID, id)
	if err := c.Patch(path, forwarderUpdateRequest{Destinations: destinations}, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func (c *Client) DeleteForwarder(mailDomainID, id string) error {
	return c.Delete(fmt.Sprintf("/v1/mail/domains/%s/forwarders/%s", mailDomainID, id))
}
