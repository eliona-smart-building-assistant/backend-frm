package tenants

import (
	"encoding/json"
	"fmt"
	"uuid"

	"github.com/eliona-smart-building-assistant/backend-frm/pkg/kafka"
)

type tenantField struct {
	ID string `json:"tenant_id"`
}

// TenantIDFromKafkaRecord tries to find tenant id in either record key or record value.
// Search is performed as follows:
// 1. record key is parsed as UUID
// 2. record key is unmarshaled as JSON expecting field `tenant_id`
// 3. record value is unmarshaled as JSON expecting field `tenant_id`
//
// If all attempts failed, empty string and corresponding error is returned
func TenantIDFromKafkaRecord(r kafka.Record) (string, error) {
	candidate, err := uuid.Parse(string(r.Key))
	if err == nil {
		return candidate.String(), nil
	}

	t := tenantField{}

	err = json.Unmarshal(r.Key, &t)
	if err == nil && len(t.ID) > 0 {
		return t.ID, nil
	}

	err = json.Unmarshal(r.Value, &t)
	if err == nil && len(t.ID) > 0 {
		return t.ID, nil
	}

	return "", fmt.Errorf("tenant id not found in kafka record key and value")
}
