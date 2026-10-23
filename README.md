# Zedu API

The **Zedu API** is a scalable Go-based backend service that integrates multiple data sources and technologies such as PostgreSQL, Redis, MinIO, MongoDB, Firebase, RabbitMQ, TypeSense, and Stripe. It also supports background jobs, external service integration, and real-time notifications.


## 🚀 Features

- PostgreSQL + GORM for relational data
- Redis caching
- MinIO for object storage
- MongoDB support
- TypeSense for search indexing     <--No longer in use -->
- Elasticsearch integration
- Stripe for payments
- RabbitMQ for async task queues
- Firebase for push notifications
- Cron jobs for background tasks
- Real-time communication via Centrifugo
- Configurable through environment variables or config file
- Built-in database migrations and seeding


## Prerequisites

- Go >= 1.24
- PostgreSQL
- Redis
- MinIO
- MongoDB
- RabbitMQ
- TypeSense         <--No longer in use -->
- Elasticsearch
- Firebase (Service Account Key)
- Stripe account (for API key)
- Centrifugo

Make sure all services are installed and running locally or in Docker containers.


## Setup

1. Clone this repo.

```sh

    git clone https://github.com/telexorg/telex_be.git
```

2. Change your active directory in the project.

```sh

   cd telex_be
```

3. Copy `app-sample.env` into `app.env`.
   Swap out values if necessary. If you're using the Docker environment, remember to set any network host (e.g localhost) in your .env file to the name of the corresponding service in the `docker-compose.yaml` file.

4. Install Go dependencies.

```sh
    go mod tidy
```

5. Run the app.

```sh
    go run main.go
```

6. Confirm that the API is running by visiting `http://localhost:{port}`.

7. Get to work :).



## 🤝 Contributing

1. Fork the repository

2. Create your feature branch: `git checkout -b feature/your-feature`.

3. Commit your changes: `git commit -am 'Add some feature'`.
4. Push to the branch: `git push origin feature/your-feature`.
5. Open a pull request.



## 🏢 The Office

The office API lives under `api/v1/office` and is registered unconditionally.
Its rooms are derived from the organisation's channels, direct messages and
people, so nothing about the building is seeded.

### The switch

The office is an organisation setting, read on every request. Owners and
administrators turn it on and off in the organisation settings, under **The
Office**, which writes:

- `GET /api/v1/office/settings` — the current options, readable by anybody in
  the organisation;
- `PUT /api/v1/office/settings` — the same options, writable by owners and
  administrators only.

Support can also set it directly:

```sql
-- take the office away from an organisation
UPDATE organisations
SET office_settings = jsonb_set(COALESCE(office_settings, '{}'::jsonb), '{enabled}', 'false')
WHERE id = '<organisation-id>';

-- give it back
UPDATE organisations
SET office_settings = jsonb_set(COALESCE(office_settings, '{}'::jsonb), '{enabled}', 'true')
WHERE id = '<organisation-id>';
```

`GET /api/v1/office/status` answers `{ "enabled": true }` and is the only
office route that answers for a disabled organisation; every other office
route answers `403 office_disabled`.

### Realtime channels

```
office:org:{orgId}:floor:{floorIndex}:room:{encodedRoomId}:presence
office:org:{orgId}:floor:{floorIndex}:room:{encodedRoomId}:layout
```

The organisation is part of the channel name on purpose: the lobby and the
focus pods exist in every organisation, so a name without it would carry one
organisation's presence to another's. The subscription token endpoint only
signs a channel whose organisation is the caller's own.

The floor of a room is `0` for the general floor and `1` for a private office
until the tower's floor assignment lands.
