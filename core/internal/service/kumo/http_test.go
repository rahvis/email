package kumo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConnectionSuccessWithMethodNotAllowedInjectionAndMetricsOK(t *testing.T) {
	injectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer injectServer.Close()

	metricsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("# HELP kumo_test test\nkumo_test 1\n"))
	}))
	defer metricsServer.Close()

	store := &memoryConfigStore{cfg: defaultConfig()}
	cleanup := setTestHooks(store, testSecretProtector{})
	defer cleanup()

	result, err := TestConnection(context.Background(), TestConnectionInput{
		BaseURL:    injectServer.URL,
		InjectPath: "/api/inject/v1",
		MetricsURL: metricsServer.URL,
		AuthMode:   "bearer",
		AuthSecret: "test-token",
		TLSVerify:  true,
		TimeoutMS:  1000,
	})
	require.NoError(t, err)
	require.True(t, result.OK)
	require.True(t, result.Inject.OK)
	require.True(t, result.Metrics.OK)
}

func TestConnectionReportsAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	store := &memoryConfigStore{cfg: defaultConfig()}
	cleanup := setTestHooks(store, testSecretProtector{})
	defer cleanup()

	result, err := TestConnection(context.Background(), TestConnectionInput{
		BaseURL:    server.URL,
		InjectPath: "/api/inject/v1",
		AuthMode:   "bearer",
		AuthSecret: "bad-token",
		TLSVerify:  true,
		TimeoutMS:  1000,
	})
	require.NoError(t, err)
	require.False(t, result.OK)
	require.False(t, result.Inject.OK)
	require.Equal(t, http.StatusUnauthorized, result.Inject.StatusCode)
	require.Contains(t, result.Message, "authentication")
}

func TestConnectionSendsHMACHeadersToInjectAndMetrics(t *testing.T) {
	assertHMAC := func(t *testing.T, r *http.Request) {
		t.Helper()
		timestamp := r.Header.Get("X-BM-Kumo-Timestamp")
		signature := r.Header.Get("X-BM-Kumo-Signature")
		require.NotEmpty(t, timestamp)
		require.Equal(t, ComputeWebhookSignature("hmac-secret", timestamp, nil), signature)
	}

	injectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHMAC(t, r)
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer injectServer.Close()

	metricsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHMAC(t, r)
		_, _ = w.Write([]byte("# HELP kumo_test test\nkumo_test 1\n"))
	}))
	defer metricsServer.Close()

	store := &memoryConfigStore{cfg: defaultConfig()}
	cleanup := setTestHooks(store, testSecretProtector{})
	defer cleanup()

	result, err := TestConnection(context.Background(), TestConnectionInput{
		BaseURL:    injectServer.URL,
		InjectPath: "/api/inject/v1",
		MetricsURL: metricsServer.URL,
		AuthMode:   "hmac",
		AuthSecret: "hmac-secret",
		TLSVerify:  true,
		TimeoutMS:  1000,
	})
	require.NoError(t, err)
	require.True(t, result.OK)
	require.True(t, result.Inject.OK)
	require.True(t, result.Metrics.OK)
}

func TestConnectionHandlesTLSVerificationFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()

	store := &memoryConfigStore{cfg: defaultConfig()}
	cleanup := setTestHooks(store, testSecretProtector{})
	defer cleanup()

	result, err := TestConnection(context.Background(), TestConnectionInput{
		BaseURL:    server.URL,
		InjectPath: "/api/inject/v1",
		AuthMode:   "none",
		TLSVerify:  true,
		TimeoutMS:  1000,
	})
	require.NoError(t, err)
	require.False(t, result.OK)

	result, err = TestConnection(context.Background(), TestConnectionInput{
		BaseURL:    server.URL,
		InjectPath: "/api/inject/v1",
		AuthMode:   "none",
		TLSVerify:  false,
		TimeoutMS:  1000,
	})
	require.NoError(t, err)
	require.True(t, result.OK)
}

func TestConnectionHandlesTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := &memoryConfigStore{cfg: defaultConfig()}
	cleanup := setTestHooks(store, testSecretProtector{})
	defer cleanup()

	result, err := TestConnection(context.Background(), TestConnectionInput{
		BaseURL:    server.URL,
		InjectPath: "/api/inject/v1",
		AuthMode:   "none",
		TLSVerify:  true,
		TimeoutMS:  100,
	})
	require.NoError(t, err)
	require.False(t, result.OK)
	require.Contains(t, result.Message, "Client.Timeout")
}

func TestConnectionReportsMetricsFailure(t *testing.T) {
	injectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer injectServer.Close()

	metricsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer metricsServer.Close()

	store := &memoryConfigStore{cfg: defaultConfig()}
	cleanup := setTestHooks(store, testSecretProtector{})
	defer cleanup()

	result, err := TestConnection(context.Background(), TestConnectionInput{
		BaseURL:    injectServer.URL,
		InjectPath: "/api/inject/v1",
		MetricsURL: metricsServer.URL,
		AuthMode:   "none",
		TLSVerify:  true,
		TimeoutMS:  1000,
	})
	require.NoError(t, err)
	require.False(t, result.OK)
	require.True(t, result.Inject.OK)
	require.False(t, result.Metrics.OK)
	require.Equal(t, http.StatusInternalServerError, result.Metrics.StatusCode)
}
