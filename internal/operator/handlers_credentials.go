package operator

import (
	"encoding/json"
	"net/http"
)

// handleCredentialsList returns all credential names and the backend type.
func (s *Server) handleCredentialsList(w http.ResponseWriter, r *http.Request) {
	if s.Credentials == nil {
		jsonErr(w, 500, "credentials backend not configured")
		return
	}

	names, err := s.Credentials.List(r.Context())
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if names == nil {
		names = []string{}
	}

	jsonOK(w, map[string]any{
		"names":   names,
		"backend": s.Credentials.Type(),
	})
}

// handleCredentialGet returns metadata about a credential (exists, length) — not the raw value.
func (s *Server) handleCredentialGet(w http.ResponseWriter, r *http.Request) {
	if s.Credentials == nil {
		jsonErr(w, 500, "credentials backend not configured")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		jsonErr(w, 400, "credential name is required")
		return
	}

	val, err := s.Credentials.Get(r.Context(), name)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}

	jsonOK(w, map[string]any{
		"name":   name,
		"exists": true,
		"length": len(val),
	})
}

// handleCredentialSet stores a credential value.
func (s *Server) handleCredentialSet(w http.ResponseWriter, r *http.Request) {
	if s.Credentials == nil {
		jsonErr(w, 500, "credentials backend not configured")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		jsonErr(w, 400, "credential name is required")
		return
	}

	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if body.Value == "" {
		jsonErr(w, 400, "value is required")
		return
	}

	if err := s.Credentials.Set(r.Context(), name, body.Value); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	jsonOK(w, map[string]string{"status": "ok", "name": name})
}

// handleCredentialDelete removes a credential.
func (s *Server) handleCredentialDelete(w http.ResponseWriter, r *http.Request) {
	if s.Credentials == nil {
		jsonErr(w, 500, "credentials backend not configured")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		jsonErr(w, 400, "credential name is required")
		return
	}

	if err := s.Credentials.Delete(r.Context(), name); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	jsonOK(w, map[string]string{"status": "ok", "name": name})
}
