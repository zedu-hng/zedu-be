package firebase

import (
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
)

type FirebaseClient struct {
	App    *firebase.App
	Client *messaging.Client
}

var Client *FirebaseClient = &FirebaseClient{}

func Connection() *FirebaseClient {
	return Client
}
