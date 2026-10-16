package onesignal

import (
	onesignalapi "github.com/OneSignal/onesignal-go-api/v5"
)

type OneSignalClient struct {
	Client *onesignalapi.APIClient
	AppID  string
	ApiKey string
}

var Client *OneSignalClient = &OneSignalClient{}

// Connection returns the global OneSignal client
func Connection() *OneSignalClient {
	return Client
}

func IsEnabled() bool {
	return Client.Client != nil && Client.AppID != "" && Client.ApiKey != ""
}
