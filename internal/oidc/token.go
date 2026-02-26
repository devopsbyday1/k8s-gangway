// Copyright © 2017 Heptio
// Copyright © 2017 Craig Tracey
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package oidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OAuth2Token is an interface which is used when exchanging an id_token for an access token
type OAuth2Token interface {
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
}

// Token is an implementation of OAuth2Token Interface
type Token struct {
	OAuth2Cfg *oauth2.Config
}

// Exchange takes an oauth2 auth token and exchanges for an id_token
func (t *Token) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return t.OAuth2Cfg.Exchange(ctx, code)
}

// IDToken represents a verified or parsed OIDC ID token
type IDToken interface {
	// Claims unmarshals the raw JSON payload of the token into dest
	Claims(dest interface{}) error
	// Issuer returns the token issuer URL
	Issuer() string
}

// Verifier is the interface for OIDC token verification
type Verifier interface {
	Verify(ctx context.Context, rawIDToken string) (IDToken, error)
}

// ProviderVerifier uses go-oidc/v3 for full cryptographic OIDC token verification.
// It validates the issuer, audience, expiry, and signature via JWKS.
type ProviderVerifier struct {
	v *gooidc.IDTokenVerifier
}

// NewProviderVerifier initialises an OIDC provider via discovery and returns a Verifier.
func NewProviderVerifier(ctx context.Context, issuerURL, clientID string) (*ProviderVerifier, error) {
	provider, err := gooidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to discover OIDC provider at %s: %w", issuerURL, err)
	}
	v := provider.Verifier(&gooidc.Config{ClientID: clientID})
	return &ProviderVerifier{v: v}, nil
}

type providerIDToken struct {
	t *gooidc.IDToken
}

func (t *providerIDToken) Claims(dest interface{}) error { return t.t.Claims(dest) }
func (t *providerIDToken) Issuer() string                { return t.t.Issuer }

// Verify verifies the raw ID token using the OIDC provider's JWKS.
func (pv *ProviderVerifier) Verify(ctx context.Context, rawIDToken string) (IDToken, error) {
	t, err := pv.v.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, err
	}
	return &providerIDToken{t: t}, nil
}

// UnsafeVerifier parses a JWT payload without signature verification.
// It is intended only for testing and backward-compatibility fallback.
// Do NOT use in production without configuring a proper IssuerURL.
type UnsafeVerifier struct{}

type unsafeIDToken struct {
	claims map[string]interface{}
	issuer string
}

func (t *unsafeIDToken) Claims(dest interface{}) error {
	b, err := json.Marshal(t.claims)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

func (t *unsafeIDToken) Issuer() string { return t.issuer }

// Verify decodes the JWT payload without validating the signature.
func (uv *UnsafeVerifier) Verify(_ context.Context, rawIDToken string) (IDToken, error) {
	parts := strings.Split(rawIDToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed JWT: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JWT claims: %w", err)
	}

	issuer, _ := claims["iss"].(string)
	return &unsafeIDToken{claims: claims, issuer: issuer}, nil
}
