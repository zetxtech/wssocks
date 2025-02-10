package wssocks

import (
	"encoding/json"
	"net/http"
	"strings"
)

// APIHandler handles HTTP API requests for WSSocksServer
type APIHandler struct {
	server *WSSocksServer
	apiKey string
}

// NewAPIHandler creates a new API handler for the given server
func NewAPIHandler(server *WSSocksServer, apiKey string) *APIHandler {
	return &APIHandler{
		server: server,
		apiKey: apiKey,
	}
}

// RegisterHandlers registers API endpoints with the provided mux
func (h *APIHandler) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/token/", h.handleToken)
	mux.HandleFunc("/api/status", h.handleStatus)
}

// TokenRequest represents a request to create a new token
type TokenRequest struct {
	Type     string `json:"type"`     // "forward" or "reverse"
	Token    string `json:"token"`    // Optional: specific token to use
	Port     int    `json:"port"`     // Optional: specific port for reverse proxy
	Username string `json:"username"` // Optional: SOCKS auth username
	Password string `json:"password"` // Optional: SOCKS auth password
}

// TokenResponse represents the response for token operations
type TokenResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token,omitempty"`
	Port    int    `json:"port,omitempty"`
	Error   string `json:"error,omitempty"`
}

// StatusResponse represents the server status
type StatusResponse struct {
	Version string        `json:"version"`
	Tokens  []interface{} `json:"tokens"`
}

// TokenStatus represents the status of a token
type TokenStatus struct {
	Token        string `json:"token"`
	Type         string `json:"type"` // "forward" or "reverse"
	ClientsCount int    `json:"clients_count"`
}

// ReverseTokenStatus represents the status of a reverse token
type ReverseTokenStatus struct {
	TokenStatus
	Port int `json:"port"`
}

// checkAPIKey verifies the API key in the request header
func (h *APIHandler) checkAPIKey(w http.ResponseWriter, r *http.Request) bool {
	providedKey := r.Header.Get("X-API-Key")
	if providedKey != h.apiKey {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(TokenResponse{
			Success: false,
			Error:   "invalid API key",
		})
		return false
	}
	return true
}

func (h *APIHandler) handleToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !h.checkAPIKey(w, r) {
		return
	}

	// Handle token deletion
	if r.Method == http.MethodDelete {
		token := strings.TrimPrefix(r.URL.Path, "/api/token/")
		if token == "" {
			json.NewEncoder(w).Encode(TokenResponse{
				Success: false,
				Error:   "token not specified",
			})
			return
		}

		success := h.server.RemoveToken(token)
		json.NewEncoder(w).Encode(TokenResponse{
			Success: success,
			Token:   token,
		})
		return
	}

	// Handle token creation
	if r.Method == http.MethodPost {
		var req TokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(TokenResponse{
				Success: false,
				Error:   "invalid request body",
			})
			return
		}

		switch req.Type {
		case "forward":
			token := h.server.AddForwardToken(req.Token)
			json.NewEncoder(w).Encode(TokenResponse{
				Success: true,
				Token:   token,
			})

		case "reverse":
			opts := &ReverseTokenOptions{
				Token:    req.Token,
				Port:     req.Port,
				Username: req.Username,
				Password: req.Password,
			}
			token, port := h.server.AddReverseToken(opts)
			if port == 0 {
				json.NewEncoder(w).Encode(TokenResponse{
					Success: false,
					Error:   "failed to allocate port",
				})
				return
			}
			json.NewEncoder(w).Encode(TokenResponse{
				Success: true,
				Token:   token,
				Port:    port,
			})

		default:
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(TokenResponse{
				Success: false,
				Error:   "invalid token type",
			})
		}
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (h *APIHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !h.checkAPIKey(w, r) {
		return
	}

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	h.server.mu.RLock()
	tokens := make([]interface{}, 0)
	for token, port := range h.server.tokens {
		tokens = append(tokens, ReverseTokenStatus{
			TokenStatus: TokenStatus{
				Token:        token,
				Type:         "reverse",
				ClientsCount: h.server.GetTokenClientCount(token),
			},
			Port: port,
		})
	}
	for token := range h.server.forwardTokens {
		tokens = append(tokens, TokenStatus{
			Token:        token,
			Type:         "forward",
			ClientsCount: h.server.GetTokenClientCount(token),
		})
	}
	h.server.mu.RUnlock()

	json.NewEncoder(w).Encode(StatusResponse{
		Version: Version,
		Tokens:  tokens,
	})
}
