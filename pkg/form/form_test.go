package form

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/erazemkos/goflex/pkg/apiclient"
	"github.com/erazemkos/goflex/pkg/httperr"
	"github.com/erazemkos/goflex/pkg/ui"
)

type sample struct {
	Title       string `json:"title" validate:"required,min=2,max=120"`
	Description string `json:"description" validate:"max=1000"`
	Priority    int    `json:"priority" validate:"oneof=1 2 3"`
	Email       string `json:"email" validate:"omitempty,email"`
	Done        bool   `json:"done"`
}

func TestFormHoldsTypedValueAndSetters(t *testing.T) {
	f := UseForm(sample{Priority: 2})
	if f.Value().Priority != 2 || f.IsDirty() {
		t.Fatalf("initial value=%+v dirty=%v", f.Value(), f.IsDirty())
	}
	f.Set("Title", "Buy milk")
	f.Set("priority", "3")
	f.Set("Done", "true")
	if got := f.Value(); got.Title != "Buy milk" || got.Priority != 3 || !got.Done {
		t.Fatalf("value=%+v", got)
	}
	if !f.IsDirty() {
		t.Fatal("want dirty")
	}
}

func TestUnknownFieldPanicsInDevMode(t *testing.T) {
	old := ui.DevMode
	ui.DevMode = true
	defer func() { ui.DevMode = old }()
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected panic")
		}
	}()
	UseForm(sample{}).Set("Missing", "x")
}

func TestValidationOnChangeAndSubmit(t *testing.T) {
	f := UseForm(sample{Priority: 2})
	f.Set("Title", "a")
	if got := f.FieldError("Title"); got == "" {
		t.Fatal("want title error")
	}
	if f.IsValid() {
		t.Fatal("form should be invalid")
	}
	f.Set("Title", "ab")
	if got := f.FieldError("title"); got != "" {
		t.Fatalf("error should clear, got %q", got)
	}
	called := 0
	f.Submit(func(sample) error { called++; return nil })()
	if called != 1 {
		t.Fatalf("submit calls=%d", called)
	}

	invalid := UseForm(sample{Priority: 4}, WithMode(OnSubmitOnly))
	invalid.Set("Title", "a")
	if invalid.FieldError("Title") != "" {
		t.Fatal("submit-only should not validate on change")
	}
	invalid.Submit(func(sample) error { called++; return nil })()
	if called != 1 {
		t.Fatalf("invalid submit should be blocked, calls=%d", called)
	}
	if invalid.FieldError("Title") == "" || invalid.FieldError("Priority") == "" {
		t.Fatalf("missing submit errors title=%q priority=%q", invalid.FieldError("Title"), invalid.FieldError("Priority"))
	}
}

func TestSubmitHappyPathSubmittingState(t *testing.T) {
	f := UseForm(sample{Title: "ok", Priority: 2})
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		f.Submit(func(sample) error {
			close(started)
			<-release
			return nil
		})()
		close(done)
	}()
	<-started
	if !f.IsSubmitting() {
		t.Fatal("want submitting while callback is running")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("submit did not finish")
	}
	if f.IsSubmitting() || !f.IsValid() {
		t.Fatalf("submitting=%v valid=%v", f.IsSubmitting(), f.IsValid())
	}
}

func TestServerSideErrorsMerge(t *testing.T) {
	f := UseForm(sample{Title: "ok", Priority: 2})
	f.Submit(func(sample) error {
		return httperr.New("validation_failed", "bad input", map[string]string{"title": "already taken"})
	})()
	if got := f.FieldError("Title"); got != "already taken" {
		t.Fatalf("httperr field error=%q", got)
	}

	f = UseForm(sample{Title: "ok", Priority: 2})
	f.Submit(func(sample) error {
		return apiclient.FieldError{Code: apiclient.Code("validation_failed"), Fields: map[string]string{"Title": "reserved"}}
	})()
	if got := f.FieldError("title"); got != "reserved" {
		t.Fatalf("field-like error=%q", got)
	}
}

func TestResetAndDirtyTracking(t *testing.T) {
	initial := sample{Title: "ab", Priority: 2}
	f := UseForm(initial)
	f.Set("Title", "changed")
	if !f.IsDirty() {
		t.Fatal("want dirty after change")
	}
	f.Reset()
	if f.IsDirty() || !reflect.DeepEqual(f.Value(), initial) || !f.IsValid() {
		t.Fatalf("after reset value=%+v dirty=%v valid=%v", f.Value(), f.IsDirty(), f.IsValid())
	}
}

func TestFieldBindingAndCustomValidator(t *testing.T) {
	f := UseForm(sample{Title: "ok", Priority: 2}, WithValidator("Title", func(value any, whole any) string {
		if value == "forbidden" {
			return "not allowed"
		}
		return ""
	}))
	field := f.Field("Priority")
	if _, ok := field.Value().(int); !ok || field.Value() != 2 {
		t.Fatalf("priority binding value=%#v", field.Value())
	}
	field.Set("3")
	if f.Value().Priority != 3 {
		t.Fatalf("priority=%d", f.Value().Priority)
	}
	f.Set("Title", "forbidden")
	if got := f.FieldError("Title"); got != "not allowed" {
		t.Fatalf("custom error=%q", got)
	}
}

func TestValidateParityAndMessages(t *testing.T) {
	cases := []struct {
		name string
		in   sample
		want map[string]string
	}{
		{"missing title", sample{Priority: 2}, map[string]string{"title": "required"}},
		{"short title", sample{Title: "a", Priority: 2}, map[string]string{"title": "min=2"}},
		{"bad priority", sample{Title: "ok", Priority: 9}, map[string]string{"priority": "oneof=1 2 3"}},
		{"bad email", sample{Title: "ok", Priority: 2, Email: "bad"}, map[string]string{"email": "email"}},
		{"valid", sample{Title: "ok", Priority: 2, Email: "a@example.com"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := Validate(tc.in)
			server := Validate(tc.in)
			if !reflect.DeepEqual(client, server) {
				t.Fatalf("client=%v server=%v", client, server)
			}
			if !reflect.DeepEqual(client, tc.want) {
				t.Fatalf("got=%v want=%v", client, tc.want)
			}
		})
	}
}

func TestSubmitPreservesGenericErrors(t *testing.T) {
	f := UseForm(sample{Title: "ok", Priority: 2})
	errBoom := errors.New("boom")
	f.Submit(func(sample) error { return errBoom })()
	if !f.IsValid() {
		t.Fatal("generic errors should not create field errors")
	}
}

func TestNonDevUnknownFieldAndInvalidConversion(t *testing.T) {
	old := ui.DevMode
	ui.DevMode = false
	f := UseForm(sample{})
	f.Set("Missing", "x")
	_ = f.Field("Missing")
	ui.DevMode = true
	defer func() { ui.DevMode = old }()
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected invalid conversion panic")
		}
	}()
	f.Set("Priority", "not-a-number")
}

func TestPointerFormAndNilValues(t *testing.T) {
	type pointerSample struct {
		Count *int `json:"count" validate:"omitempty,min=1"`
	}
	one := 1
	f := UseForm(&pointerSample{Count: &one})
	if f.Field("count").Value().(*int) == nil || *f.Field("count").Value().(*int) != 1 {
		t.Fatalf("bad pointer field: %#v", f.Field("count").Value())
	}
	f.Set("Count", nil)
	if f.Value().Count != nil {
		t.Fatal("nil should clear pointer")
	}
	f.Set("Count", "2")
	if f.Value().Count == nil || *f.Value().Count != 2 {
		t.Fatalf("pointer conversion failed: %+v", f.Value())
	}
}

func TestValidateWithJSONNamedCustomValidator(t *testing.T) {
	got := ValidateWith(sample{Title: "ok", Priority: 2}, CustomValidators{
		"title": func(value any, whole any) string { return "json custom" },
	})
	if got["title"] != "json custom" {
		t.Fatalf("custom validation=%v", got)
	}
}
