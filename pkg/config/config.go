// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package config defines the environment baased configuration for this server.
package config

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/exposure-notifications-server/pkg/base64util"
	"github.com/google/exposure-notifications-server/pkg/secrets"
	"github.com/google/exposure-notifications-verification-server/pkg/database"

	firebase "firebase.google.com/go"
	"github.com/sethvargo/go-envconfig/pkg/envconfig"
)

// New returns the environment config for the server. Only needs to be called once
// per instance, but may be called multiple times.
func New(ctx context.Context) (*Config, error) {
	return NewWith(ctx, envconfig.OsLookuper())
}

// NewWith creates a new config with the given lookuper for parsing config.
func NewWith(ctx context.Context, l envconfig.Lookuper) (*Config, error) {
	// Build a list of mutators. This list will grow as we initialize more of the
	// configuration, such as the secret manager.
	var mutatorFuncs []envconfig.MutatorFunc

	{
		// Load the secret manager configuration first - this needs to be loaded first
		// because other processors may need secrets.
		var smConfig secrets.Config
		if err := envconfig.ProcessWith(ctx, &smConfig, l); err != nil {
			return nil, fmt.Errorf("unable to process secret configuration: %w", err)
		}

		sm, err := secrets.SecretManagerFor(ctx, smConfig.SecretManagerType)
		if err != nil {
			return nil, fmt.Errorf("unable to connect to secret manager: %w", err)
		}

		// Enable caching, if a TTL was provided.
		if ttl := smConfig.SecretCacheTTL; ttl > 0 {
			sm, err = secrets.WrapCacher(ctx, sm, ttl)
			if err != nil {
				return nil, fmt.Errorf("unable to create secret manager cache: %w", err)
			}
		}

		// Update the mutators to process secrets.
		mutatorFuncs = append(mutatorFuncs, secrets.Resolver(sm, &smConfig))
	}

	// Parse the main configuration.
	var config Config
	if err := envconfig.ProcessWith(ctx, &config, l, mutatorFuncs...); err != nil {
		return nil, err
	}

	// For the, when inserting into the javascript, gets escaped and becomes unusable.
	config.Firebase.DatabaseURL = strings.ReplaceAll(config.Firebase.DatabaseURL, "https://", "")

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

// Config represents the environment based config for the server.
type Config struct {
	Firebase FirebaseConfig
	Database database.Config

	Port int `env:"PORT,default=8080"`

	// Login Config
	SessionCookieDuration time.Duration `env:"SESSION_DURATION,default=24h"`
	RevokeCheckPeriod     time.Duration `env:"REVOKE_CHECK_DURATION,default=5m"`

	// CSRF Secret Key. Must be 32-bytes. Can be generated with tools/gen-secret
	// Use the syntax of secret:// to pull the secret from secret manager.
	// We assume the secret itself is base64 encoded. Use CSRFKey() to transform to bytes.
	CSRFAuthKey string `env:"CSRF_AUTH_KEY,required"`

	// Application Config
	ServerName          string        `env:"SERVER_NAME,default=Diagnosis Verification Server"`
	CodeDuration        time.Duration `env:"CODE_DURATION,default=1h"`
	CodeDigits          uint          `env:"CODE_DIGITS,default=8"`
	CollisionRetryCount uint          `env:"COLISSION_RETRY_COUNT,default=6"`
	AllowedTestAge      time.Duration `env:"ALLOWRD_PAST_TEST_DAYS,default=336h"` // 336h is 14 days.
	APIKeyCacheDuration time.Duration `env:"API_KEY_CACHE_DURATION,default=5m"`
	RateLimit           uint64        `env:"RATE_LIMIT,default=60"`

	// Verification Token Config
	// Currently this does not easily support rotation. TODO(mikehelmick) - add support.
	VerificationTokenDuration time.Duration `env:"VERIFICATION_TOKEN_DURATION,default=24h"`
	TokenSigningKey           string        `env:"TOKEN_SIGNING_KEY,required"`
	TokenSigningKeyID         string        `env:"TOKEN_SIGNING_KEY_ID,default=v1"`
	TokenIssuer               string        `env:"TOKEN_ISSUER,default=diagnosis-verification-example"`

	// Verification certificate config
	PublicKeyCacheDuration  time.Duration `env:"PUBLIC_KEY_CACHE_DURATION,default=15m"`
	CertificateSigningKey   string        `env:"CERTIFICATE_SIGNING_KEY,required"`
	CertificateSigningKeyID string        `env:"CERTIFICATE_SIGNING_KEY_ID,default=v1"`
	CertificateIssuer       string        `env:"CERTIFICATE_ISSUER,default=diagnosis-verification-example"`
	CertificateAudience     string        `env:"CERTIFICATE_AUDIENCE,default=exposure-notifications-server"`
	CertificateDuration     time.Duration `env:"CERTIFICATE_DURATION,default=15m"`

	// Cleanup config
	CleanupPeriod           time.Duration `env:"CLEANUP_PERIOD,default=15m"`
	DisabledUserMaxAge      time.Duration `env:"DIABLED_USER_MAX_AGE,default=336h"`
	VerificationCodeMaxAge  time.Duration `env:"VERIFICATION_CODE_MAX_AGE,default=24h"`
	VerificationTokenMaxAge time.Duration `env:"VERIFICATION_TOKEN_MAX_AGE,default=24h"`

	AssetsPath string `env:"ASSETS_PATH,default=./cmd/server/assets"`

	// If Dev mode is true, cookies aren't required to be sent over secure channels.
	// This includes CSRF protection base cookie. You want this false in production (the default).
	DevMode bool `env:"DEV_MODE"`
}

func (c *Config) CSRFKey() ([]byte, error) {
	key, err := base64util.DecodeString(c.CSRFAuthKey)
	if err != nil {
		return nil, fmt.Errorf("error decoding CSRF_AUTH_KEY: %v", err)
	}
	if l := len(key); l != 32 {
		return nil, fmt.Errorf("CSRF_AUTH_KEY is not 32 bytes, got: %v", l)
	}
	return key, nil
}

func checkPositiveDuration(d time.Duration, name string) error {
	if d < 0 {
		return fmt.Errorf("%v must be a positive duration, got: %v", name, d)
	}
	return nil
}

func (c *Config) Validate() error {
	fields := []struct {
		Var  time.Duration
		Name string
	}{
		{c.SessionCookieDuration, "SESSION_DUATION"},
		{c.RevokeCheckPeriod, "REVOKE_CHECK_DURATION"},
		{c.CodeDuration, "CODE_DURATION"},
		{c.AllowedTestAge, "ALLOWED_PAST_TEST_DAYS"},
		{c.APIKeyCacheDuration, "API_KEY_CACHE_DURATION"},
		{c.VerificationCodeMaxAge, "VERIFICATION_TOKEN_DURATION"},
		{c.PublicKeyCacheDuration, "PUBLIC_KEY_CACHE_DURATION"},
		{c.CleanupPeriod, "CLEANUP_PERIOD"},
		{c.DisabledUserMaxAge, "DISABLED_USER_MAX_AGE"},
		{c.VerificationCodeMaxAge, "VERIFICATION_CODE_MAX_AGE"},
		{c.VerificationTokenMaxAge, "VERIFICATION_TOKEN_MAX_AGE"},
	}

	for _, f := range fields {
		if err := checkPositiveDuration(f.Var, f.Name); err != nil {
			return err
		}
	}

	return nil
}

// FirebaseConfig represents configuration specific to firebase auth.
type FirebaseConfig struct {
	APIKey          string `env:"FIREBASE_API_KEY,required"`
	AuthDomain      string `env:"FIREBASE_AUTH_DOMAIN,required"`
	DatabaseURL     string `env:"FIREBASE_DATABASE_URL,required"`
	ProjectID       string `env:"FIREBASE_PROJECT_ID,required"`
	StorageBucket   string `env:"FIREBASE_STORAGE_BUCKET,required"`
	MessageSenderID string `env:"FIREBASE_MESSAGE_SENDER_ID,required"`
	AppID           string `env:"FIREBASE_APP_ID,required"`
	MeasurementID   string `env:"FIREBASE_MEASUREMENT_ID,required"`
}

// FirebaseConfig returns the firebase SDK config based on the local env config.
func (c *Config) FirebaseConfig() *firebase.Config {
	return &firebase.Config{
		DatabaseURL:   c.Firebase.DatabaseURL,
		ProjectID:     c.Firebase.ProjectID,
		StorageBucket: c.Firebase.StorageBucket,
	}
}
