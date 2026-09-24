package api

import "testing"

func TestNegotiate_Empty(t *testing.T) {
	v, exp := negotiateVersion("")
	if v != "1" || exp {
		t.Fatalf("empty Accept → default v1 implicit, got %q explicit=%v", v, exp)
	}
}

func TestNegotiate_StarStar(t *testing.T) {
	v, exp := negotiateVersion("*/*")
	if v != "1" || exp {
		t.Fatalf("*/* → default v1 implicit, got %q explicit=%v", v, exp)
	}
}

func TestNegotiate_PlainJSON(t *testing.T) {
	v, exp := negotiateVersion("application/json")
	if v != "1" || exp {
		t.Fatalf("application/json → default v1 implicit, got %q explicit=%v", v, exp)
	}
}

func TestNegotiate_VndWithoutVersion(t *testing.T) {
	v, exp := negotiateVersion("application/vnd.cp+json")
	if v != "1" || !exp {
		t.Fatalf("vnd.cp+json → default v1 explicit, got %q explicit=%v", v, exp)
	}
}

func TestNegotiate_VndVersion1(t *testing.T) {
	v, exp := negotiateVersion("application/vnd.cp+json; version=1")
	if v != "1" || !exp {
		t.Fatalf("version=1 → v1 explicit, got %q explicit=%v", v, exp)
	}
}

func TestNegotiate_UnknownVersion(t *testing.T) {
	v, exp := negotiateVersion("application/vnd.cp+json; version=99")
	if v != "" || !exp {
		t.Fatalf("version=99 → empty explicit (406), got %q explicit=%v", v, exp)
	}
}

func TestNegotiate_MultipleWithFallback(t *testing.T) {
	// vnd.cp+json with unknown v, but also plain json — must still 406,
	// because the client asked for our type by name.
	v, exp := negotiateVersion("application/vnd.cp+json; version=42, application/json")
	if v != "" || !exp {
		t.Fatalf("mixed with unknown vnd → 406, got %q explicit=%v", v, exp)
	}
}

func TestNegotiate_MultipleWithMatch(t *testing.T) {
	v, exp := negotiateVersion("application/vnd.cp+json; version=1, application/json")
	if v != "1" || !exp {
		t.Fatalf("mixed with matching vnd → v1 explicit, got %q explicit=%v", v, exp)
	}
}

func TestNegotiate_QuotedVersion(t *testing.T) {
	v, exp := negotiateVersion(`application/vnd.cp+json; version="1"`)
	if v != "1" || !exp {
		t.Fatalf("quoted version=\"1\" → v1 explicit, got %q explicit=%v", v, exp)
	}
}

func TestParseMediaType_CaseInsensitive(t *testing.T) {
	media, params := parseMediaType("Application/VND.CP+JSON; Version=1")
	if media != "application/vnd.cp+json" {
		t.Fatalf("case-insensitive media type failed: %q", media)
	}
	if params["version"] != "1" {
		t.Fatalf("case-insensitive param key failed: %#v", params)
	}
}
