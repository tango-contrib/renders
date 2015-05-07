// Copyright 2015 The Tango Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package renders

import (
	"bytes"
	"fmt"
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

func TestRender1(t *testing.T) {
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

func TestRender2(t *testing.T) {
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

func TestRender3(t *testing.T) {
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

func TestRender4(t *testing.T) {
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

type Render5Action struct {
	Renderer
}

func (a *Render5Action) Get() error {
	return a.Render("test2.html", T{
		"name": "lunny",
	})
}

func TestRender5(t *testing.T) {
	buff := bytes.NewBufferString("")
	recorder := httptest.NewRecorder()
	recorder.Body = buff

	o := tango.Classic()
	o.Use(New(Options{
		DelimsLeft:  "[[",
		DelimsRight: "]]",
		Reload:      true,
	}))
	o.Get("/", new(Render5Action))

	req, err := http.NewRequest("GET", "http://localhost:3000/", nil)
	if err != nil {
		t.Error(err)
	}

	o.ServeHTTP(recorder, req)
	expect(t, recorder.Code, http.StatusOK)
	expect(t, buff.String(), "Hello lunny!")
}

type Render6Action struct {
	Renderer
}

func (a *Render6Action) Get() error {
	return a.Render("test3.html", T{
		"name": "lunny",
	})
}

func TestRender6(t *testing.T) {
	buff := bytes.NewBufferString("")
	recorder := httptest.NewRecorder()
	recorder.Body = buff

	o := tango.Classic()
	o.Use(New(Options{
		DelimsLeft:  "[[",
		DelimsRight: "]]",
	}))
	o.Get("/", new(Render6Action))

	req, err := http.NewRequest("GET", "http://localhost:3000/", nil)
	if err != nil {
		t.Error(err)
	}

	o.ServeHTTP(recorder, req)
	expect(t, recorder.Code, http.StatusOK)
	fmt.Println("output:", buff.String())
	expect(t, buff.String(), "------ begin test2 ------\nHello lunny!\n------ end test2 ------")
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
