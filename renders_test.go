package renders

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/lunny/tango"
)

func TestIsNil(t *testing.T) {
	if !isNil(nil) {
		t.Error("nil")
	}

	if isNil(1) {
		t.Error("1")
	}

	if isNil("tttt") {
		t.Error("tttt")
	}

	type A struct {
	}

	var a A

	if isNil(a) {
		t.Error("a0")
	}

	if isNil(&a) {
		t.Error("a")
	}

	if isNil(new(A)) {
		t.Error("a2")
	}

	var b *A
	if !isNil(b) {
		t.Error("b")
	}

	var c interface{}
	if !isNil(c) {
		t.Error("c")
	}
}

type RenderAction struct {
	Renderer
}

func (a *RenderAction) Get() error {
	return a.Render("test1.html", T{
		"name": "tango",
	})
}

func TestRender_1(t *testing.T) {
	buff := bytes.NewBufferString("")
	recorder := httptest.NewRecorder()
	recorder.Body = buff

	o := tango.Classic()
	o.Use(New())
	o.Get("/", new(RenderAction))

	req, err := http.NewRequest("GET", "http://localhost:3000/", nil)
	if err != nil {
		t.Error(err)
	}

	o.ServeHTTP(recorder, req)
	expect(t, recorder.Code, http.StatusOK)
	refute(t, len(buff.String()), 0)
	expect(t, buff.String(), "Hello tango!")
}

var beforeAndAfter string

type Render2Action struct {
	Renderer
}

func (a *Render2Action) Get() error {
	return a.Render("test1.html", T{
		"name": "tango",
	})
}

func (a *Render2Action) BeforeRender(name string) {
	beforeAndAfter += "before " + name
}

func (a *Render2Action) AfterRender(name string) {
	beforeAndAfter += " after " + name
}

func TestRender_2(t *testing.T) {
	buff := bytes.NewBufferString("")
	recorder := httptest.NewRecorder()
	recorder.Body = buff

	o := tango.Classic()
	o.Use(New())
	o.Get("/", new(Render2Action))

	req, err := http.NewRequest("GET", "http://localhost:3000/", nil)
	if err != nil {
		t.Error(err)
	}

	o.ServeHTTP(recorder, req)
	expect(t, recorder.Code, http.StatusOK)
	refute(t, len(buff.String()), 0)
	expect(t, buff.String(), "Hello tango!")
	expect(t, beforeAndAfter, "before test1.html after test1.html")
}

type Render3Action struct {
	Renderer
}

func (a *Render3Action) Get() error {
	return a.Render("admin/home.html", nil)
}

func TestRender_3(t *testing.T) {
	buff := bytes.NewBufferString("")
	recorder := httptest.NewRecorder()
	recorder.Body = buff

	o := tango.Classic()
	o.Use(New())
	o.Get("/", new(Render3Action))

	req, err := http.NewRequest("GET", "http://localhost:3000/", nil)
	if err != nil {
		t.Error(err)
	}

	o.ServeHTTP(recorder, req)
	expect(t, recorder.Code, http.StatusOK)
	expect(t, buff.String(), "admin")
}

type Render4Action struct {
	Renderer
}

func (a *Render4Action) Get() error {
	return a.Render("admin\\home.html", nil)
}

func TestRender_4(t *testing.T) {
	buff := bytes.NewBufferString("")
	recorder := httptest.NewRecorder()
	recorder.Body = buff

	o := tango.Classic()
	o.Use(New(Options{
		Directory: "../renders/templates",
	}))
	o.Get("/", new(Render4Action))

	req, err := http.NewRequest("GET", "http://localhost:3000/", nil)
	if err != nil {
		t.Error(err)
	}

	o.ServeHTTP(recorder, req)
	expect(t, recorder.Code, http.StatusOK)
	expect(t, buff.String(), "admin")
}

/* Test Helpers */
func expect(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Errorf("Expected %v (type %v) - Got %v (type %v)", b, reflect.TypeOf(b), a, reflect.TypeOf(a))
	}
}

func refute(t *testing.T, a interface{}, b interface{}) {
	if a == b {
		t.Errorf("Did not expect %v (type %v) - Got %v (type %v)", b, reflect.TypeOf(b), a, reflect.TypeOf(a))
	}
}
