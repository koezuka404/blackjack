package dto

import "testing"

func TestOK(t *testing.T) {
	res := OK(map[string]string{"k": "v"})
	if !res.Success {
		t.Fatal("success should be true")
	}
	if res.Data["k"] != "v" {
		t.Fatalf("unexpected data: %#v", res.Data)
	}
}

func TestFail(t *testing.T) {
	res := Fail("invalid_input", "missing field")
	if res.Success {
		t.Fatal("success should be false")
	}
	if res.Error.Code != "invalid_input" {
		t.Fatalf("unexpected code: %s", res.Error.Code)
	}
	if res.Error.Message != "missing field" {
		t.Fatalf("unexpected message: %s", res.Error.Message)
	}
}

