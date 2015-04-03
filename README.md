renders [![Build Status](https://drone.io/github.com/tango-contrib/renders/status.png)](https://drone.io/github.com/tango-contrib/renders/latest) [![](http://gocover.io/_badge/github.com/tango-contrib/renders)](http://gocover.io/github.com/tango-contrib/renders)
======

Middleware renders is a go template render middlewaer for [Tango](https://github.com/lunny/tango). 

## Installation

    go get github.com/tango-contrib/renders

## Simple Example

```Go
type RenderAction struct {
    renders.Renderer
}

func (x *RenderAction) Get() {
    x.Render("test.html", renders.T{
        "test": "test",
    })
}

func main() {
    t := tango.Classic()
    t.Use(renders.New(renders.Options{
        Reload: true,
        Directory: "./templates",
    }))
}
```