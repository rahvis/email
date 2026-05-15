package kumo

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

type memoryConfigStore struct {
	cfg       *StoredConfig
	loadCount int
	saveCount int
}

func (s *memoryConfigStore) Load(ctx context.Context) (*StoredConfig, error) {
	s.loadCount++
	return cloneConfig(s.cfg), nil
}

func (s *memoryConfigStore) Save(ctx context.Context, cfg *StoredConfig) error {
	s.saveCount++
	s.cfg = cloneConfig(cfg)
	return nil
}

type testSecretProtector struct{}

func (testSecretProtector) Encrypt(ctx context.Context, plainText string) (string, error) {
	return "enc:" + base64.StdEncoding.EncodeToString([]byte(plainText)), nil
}

func (testSecretProtector) Decrypt(ctx context.Context, encrypted string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encrypted[4:])
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func TestUpdateConfigDoesNotExposeSecrets(t *testing.T) {
	store := &memoryConfigStore{cfg: defaultConfig()}
	cleanup := setTestHooks(store, testSecretProtector{})
	defer cleanup()

	publicConfig, err := UpdateConfig(context.Background(), UpdateConfigInput{
		Enabled:          true,
		CampaignsEnabled: true,
		APIEnabled:       true,
		BaseURL:          "https://email.example.com",
		InjectPath:       "/api/inject/v1",
		MetricsURL:       "https://email.example.com/metrics",
		TLSVerify:        true,
		AuthMode:         "bearer",
		AuthSecret:       "auth-token",
		WebhookSecret:    "webhook-token",
		TimeoutMS:        5000,
		DefaultPool:      "shared-default",
	})
	require.NoError(t, err)
	require.True(t, publicConfig.HasAuthSecret)
	require.True(t, publicConfig.HasWebhookSecret)
	require.NotContains(t, publicConfig.BaseURL, "auth-token")
	require.NotEqual(t, "auth-token", store.cfg.AuthSecretEncrypted)
	require.NotEqual(t, "webhook-token", store.cfg.WebhookSecretEncrypted)

	authSecret, err := GetAuthSecret(context.Background(), store.cfg)
	require.NoError(t, err)
	require.Equal(t, "auth-token", authSecret)
}

func TestUpdateConfigEmptySecretsKeepExisting(t *testing.T) {
	store := &memoryConfigStore{cfg: defaultConfig()}
	cleanup := setTestHooks(store, testSecretProtector{})
	defer cleanup()

	_, err := UpdateConfig(context.Background(), UpdateConfigInput{
		BaseURL:       "https://email.example.com",
		InjectPath:    "/api/inject/v1",
		AuthMode:      "bearer",
		AuthSecret:    "first-secret",
		WebhookSecret: "first-webhook",
		TimeoutMS:     5000,
	})
	require.NoError(t, err)
	firstAuth := store.cfg.AuthSecretEncrypted
	firstWebhook := store.cfg.WebhookSecretEncrypted

	_, err = UpdateConfig(context.Background(), UpdateConfigInput{
		BaseURL:    "https://email2.example.com",
		InjectPath: "/api/inject/v1",
		AuthMode:   "bearer",
		TimeoutMS:  5000,
	})
	require.NoError(t, err)
	require.Equal(t, firstAuth, store.cfg.AuthSecretEncrypted)
	require.Equal(t, firstWebhook, store.cfg.WebhookSecretEncrypted)
	require.Equal(t, "https://email2.example.com", store.cfg.BaseURL)
}

func TestValidateConfigInputRejectsInvalidValues(t *testing.T) {
	err := validateConfigInput(UpdateConfigInput{BaseURL: "ftp://example.com", TimeoutMS: 5000})
	require.Error(t, err)

	err = validateConfigInput(UpdateConfigInput{BaseURL: "https://example.com", InjectPath: "https://bad.example.com", TimeoutMS: 5000})
	require.Error(t, err)

	err = validateConfigInput(UpdateConfigInput{BaseURL: "https://example.com", AuthMode: "basic", TimeoutMS: 5000})
	require.Error(t, err)

	err = validateConfigInput(UpdateConfigInput{BaseURL: "https://example.com", TimeoutMS: 10})
	require.Error(t, err)
}

func TestConfigCacheInvalidatesAfterUpdate(t *testing.T) {
	store := &memoryConfigStore{cfg: &StoredConfig{
		BaseURL:    "https://cached.example.com",
		InjectPath: "/api/inject/v1",
		AuthMode:   "bearer",
		TimeoutMS:  5000,
		TLSVerify:  true,
	}}
	cleanup := setTestHooks(store, testSecretProtector{})
	defer cleanup()

	cfg, err := LoadConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "https://cached.example.com", cfg.BaseURL)

	store.cfg.BaseURL = "https://not-visible-until-invalidation.example.com"
	cfg, err = LoadConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "https://cached.example.com", cfg.BaseURL)
	require.Equal(t, 1, store.loadCount)

	_, err = UpdateConfig(context.Background(), UpdateConfigInput{
		BaseURL:    "https://updated.example.com",
		InjectPath: "/api/inject/v1",
		AuthMode:   "bearer",
		TimeoutMS:  5000,
		TLSVerify:  true,
	})
	require.NoError(t, err)

	cfg, err = LoadConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "https://updated.example.com", cfg.BaseURL)
	require.Equal(t, 1, store.saveCount)
}
