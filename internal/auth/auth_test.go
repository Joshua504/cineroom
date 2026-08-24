package auth

import (
	"net/http/httptest"
	"testing"
)

func TestPasswordAndSession(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if CheckPassword(hash, "wrong") == nil {
		t.Fatal("wrong password accepted")
	}
	manager := New([]byte("01234567890123456789012345678901"), false)
	recorder := httptest.NewRecorder()
	if err := manager.SetSession(recorder, User{ID: "u1", Email: "a@example.com", Username: "alice"}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range recorder.Result().Cookies() {
		req.AddCookie(cookie)
	}
	user, err := manager.Current(req)
	if err != nil || user.ID != "u1" || user.Username != "alice" {
		t.Fatalf("unexpected session: %#v, %v", user, err)
	}
}
