package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/mrramdn/tenggat/api/internal/mcpserver"
	"github.com/mrramdn/tenggat/api/internal/store"
)

const mcpPerMinute = 120

func (s *Server) mcpHandler(api http.Handler) http.HandlerFunc {
	h := mcpserver.New(mcpserver.Config{
		API:                 api,
		Verify:              s.verifyToken,
		Log:                 s.Log,
		ResourceMetadataURL: s.MCPResourceMetadataURL,
	})
	return s.limitTo("mcp", mcpPerMinute, h.ServeHTTP)
}

func (s *Server) verifyToken(ctx context.Context, raw string, _ *http.Request) (*mcpauth.TokenInfo, error) {
	u, tok, err := s.lookupToken(ctx, raw)
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("%w: token akses tidak dikenal", mcpauth.ErrInvalidToken)
	}
	if err != nil {
		return nil, err
	}
	var exp time.Time
	if tok.Kedaluwarsa != nil {
		exp = *tok.Kedaluwarsa
	}
	return &mcpauth.TokenInfo{
		UserID:     u.ID,
		Scopes:     []string{tok.Cakupan},
		Expiration: exp,
		Extra:      map[string]any{"token_id": tok.ID},
	}, nil
}
