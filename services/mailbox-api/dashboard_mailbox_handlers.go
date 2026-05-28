package main

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	"github.com/byte-v-forge/common-lib/httpx"

	"mailboxapi/pb"
)

func (s *dashboardServer) handleMailboxes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := int32(httpx.QueryInt(r, "limit", 100))
		authStatus := strings.TrimSpace(r.URL.Query().Get("auth_status"))
		if authStatus == "" {
			authStatus = strings.TrimSpace(r.URL.Query().Get("status"))
		}
		resp, err := s.mailboxClient.ListMailboxes(r.Context(), &pb.ListEmailMailboxesRequest{
			AuthStatus:  authStatus,
			ProviderKey: strings.TrimSpace(r.URL.Query().Get("provider_key")),
			Limit:       limit,
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeProtoJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		var req pb.UpsertEmailMailboxRequest
		if err := readProtoJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.GetMailbox() == nil || strings.TrimSpace(req.GetMailbox().GetEmailAddress()) == "" {
			writeError(w, http.StatusBadRequest, errors.New("mailbox.email_address is required"))
			return
		}
		req.Mailbox.EmailAddress = strings.TrimSpace(req.GetMailbox().GetEmailAddress())
		req.Mailbox.ProviderKey = strings.TrimSpace(req.GetMailbox().GetProviderKey())
		resp, err := s.mailboxClient.UpsertMailbox(r.Context(), &req)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		email := strings.ToLower(strings.TrimSpace(resp.GetMailbox().GetEmailAddress()))
		if email == "" {
			writeError(w, http.StatusBadGateway, errors.New("mailbox returned empty mailbox"))
			return
		}
		writeProtoJSON(w, http.StatusCreated, resp)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *dashboardServer) handleMailbox(w http.ResponseWriter, r *http.Request) {
	emailPath := strings.Trim(strings.TrimPrefix(r.URL.Path, "/mailboxes/"), "/")
	parts := strings.Split(emailPath, "/")
	emailPath = parts[0]
	email, err := url.PathUnescape(emailPath)
	if err != nil || strings.TrimSpace(email) == "" {
		writeError(w, http.StatusBadRequest, errors.New("email_address is required"))
		return
	}
	if len(parts) == 2 && parts[1] == "inbox" {
		s.handleMailboxStoredInbox(w, r, email)
		return
	}
	if len(parts) > 1 {
		writeError(w, http.StatusNotFound, errors.New("mailbox endpoint not found"))
		return
	}
	switch r.Method {
	case http.MethodDelete:
		resp, err := s.mailboxClient.DeleteMailbox(r.Context(), &pb.DeleteMailboxRequest{EmailAddress: strings.TrimSpace(email)})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeProtoJSON(w, http.StatusOK, resp)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *dashboardServer) handleMailboxStoredInbox(w http.ResponseWriter, r *http.Request, email string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	resp, err := s.mailboxClient.ListMailboxInbox(r.Context(), &mailboxv1.ListMailboxInboxRequest{
		EmailAddress:  strings.TrimSpace(email),
		Limit:         int32(httpx.QueryInt(r, "limit", 20)),
		ParserProfile: strings.TrimSpace(r.URL.Query().Get("parser_profile")),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if resp.GetErrorMessage() != "" {
		writeError(w, http.StatusBadGateway, errors.New(resp.GetErrorMessage()))
		return
	}
	writeProtoJSON(w, http.StatusOK, resp)
}
