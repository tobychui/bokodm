package utils

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// Error messages from system tools regularly contain newlines and quotes
// (e.g. mkfs.vfat: "/dev/md0 contains a mounted filesystem.\nmkfs.fat 4.2"),
// the response must stay valid JSON so the frontend can display them.
func TestSendErrorResponseEscapesSpecialCharacters(t *testing.T) {
	cases := []string{
		"simple error",
		"mkfs.vfat: /dev/md0 contains a mounted filesystem.\nmkfs.fat 4.2 (2021-01-31)",
		`error with "quotes" and back\slash`,
		"tabs\tand\r\nCRLF",
	}

	for _, msg := range cases {
		rec := httptest.NewRecorder()
		SendErrorResponse(rec, msg)

		var parsed map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
			t.Errorf("response is not valid JSON for %q: %v (body: %s)", msg, err, rec.Body.String())
			continue
		}
		if parsed["error"] != msg {
			t.Errorf("message mangled: want %q, got %q", msg, parsed["error"])
		}
	}
}
