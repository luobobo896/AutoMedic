package ws

import (
	"encoding/json"
	"strconv"
)

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func parseID(s string) (uint, bool) {
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}
