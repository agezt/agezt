// SPDX-License-Identifier: MIT

// Email channel: IMAP transport (dialIMAP + pollIMAP).
// Code extracted from inbound.go during the Day-125 god-file split.
// Public API unchanged.
package email

import (
	"context"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

func (c *Channel) dialIMAP() (*imapclient.Client, error) {
	switch c.inboxTLS {
	case "starttls":
		return imapclient.DialStartTLS(c.inboxAddr, nil)
	case "none":
		return imapclient.DialInsecure(c.inboxAddr, nil)
	default:
		return imapclient.DialTLS(c.inboxAddr, nil)
	}
}

func (c *Channel) pollIMAP(_ context.Context) ([]inboundMail, error) {
	cl, err := c.dialIMAP()
	if err != nil {
		return nil, err
	}
	defer cl.Close()
	if err := cl.Login(c.inboxUser, c.inboxPass).Wait(); err != nil {
		return nil, err
	}
	defer cl.Logout().Wait()
	if _, err := cl.Select("INBOX", nil).Wait(); err != nil {
		return nil, err
	}
	data, err := cl.UIDSearch(&imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagSeen}}, nil).Wait()
	if err != nil {
		return nil, err
	}
	uids := data.AllUIDs()
	if len(uids) == 0 {
		return nil, nil
	}
	set := imap.UIDSetNum(uids...)
	section := &imap.FetchItemBodySection{}
	msgs, err := cl.Fetch(set, &imap.FetchOptions{
		Envelope:    true,
		BodySection: []*imap.FetchItemBodySection{section},
	}).Collect()
	if err != nil {
		return nil, err
	}
	var out []inboundMail
	for _, mb := range msgs {
		raw := mb.FindBodySection(section)
		if len(raw) == 0 {
			continue
		}
		if len(raw) > maxInboxBytes {
			raw = raw[:maxInboxBytes]
		}
		if m, ok := parseMail(raw); ok {
			out = append(out, m)
		}
	}
	// Mark fetched messages \Seen so they aren't reprocessed next poll.
	_ = cl.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Flags: []imap.Flag{imap.FlagSeen}, Silent: true}, nil).Close()
	return out, nil
}

// --- POP3 (stdlib net/textproto) ------------------------------------------

// popConn is a minimal POP3 session.
