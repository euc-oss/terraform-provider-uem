package profile

import (
	"encoding/json"
	"sort"
)

// payloadSectionNames returns the sorted list of non-nil top-level JSON
// section names present on entity (e.g. "General", "NetworkList",
// "Passcode"), without exposing any field values.
//
// It exists so Create/Update logging can report *what* is being sent to
// UEM without ever marshalling (and thus logging) the full payload: typed
// profile entities carry secret-bearing sections (network/credentials
// passwords, macOS passcodes, unmodeled General.Password preserved from a
// prior read-modify-write) that must never reach TF_LOG output.
//
// On marshal/unmarshal failure it returns nil, signalling callers to skip
// the log line entirely (mirroring the historical "skip on Marshal error"
// behavior).
func payloadSectionNames(entity any) []string {
	body, err := json.Marshal(entity)
	if err != nil {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	names := make([]string, 0, len(raw))
	for k, v := range raw {
		if string(v) == "null" {
			continue
		}
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
