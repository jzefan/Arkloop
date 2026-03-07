package data

import "github.com/google/uuid"

func WebhookSecretName(endpointID uuid.UUID) string {
	return "webhook_endpoint:" + endpointID.String()
}
