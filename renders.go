// Copyright 2015 The Tango Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package renders

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/lunny/tango"
	"github.com/oxtoacart/bpool"
)

const (
	ContentType    = "Content-Type"
	ContentLength  = "Content-Length"
	ContentHTML    = "text/html"
	ContentXHTML   = "application/xhtml+xml"
	defaultCharset = "UTF-8"
)

// Provides a common buffer to execute templates.
type T map[string]interface{}

func (t T) Merge(at T) T {
	if len(at) <= 0 {
		return t
	}

	for k, v := range at {
		t[k] = v
	}
	return t
}

// Options is a struct for specifying configuration options for the render.Renderer middleware
type Options struct {
	// if reload templates
	Reload bool
	// Directory to load templates. Default is "templates"
	Directory string
	// Extensions to parse template files from. Defaults to [".tmpl"]
	Extensions []string
	// Funcs is a slice of FuncMaps to apply to the template upon compilation. This is useful for helper functions. Defaults to [].
	Funcs template.FuncMap
	// Vars is a data map for global
	Vars T
	// Appends the given charset to the Content-Type header. Default is "UTF-8".
	Charset string
	// Allows changing of output to XHTML instead of HTML. Default is "text/html"
	HTMLContentType string
	// default Delims
	DelimsLeft, DelimsRight string
}

type Renders struct {
	Options
	cs        string
	pool      *bpool.BufferPool
	templates map[string]*template.Template
}

func New(options ...Options) *Renders {
	opt := prepareOptions(options)
	t, err := compile(opt)
	if err != nil {
		panic(err)
	}
	return &Renders{
		Options:   opt,
		cs:        prepareCharset(opt.Charset),
		pool:      bpool.NewBufferPool(64),
		templates: t,
	}
}

type IRenderer interface {
	SetRenderer(*Renders, *tango.Context, func(string), func(string))
}

type Renderer struct {
	ctx                     *tango.Context
	renders                 *Renders
	before, after           func(string)
	t                       map[string]*template.Template
	compiledCharset         string
	Charset                 string
	HTMLContentType         string
	Funcs                   template.FuncMap
	delimsLeft, delimsRight string
}

func (r *Renderer) SetRenderer(renders *Renders, ctx *tango.Context, before, after func(string)) {
	r.renders = renders
	r.ctx = ctx
	r.before = before
	r.after = after
	r.t = renders.templates
	r.HTMLContentType = renders.Options.HTMLContentType
	r.compiledCharset = renders.cs
	r.Funcs = make(template.FuncMap)
	r.delimsLeft = renders.Options.DelimsLeft
	r.delimsRight = renders.Options.DelimsRight
}

type Before interface {
	BeforeRender(string)
}

type After interface {
	AfterRender(string)
}

func (r *Renders) RenderBytes(name string, bindings ...interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := r.Render(buf, name, bindings...)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (r *Renders) Render(w io.Writer, name string, bindings ...interface{}) error {
	var binding interface{}
	if len(bindings) > 0 {
		binding = bindings[0]
	}
	if t, ok := binding.(T); ok {
		binding = t.Merge(r.Options.Vars)
	}

	if r.Reload {
		var err error
		// recompile for easy development
		r.templates, err = compile(r.Options)
		if err != nil {
			return err
		}
	}

	buf, err := r.execute(name, binding)
	if err != nil {
		return err
	}

	// template rendered fine, write out the result
	_, err = io.Copy(w, buf)
	r.pool.Put(buf)
	return err
}

func (r *Renders) execute(name string, binding interface{}) (*bytes.Buffer, error) {
	buf := r.pool.Get()

	name = alignTmplName(name)

	if rt, ok := r.templates[name]; ok {
		return buf, rt.ExecuteTemplate(buf, name, binding)
	}
	return nil, errors.New("template is not exist")
}

func (r *Renders) Handle(ctx *tango.Context) {
	if action := ctx.Action(); action != nil {
		if rd, ok := action.(IRenderer); ok {
			var before, after func(string)
			if b, ok := action.(Before); ok {
				before = b.BeforeRender
			}
			if a, ok := action.(After); ok {
				after = a.AfterRender
			}

			rd.SetRenderer(r, ctx, before, after)
		}
	}

	ctx.Next()
}

func compile(options Options) (map[string]*template.Template, error) {
	if len(options.Funcs) > 0 {
		return LoadWithFuncMap(options)
	}
	return Load(options)
}

func prepareCharset(charset string) string {
	if len(charset) != 0 {
		return "; charset=" + charset
	}

	return "; charset=" + defaultCharset
}

func prepareOptions(options []Options) Options {
	var opt Options
	if len(options) > 0 {
		opt = options[0]
	}

	// Defaults
	if len(opt.Directory) == 0 {
		opt.Directory = "templates"
	}
	if len(opt.Extensions) == 0 {
		opt.Extensions = []string{".html"}
	}
	if len(opt.HTMLContentType) == 0 {
		opt.HTMLContentType = ContentHTML
	}
	if len(opt.DelimsLeft) == 0 {
		opt.DelimsLeft = "{{"
	}
	if len(opt.DelimsRight) == 0 {
		opt.DelimsRight = "}}"
	}

	return opt
}

// Render a template
//     r.Render("index.html")
//     r.Render("index.html", renders.T{
//                "name": value,
//           })
func (r *Renderer) Render(name string, bindings ...interface{}) error {
	return r.StatusRender(http.StatusOK, name, bindings...)
}

// This method Will not called before & after method.
func (r *Renderer) RenderBytes(name string, binding ...interface{}) ([]byte, error) {
	return r.renders.RenderBytes(name, binding...)
}

func (r *Renderer) StatusRender(status int, name string, bindings ...interface{}) error {
	var binding interface{}
	if len(bindings) > 0 {
		binding = bindings[0]
	}
	if t, ok := binding.(T); ok {
		binding = t.Merge(r.renders.Options.Vars)
	}

	buf, err := r.execute(name, binding)
	if err != nil {
		return err
	}

	var cs string
	if len(r.Charset) > 0 {
		cs = prepareCharset(r.Charset)
	} else {
		cs = r.compiledCharset
	}
	// template rendered fine, write out the result
	r.ctx.Header().Set(ContentType, r.HTMLContentType+cs)
	r.ctx.WriteHeader(status)
	_, err = io.Copy(r.ctx.ResponseWriter, buf)
	r.renders.pool.Put(buf)
	return err
}

func funcSignature(f interface{}) string {
	return fmt.Sprintf("%v", f)
}

var (
	sigTemplates map[string]*template.Template
)

func signature(funcs template.FuncMap) string {
	var sig string
	for k, f := range funcs {
		fmt.Sprintf("%s-%v", k, f)
	}
	return sig
}

func (r *Renderer) Template(name string) *template.Template {
	return r.t[alignTmplName(name)]
}

func (r *Renderer) execute(name string, binding interface{}) (*bytes.Buffer, error) {
	if len(r.Funcs) > 0 {
		// TODO: if has temprory funcs, then should recompile templates,
		// but the performance is lower.
	}

	buf := r.renders.pool.Get()
	if r.before != nil {
		r.before(name)
	}
	if r.after != nil {
		defer r.after(name)
	}

	name = alignTmplName(name)

	if rt, ok := r.t[name]; ok {
		return buf, rt.Delims(r.delimsLeft, r.delimsRight).ExecuteTemplate(buf, name, binding)
	}
	return nil, fmt.Errorf("template %s is not exist", name)
}

var (
	cache               []*namedTemplate
	regularTemplateDefs []string
	lock                sync.Mutex
	//re_defineTag        = regexp.MustCompile("{{ ?define \"([^\"]*)\" ?\"?([a-zA-Z0-9]*)?\"? ?}}")
	//re_defineTag = regexp.MustCompile("{{[ ]*define[ ]+\"([^\"]+)\"")
	//re_templateTag      = regexp.MustCompile("{{ ?template \"([^\"]*)\" ?([^ ]*)? ?}}")
	//re_templateTag = regexp.MustCompile("{{[ ]*template[ ]+\"([^\"]+)\"")
)

func getReDefineTag(delimsLeft string) *regexp.Regexp {
	return regexp.MustCompile(delimsLeft + "[ ]*define[ ]+\"([^\"]+)\"")
}

func getReTemplateTag(delimsLeft string) *regexp.Regexp {
	return regexp.MustCompile(delimsLeft + "[ ]*template[ ]+\"([^\"]+)\"")
}

type namedTemplate struct {
	Name string
	Src  string
}

// Load prepares and parses all templates from the passed basePath
func Load(opt Options) (map[string]*template.Template, error) {
	return loadTemplates(opt.Directory, opt.Extensions, opt.DelimsLeft, opt.DelimsRight, nil)
}

// LoadWithFuncMap prepares and parses all templates from the passed basePath and injects
// a custom template.FuncMap into each template
func LoadWithFuncMap(opt Options) (map[string]*template.Template, error) {
	return loadTemplates(opt.Directory, opt.Extensions, opt.DelimsLeft, opt.DelimsRight, opt.Funcs)
}

func alignTmplName(name string) string {
	name = strings.Replace(name, "\\\\", "/", -1)
	name = strings.Replace(name, "\\", "/", -1)
	return name
}

func loadTemplates(basePath string, exts []string, delimsLeft, delimsRight string, funcMap template.FuncMap) (map[string]*template.Template, error) {
	lock.Lock()
	defer lock.Unlock()

	templates := make(map[string]*template.Template)

	rootPath, _ := filepath.Abs(basePath)

	re_templateTag := getReTemplateTag(delimsLeft)
	re_defineTag := getReDefineTag(delimsLeft)

	err := filepath.Walk(rootPath, func(path string, fi os.FileInfo, err error) error {
		if fi == nil || fi.IsDir() {
			return nil
		}

		r, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}

		ext := filepath.Ext(r)
		var extRight bool
		for _, extension := range exts {
			if ext != extension {
				continue
			}
			extRight = true
			break
		}
		if !extRight {
			return nil
		}

		if err := add(rootPath, path, re_templateTag); err != nil {
			panic(err)
		}

		// Now we find all regular template definitions and check for the most recent definiton
		for _, t := range regularTemplateDefs {
			found := false
			defineIdx := 0

			// From the beginning (which should) most specifc we look for definitions
			for _, nt := range cache {
				nt.Src = re_defineTag.ReplaceAllStringFunc(nt.Src, func(raw string) string {
					parsed := re_defineTag.FindStringSubmatch(raw)
					name := parsed[1]
					if name != t {
						return raw
					}
					// Don't touch the first definition
					if !found {
						found = true
						return raw
					}

					defineIdx += 1

					return fmt.Sprintf(delimsLeft+" define \"%s_invalidated_#%d\" "+delimsRight, name, defineIdx)
				})
			}
		}

		var (
			baseTmpl *template.Template
			i        int
		)

		for _, nt := range cache {
			var currentTmpl *template.Template
			if i == 0 {
				baseTmpl = template.New(nt.Name).Delims(delimsLeft, delimsRight)
				currentTmpl = baseTmpl
			} else {
				currentTmpl = baseTmpl.New(nt.Name).Delims(delimsLeft, delimsRight)
			}

			template.Must(currentTmpl.Funcs(funcMap).Parse(nt.Src))
			i++
		}
		tname := generateTemplateName(rootPath, path)
		templates[tname] = baseTmpl

		// Make sure we empty the cache between runs
		cache = cache[0:0]
		return nil
	})

	return templates, err
}

func add(basePath, path string, re_templateTag *regexp.Regexp) error {
	// Get file content
	tplSrc, err := file_content(path)
	if err != nil {
		return err
	}

	tplName := generateTemplateName(basePath, path)

	// Make sure template is not already included
	alreadyIncluded := false
	for _, nt := range cache {
		if nt.Name == tplName {
			alreadyIncluded = true
			break
		}
	}
	if alreadyIncluded {
		return nil
	}

	// Add to the cache
	nt := &namedTemplate{
		Name: tplName,
		Src:  tplSrc,
	}
	cache = append(cache, nt)

	// Check for any template block
	for _, raw := range re_templateTag.FindAllString(nt.Src, -1) {
		parsed := re_templateTag.FindStringSubmatch(raw)
		templatePath := parsed[1]
		ext := filepath.Ext(templatePath)
		if !strings.Contains(templatePath, ext) {
			regularTemplateDefs = append(regularTemplateDefs, templatePath)
			continue
		}

		// Add this template and continue looking for more template blocks
		add(basePath, filepath.Join(basePath, templatePath), re_templateTag)
	}

	return nil
}

func isNil(a interface{}) bool {
	if a == nil {
		return true
	}
	aa := reflect.ValueOf(a)
	return !aa.IsValid() || (aa.Type().Kind() == reflect.Ptr && aa.IsNil())
}

func generateTemplateName(base, path string) string {
	return alignTmplName(path[len(base)+1:])
}

func Version() string {
	return "0.2.0510"
}

func file_content(path string) (string, error) {
	// Read the file content of the template
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	b, err := ioutil.ReadAll(file)
	if err != nil {
		return "", err
	}
	s := string(b)

	if len(s) < 1 {
		return "", errors.New("render: template file is empty")
	}

	return s, nil
}
