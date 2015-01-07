package renders

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"bytes"
	"io"
	"net/http"
	"errors"
	"io/ioutil"
	"reflect"

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

// Provides a temporary buffer to execute templates into and catch errors.

type T map[string]interface{}

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
}

type Renders struct {
	Options
	cs string
	pool *bpool.BufferPool
	templates map[string]*template.Template
}

func New(options ...Options) *Renders {
	opt := prepareOptions(options)
	t, err := compile(opt)
	if err != nil {
		panic(err)
	}
	return &Renders{
		Options: opt,
		cs: prepareCharset(opt.Charset),
		pool: bpool.NewBufferPool(64),
		templates: t,
	}
}

type IRenderer interface {
	SetRenderer(render *renderer)
}

type Renderer struct {
	*renderer
}
func (r *Renderer) SetRenderer(render *renderer) {
	r.renderer = render
}

type Before interface {
	BeforeRender(string)
}

type After interface {
	AfterRender(string)
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

			var templates = r.templates

			if r.Reload {
				var err error
				// recompile for easy development
				templates, err = compile(r.Options)
				if err != nil {
					panic(err)
				}
			}

			rd.SetRenderer(&renderer{
				renders: r,
				Context: ctx,
				action: action,
				before: before,
				after: after,
				t: templates,
				opt: r.Options,
				compiledCharset: r.cs,
			})
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

	return opt
}

type renderer struct {
	*tango.Context
	renders *Renders
	action interface{}
	before, after func(string)
	t               map[string]*template.Template
	opt             Options
	compiledCharset string
}

func (r *Renderer) Render(name string, binding interface{}) error {
	return r.StatusRender(http.StatusOK, name, binding)
}

func (r *Renderer) StatusRender(status int, name string, binding interface{}) error {
	buf, err := r.execute(name, binding)
	if err != nil {
		return err
	}

	// template rendered fine, write out the result
	r.Header().Set(ContentType, r.opt.HTMLContentType+r.compiledCharset)
	r.WriteHeader(status)
	_, err = io.Copy(r, buf)
	r.renders.pool.Put(buf)
	return err
}

func (r *Renderer) Template(name string) *template.Template {
	return r.t[name]
}

func (r *Renderer) execute(name string, binding interface{}) (*bytes.Buffer, error) {
	buf := r.renders.pool.Get()
	if r.before != nil {
		r.before(name)
	}
	if r.after != nil {
		defer r.after(name)
	}
	if rt, ok := r.t[name]; ok {
		return buf, rt.ExecuteTemplate(buf, name, binding)
	}
	return nil, errors.New("template is not exist")
}

var (
	cache               []*namedTemplate
	regularTemplateDefs []string
	lock                sync.Mutex
	//re_defineTag        = regexp.MustCompile("{{ ?define \"([^\"]*)\" ?\"?([a-zA-Z0-9]*)?\"? ?}}")
	re_defineTag        = regexp.MustCompile("{{[ ]*define[ ]+\"([^\"]+)\"")
	//re_templateTag      = regexp.MustCompile("{{ ?template \"([^\"]*)\" ?([^ ]*)? ?}}")
	re_templateTag      = regexp.MustCompile("{{[ ]*template[ ]+\"([^\"]+)\"")
)

type namedTemplate struct {
	Name string
	Src  string
}

// Load prepares and parses all templates from the passed basePath
func Load(opt Options) (map[string]*template.Template, error) {
	return loadTemplates(opt.Directory, opt.Extensions, nil)
}

// LoadWithFuncMap prepares and parses all templates from the passed basePath and injects
// a custom template.FuncMap into each template
func LoadWithFuncMap(opt Options) (map[string]*template.Template, error) {
	return loadTemplates(opt.Directory, opt.Extensions, opt.Funcs)
}

func loadTemplates(basePath string, exts []string, funcMap template.FuncMap) (map[string]*template.Template, error) {
	lock.Lock()
	defer lock.Unlock()

	templates := make(map[string]*template.Template)

	err := filepath.Walk(basePath, func(path string, fi os.FileInfo, err error) error {
		if fi == nil || fi.IsDir() {
			return nil
		}

		r, err := filepath.Rel(basePath, path)
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

		if err := add(basePath, path); err != nil {
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

					return fmt.Sprintf("{{ define \"%s_invalidated_#%d\" }}", name, defineIdx)
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
				baseTmpl = template.New(nt.Name)
				currentTmpl = baseTmpl
			} else {
				currentTmpl = baseTmpl.New(nt.Name)
			}

			template.Must(currentTmpl.Funcs(funcMap).Parse(nt.Src))
			i++
		}
		tname := generateTemplateName(basePath, path)
		templates[tname] = baseTmpl

		// Make sure we empty the cache between runs
		cache = cache[0:0]
		return nil
	})

	return templates, err
}

func add(basePath, path string) error {
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
		add(basePath, filepath.Join(basePath, templatePath))
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
	return filepath.ToSlash(path[len(base)+1:])
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