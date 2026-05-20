package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	tokenURL    = "https://auth.puregym.com/connect/token"
	sessionsURL = "https://capi.puregym.com/api/v2/gymSessions/gym"

	// Refresh slightly before stated expiry so we never serve a stale token.
	tokenRefreshBuffer = 60 * time.Second
)

var errUnauthorized = fmt.Errorf("unauthorized")

type Credentials struct {
	email string
	pin   string
}

type PureGymClient struct {
	creds Credentials
	http  *http.Client

	token     string
	expiresAt time.Time
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type GymSessionResponse struct {
	TotalPeopleInGym int `json:"TotalPeopleInGym"`
}

func NewPureGymClient(creds Credentials) *PureGymClient {
	return &PureGymClient{
		creds: creds,
		http:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *PureGymClient) tokenExpired() bool {
	return c.token == "" || time.Now().After(c.expiresAt.Add(-tokenRefreshBuffer))
}

func (c *PureGymClient) getToken(ctx context.Context) error {
	form := url.Values{
		"grant_type": {"password"},
		"username":   {c.creds.email},
		"password":   {c.creds.pin},
		"scope":      {"pgcapi"},
		"client_id":  {"ro.client"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token endpoint returned %s", resp.Status)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("decode token: %w", err)
	}

	c.token = tokenResp.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return nil
}

func (c *PureGymClient) getSession(ctx context.Context, gymId int) (int, error) {
	url := fmt.Sprintf("%s?gymId=%d", sessionsURL, gymId)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		var body GymSessionResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return 0, fmt.Errorf("decode sessions: %w", err)
		}
		return body.TotalPeopleInGym, nil
	case http.StatusUnauthorized:
		return 0, errUnauthorized
	default:
		return 0, fmt.Errorf("sessions endpoint returned %s", resp.Status)
	}
}

func (c *PureGymClient) FetchOccupancy(ctx context.Context, gymID int) (int, error) {
	if c.tokenExpired() {
		if err := c.getToken(ctx); err != nil {
			return 0, fmt.Errorf("refresh token: %w", err)
		}
	}

	count, err := c.getSession(ctx, gymID)
	if errors.Is(err, errUnauthorized) {
		if err := c.getToken(ctx); err != nil {
			return 0, fmt.Errorf("refresh after 401: %w", err)
		}
		return c.getSession(ctx, gymID)
	}
	return count, err
}
