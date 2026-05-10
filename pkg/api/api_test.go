package api

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestEndpointRegister(t *testing.T) {
	Registry.ResetForTest()
	ep := Endpoint[struct{}, struct{}]{Method: "get", Path: "/x"}
	ep.Register(func(ctx Context, req struct{}) (struct{}, error) { return struct{}{}, nil })
	all := Registry.All()
	if len(all) != 1 || all[0].Method != "GET" || all[0].Path != "/x" {
		t.Fatalf("all=%+v", all)
	}
	defer func() {
		if recovered := recover(); recovered == nil || !strings.Contains(recovered.(string), "duplicate endpoint GET /x") {
			t.Fatalf("want duplicate panic, got %v", recovered)
		}
	}()
	Registry.Register("GET", "/x", ep)
}

func TestRegistryAllDeterministic(t *testing.T) {
	Registry.ResetForTest()
	Registry.Register("POST", "/b", Endpoint[struct{}, struct{}]{})
	Registry.Register("GET", "/a", Endpoint[struct{}, struct{}]{})
	got := Registry.All()
	if got[0].Method != "GET" || got[0].Path != "/a" || got[1].Method != "POST" {
		t.Fatalf("not sorted: %+v", got)
	}
}

func TestDecodeRequest(t *testing.T) {
	type request struct {
		ID    uint   `path:"id" json:"-"`
		Q     string `query:"q" json:"-"`
		Limit int    `query:"limit" json:"-"`
		Title string `json:"title"`
	}
	req := httptest.NewRequest(http.MethodPost, "/todos/42?q=hello+world&limit=10", strings.NewReader(`{"title":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	var decoded request
	err := DecodeRequest(req, func(name string) string {
		if name == "id" {
			return "42"
		}
		return ""
	}, &decoded)
	if err != nil {
		t.Fatal(err)
	}
	want := request{ID: 42, Q: "hello world", Limit: 10, Title: "x"}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("decoded=%+v want=%+v", decoded, want)
	}
}
