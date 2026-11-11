package storage

import "testing"

func TestPublicURL(t *testing.T) {
	cases := map[string]string{
		"uploads/12/a8K2mP9x/photo_1.jpg":       "/static/materials/12/a8K2mP9x/photo_1.jpg",
		"uploads/avatars/a8K2mP9x/avatar_1.jpg": "/static/avatars/a8K2mP9x/avatar_1.jpg",
		"https://cdn.example.com/x.jpg":         "https://cdn.example.com/x.jpg",
		"":                                      "",
	}
	for in, want := range cases {
		if got := PublicURL(in); got != want {
			t.Errorf("PublicURL(%q) = %q, want %q", in, got, want)
		}
	}
}
