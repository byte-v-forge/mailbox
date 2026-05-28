package main

import (
	"errors"
	"net/http"
	"strings"

	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
)

func (s *dashboardServer) handleMailboxDomains(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		resp, err := s.mailboxClient.ListMailboxDomains(r.Context(), &mailboxv1.ListMailboxDomainsRequest{
			ProviderKey: requestProviderKey(r),
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
	case http.MethodPost:
		var req mailboxv1.SyncMailboxDomainsRequest
		if err := readProtoJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		resp, err := s.mailboxClient.SyncMailboxDomains(r.Context(), &req)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if resp.GetErrorMessage() != "" {
			writeError(w, http.StatusBadGateway, errors.New(resp.GetErrorMessage()))
			return
		}
		writeProtoJSON(w, http.StatusOK, resp)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *dashboardServer) handleMailboxProviderCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	resp, err := s.mailboxClient.ListMailboxProviderCapabilities(r.Context(), &mailboxv1.ListMailboxProviderCapabilitiesRequest{
		ProviderKey: requestProviderKey(r),
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

func requestProviderKey(r *http.Request) string {
	if r == nil {
		return ""
	}
	return normalizeMailboxProviderInput(strings.TrimSpace(r.URL.Query().Get("provider_key")))
}
