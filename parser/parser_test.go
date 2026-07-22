package parser

import "testing"

func TestParseLengthPrefixedSet(t *testing.T) {
	cmd, err := Parse("SET name 12 EX 5")
	if err != nil {
		t.Fatal(err)
	}

	if cmd.Name != "SET" {
		t.Fatalf("expected SET got %s", cmd.Name)
	}

	if cmd.Key != "name" {
		t.Fatalf("expected name got %s", cmd.Key)
	}

	if cmd.ValueLength != 12 {
		t.Fatalf("expected value length 12 got %d", cmd.ValueLength)
	}

	if cmd.TTL != 5 {
		t.Fatalf("expected ttl 5 got %d", cmd.TTL)
	}
}

func TestParseSetWithoutTTL(t *testing.T) {
	cmd, err := Parse("SET name 12")
	if err != nil {
		t.Fatal(err)
	}

	if cmd.ValueLength != 12 {
		t.Fatalf("expected value length 12 got %d", cmd.ValueLength)
	}
}

func TestParseRejectsLegacyValueFormat(t *testing.T) {
	if _, err := Parse("SET name alice"); err == nil {
		t.Fatal("expected legacy space-delimited value format to fail")
	}
}
