// Copyright © 2018 Heptio
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
	"testing"
)

// HS256-signed JWT with claims: iss=GangwayTest, sub=gangway@heptio.com, etc.
const testHMACToken = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJHYW5nd2F5VGVzdCIsImlhdCI6MTU0MDA0NjM0NywiZXhwIjoxODg3MjAxNTQ3LCJhdWQiOiJnYW5nd2F5LmhlcHRpby5jb20iLCJzdWIiOiJnYW5nd2F5QGhlcHRpby5jb20iLCJHaXZlbk5hbWUiOiJHYW5nIiwiU3VybmFtZSI6IldheSIsIkVtYWlsIjoiZ2FuZ3dheUBoZXB0aW8uY29tIiwiR3JvdXBzIjoiZGV2LGFkbWluIn0.zNG4Dnxr76J0p4phfsAUYWunioct0krkMiunMynlQsU"

func TestUnsafeVerifier(t *testing.T) {
	tests := map[string]struct {
		rawToken    string
		wantIssuer  string
		wantSub     string
		expectError bool
	}{
		"valid HMAC token": {
			rawToken:   testHMACToken,
			wantIssuer: "GangwayTest",
			wantSub:    "gangway@heptio.com",
		},
		"malformed token": {
			rawToken:    "notavalidjwt",
			expectError: true,
		},
		"empty token": {
			rawToken:    "",
			expectError: true,
		},
	}

	v := &UnsafeVerifier{}
	ctx := context.Background()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tok, err := v.Verify(ctx, tc.rawToken)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok.Issuer() != tc.wantIssuer {
				t.Errorf("Issuer: want %q, got %q", tc.wantIssuer, tok.Issuer())
			}

			var claims map[string]interface{}
			if err := tok.Claims(&claims); err != nil {
				t.Fatalf("Claims() error: %v", err)
			}
			if sub, _ := claims["sub"].(string); sub != tc.wantSub {
				t.Errorf("sub claim: want %q, got %q", tc.wantSub, sub)
			}
		})
	}
}
