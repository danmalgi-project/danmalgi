package test

import (
	"context"
	"testing"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/server/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserID_Present(t *testing.T) {
	ctx := context.WithValue(context.Background(), middleware.UID{}, int64(99))

	userID, err := middleware.GetUserID(ctx)

	require.NoError(t, err)
	assert.Equal(t, int64(99), userID)
}

func TestGetUserID_Missing(t *testing.T) {
	_, err := middleware.GetUserID(context.Background())

	require.Error(t, err)
	customErr, ok := err.(*domain.CustomError)
	require.True(t, ok)
	assert.Equal(t, domain.INVALID_TOKEN, customErr.StatusCode)
}

func TestGetDeviceID_Present(t *testing.T) {
	ctx := context.WithValue(context.Background(), middleware.DID{}, "device-xyz")

	deviceID, err := middleware.GetDeviceID(ctx)

	require.NoError(t, err)
	assert.Equal(t, "device-xyz", deviceID)
}

func TestGetDeviceID_Missing(t *testing.T) {
	_, err := middleware.GetDeviceID(context.Background())

	require.Error(t, err)
	customErr, ok := err.(*domain.CustomError)
	require.True(t, ok)
	assert.Equal(t, domain.INVALID_TOKEN, customErr.StatusCode)
}

func TestGetUser_Present(t *testing.T) {
	expected := domain.User{ID: 42, Name: "tester"}
	ctx := context.WithValue(context.Background(), middleware.USER{}, expected)

	user, err := middleware.GetUser(ctx)

	require.NoError(t, err)
	assert.Equal(t, &expected, user)
}

func TestGetUser_Missing(t *testing.T) {
	_, err := middleware.GetUser(context.Background())

	require.Error(t, err)
	customErr, ok := err.(*domain.CustomError)
	require.True(t, ok)
	assert.Equal(t, domain.INVALID_TOKEN, customErr.StatusCode)
}
