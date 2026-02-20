package auth

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDCProvider struct {
	provider    *oidc.Provider
	verifier    *oidc.IDTokenVerifier
	oauth2Cfg   oauth2.Config
	adminRole   string
}

type Claims struct {
	Subject string   `json:"sub"`
	Email   string   `json:"email"`
	Name    string   `json:"name"`
	Roles   []string `json:"roles"`
}

func NewOIDCProvider(baseURL string) (*OIDCProvider, error) {
	ctx := context.Background()

	tenantID := os.Getenv("AZURE_TENANT_ID")
	clientID := os.Getenv("AZURE_CLIENT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	adminRole := os.Getenv("AZURE_ADMIN_ROLE")

	if tenantID == "" || clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("AZURE_TENANT_ID, AZURE_CLIENT_ID, and AZURE_CLIENT_SECRET are required")
	}

	if adminRole == "" {
		adminRole = "InventoryAdmin"
	}

	issuerURL := fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", tenantID)

	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  baseURL + "/auth/callback",
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: clientID})

	return &OIDCProvider{
		provider:  provider,
		verifier:  verifier,
		oauth2Cfg: oauth2Cfg,
		adminRole: adminRole,
	}, nil
}

func (p *OIDCProvider) AuthCodeURL(state string) string {
	return p.oauth2Cfg.AuthCodeURL(state)
}

func (p *OIDCProvider) Exchange(ctx context.Context, code string) (*Claims, error) {
	token, err := p.oauth2Cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in response")
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	var claims struct {
		Email string   `json:"email"`
		Name  string   `json:"name"`
		Roles []string `json:"roles"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	// Also try preferred_username if email is empty
	if claims.Email == "" {
		var altClaims struct {
			PreferredUsername string `json:"preferred_username"`
		}
		idToken.Claims(&altClaims)
		claims.Email = altClaims.PreferredUsername
	}

	return &Claims{
		Subject: idToken.Subject,
		Email:   claims.Email,
		Name:    claims.Name,
		Roles:   claims.Roles,
	}, nil
}

func (p *OIDCProvider) IsAdmin(claims *Claims) bool {
	for _, role := range claims.Roles {
		if strings.EqualFold(role, p.adminRole) {
			return true
		}
	}
	return false
}
