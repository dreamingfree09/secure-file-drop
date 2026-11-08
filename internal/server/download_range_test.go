package server

import "testing"

func TestSingleDownloadRange(t *testing.T) {
	for _, tc := range []struct {
		value string
		size  int64
		want  downloadRange
		valid bool
	}{
		{"bytes=2-4", 10, downloadRange{2, 4}, true},
		{"bytes=8-100", 10, downloadRange{8, 9}, true},
		{"bytes=1-", 10, downloadRange{1, 9}, true},
		{"bytes=-3", 10, downloadRange{7, 9}, true},
		{"bytes=-40", 10, downloadRange{0, 9}, true},
		{"bytes=107374182399-", 107374182400, downloadRange{107374182399, 107374182399}, true},
		{"bytes=10-", 10, downloadRange{}, false},
		{"bytes=8-4", 10, downloadRange{}, false},
		{"bytes=0-", 0, downloadRange{}, false},
		{"bytes=-0", 10, downloadRange{}, false},
		{"bytes=+1-2", 10, downloadRange{}, false},
		{"bytes=2--1", 10, downloadRange{}, false},
		{"bytes=9223372036854775808-", 10, downloadRange{}, false},
	} {
		if got, valid := parseDownloadRange(tc.value, tc.size); got != tc.want || valid != tc.valid {
			t.Errorf("range %q with length %d = (%+v, %t), want (%+v, %t)",
				tc.value, tc.size, got, valid, tc.want, tc.valid)
		}
	}
	if downloadETag("part-hash-2") != "\"part-hash-2\"" || downloadETag("untrusted\"value") != "" {
		t.Error("storage ETag must be safely quoted before use")
	}
}
