package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuth2 is a generic authorization-code provider. Google and Discord are
// configured through NewGoogle / NewDiscord; adding another provider is a
// matter of endpoints and a userinfo mapper.
type OAuth2 struct {
	name         string
	clientID     string
	clientSecret string
	authURL      string
	tokenURL     string
	userURL      string
	scopes       []string
	redirectURL  string
	extraAuth    url.Values
	mapUser      func(map[string]any) (Identity, error)
	HTTP         *http.Client
}

func (o *OAuth2) Name() string { return o.name }

func (o *OAuth2) AuthURL(state string) string {
	q := url.Values{}
	q.Set("client_id", o.clientID)
	q.Set("redirect_uri", o.redirectURL)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(o.scopes, " "))
	q.Set("state", state)
	for k, vs := range o.extraAuth {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	return o.authURL + "?" + q.Encode()
}

func (o *OAuth2) Exchange(ctx context.Context, code string) (Identity, error) {
	if code == "" {
		return Identity{}, errors.New("missing code")
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", o.redirectURL)
	form.Set("client_id", o.clientID)
	form.Set("client_secret", o.clientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := o.HTTP.Do(req)
	if err != nil {
		return Identity{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return Identity{}, fmt.Errorf("%s token exchange failed (%d)", o.name, res.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return Identity{}, fmt.Errorf("%s: no access token", o.name)
	}
	ureq, err := http.NewRequestWithContext(ctx, http.MethodGet, o.userURL, nil)
	if err != nil {
		return Identity{}, err
	}
	ureq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	ures, err := o.HTTP.Do(ureq)
	if err != nil {
		return Identity{}, err
	}
	defer ures.Body.Close()
	ubody, _ := io.ReadAll(io.LimitReader(ures.Body, 1<<20))
	if ures.StatusCode >= 300 {
		return Identity{}, fmt.Errorf("%s userinfo failed (%d)", o.name, ures.StatusCode)
	}
	var info map[string]any
	if err := json.Unmarshal(ubody, &info); err != nil {
		return Identity{}, err
	}
	id, err := o.mapUser(info)
	if err != nil {
		return Identity{}, err
	}
	id.Provider = o.name
	return id, nil
}

func str(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

// NewGoogle configures Google Sign-In (OpenID Connect userinfo).
func NewGoogle(clientID, clientSecret, redirectURL string) *OAuth2 {
	return &OAuth2{
		name: "google", clientID: clientID, clientSecret: clientSecret, redirectURL: redirectURL,
		authURL:   "https://accounts.google.com/o/oauth2/v2/auth",
		tokenURL:  "https://oauth2.googleapis.com/token",
		userURL:   "https://openidconnect.googleapis.com/v1/userinfo",
		scopes:    []string{"openid", "email", "profile"},
		extraAuth: url.Values{"prompt": {"select_account"}},
		mapUser: func(m map[string]any) (Identity, error) {
			if v, ok := m["email_verified"].(bool); ok && !v {
				return Identity{}, errors.New("google email not verified")
			}
			return Identity{Subject: str(m, "sub"), Email: str(m, "email"), Name: str(m, "name")}, nil
		},
		HTTP: &http.Client{Timeout: 15 * time.Second},
	}
}

// NewDiscord configures Discord OAuth2.
func NewDiscord(clientID, clientSecret, redirectURL string) *OAuth2 {
	return &OAuth2{
		name: "discord", clientID: clientID, clientSecret: clientSecret, redirectURL: redirectURL,
		authURL:  "https://discord.com/oauth2/authorize",
		tokenURL: "https://discord.com/api/oauth2/token",
		userURL:  "https://discord.com/api/users/@me",
		scopes:   []string{"identify", "email"},
		mapUser: func(m map[string]any) (Identity, error) {
			if v, ok := m["verified"].(bool); ok && !v {
				return Identity{}, errors.New("discord email not verified")
			}
			name := str(m, "global_name")
			if name == "" {
				name = str(m, "username")
			}
			return Identity{Subject: str(m, "id"), Email: str(m, "email"), Name: name}, nil
		},
		HTTP: &http.Client{Timeout: 15 * time.Second},
	}
}
