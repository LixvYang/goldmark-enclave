package main

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/asaskevich/govalidator"

	enclave "github.com/lixvyang/goldmark-enclave"
	"github.com/microcosm-cc/bluemonday"
	"github.com/segmentfault/pacman/log"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	goldmarkHTML "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

/*
![](https://discuss.mixin.one/uploads/post/5nn8Kd4aJmY.mp4)
# hello
![](https://discuss.mixin.one/uploads/post/5nn8Kd4aJmY.png)
![](https://discuss.mixin.one/uploads/post/5nn8Kd4aJmY.jpeg)
*/
func main() {
	source := `![](https://discuss.mixin.one/uploads/post/5nn8Kd4aJmY.jpg)
	
![](https://discuss.mixin.one/uploads/post/5nn8Kd4aJmY.mp4)

# hello world`
	html := Markdown2HTML(source)
	fmt.Println(html)
}

// Markdown2HTML convert markdown to html
func Markdown2HTML(source string) string {
	mdConverter := goldmark.New(
		goldmark.WithExtensions(&DangerousHTMLFilterExtension{}, extension.GFM, enclave.New(&enclave.Config{})),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			goldmarkHTML.WithHardWraps(),
		),
	)
	var buf bytes.Buffer
	if err := mdConverter.Convert([]byte(source), &buf); err != nil {
		log.Error(err)
		return source
	}
	html := buf.String()
	return html
}

type DangerousHTMLFilterExtension struct {
}

func (e *DangerousHTMLFilterExtension) Extend(m goldmark.Markdown) {
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(&DangerousHTMLRenderer{
			Config: goldmarkHTML.NewConfig(),
			Filter: bluemonday.UGCPolicy(),
		}, 1),
	))
}

type DangerousHTMLRenderer struct {
	goldmarkHTML.Config
	Filter *bluemonday.Policy
}

// RegisterFuncs implements renderer.NodeRenderer.RegisterFuncs.
func (r *DangerousHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	reg.Register(ast.KindRawHTML, r.renderRawHTML)
	reg.Register(ast.KindLink, r.renderLink)
	reg.Register(ast.KindAutoLink, r.renderAutoLink)
}

func (r *DangerousHTMLRenderer) renderRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n := node.(*ast.RawHTML)
	l := n.Segments.Len()
	for i := 0; i < l; i++ {
		segment := n.Segments.At(i)
		if string(source[segment.Start:segment.Stop]) == "<kbd>" || string(source[segment.Start:segment.Stop]) == "</kbd>" {
			_, _ = w.Write(segment.Value(source))
		} else {
			_, _ = w.Write(r.Filter.SanitizeBytes(segment.Value(source)))
		}
	}
	return ast.WalkSkipChildren, nil
}

func (r *DangerousHTMLRenderer) renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.HTMLBlock)
	if entering {
		l := n.Lines().Len()
		for i := 0; i < l; i++ {
			line := n.Lines().At(i)
			r.Writer.SecureWrite(w, r.Filter.SanitizeBytes(line.Value(source)))
		}
	} else {
		if n.HasClosure() {
			closure := n.ClosureLine
			r.Writer.SecureWrite(w, closure.Value(source))
		}
	}
	return ast.WalkContinue, nil
}

func (r *DangerousHTMLRenderer) renderLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)
	if entering && r.renderLinkIsUrl(string(n.Destination)) {
		_, _ = w.WriteString("<a href=\"")
		// _, _ = w.WriteString("<a test=\"1\" rel=\"nofollow\" href=\"")
		if r.Unsafe || !html.IsDangerousURL(n.Destination) {
			_, _ = w.Write(util.EscapeHTML(util.URLEscape(n.Destination, true)))
		}
		_ = w.WriteByte('"')
		if n.Title != nil {
			_, _ = w.WriteString(` title="`)
			r.Writer.Write(w, n.Title)
			_ = w.WriteByte('"')
		}
		if n.Attributes() != nil {
			html.RenderAttributes(w, n, html.LinkAttributeFilter)
		}
		_ = w.WriteByte('>')
	} else {
		_, _ = w.WriteString("</a>")
	}
	return ast.WalkContinue, nil
}

func (r *DangerousHTMLRenderer) renderAutoLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.AutoLink)

	if !entering || !r.renderLinkIsUrl(string(n.URL(source))) {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString(`<a href="`)
	url := n.URL(source)
	label := n.Label(source)
	if n.AutoLinkType == ast.AutoLinkEmail && !bytes.HasPrefix(bytes.ToLower(url), []byte("mailto:")) {
		_, _ = w.WriteString("mailto:")
	}
	_, _ = w.Write(util.EscapeHTML(util.URLEscape(url, false)))
	if n.Attributes() != nil {
		_ = w.WriteByte('"')
		html.RenderAttributes(w, n, html.LinkAttributeFilter)
		_ = w.WriteByte('>')
	} else {
		_, _ = w.WriteString(`">`)
	}
	_, _ = w.Write(util.EscapeHTML(label))
	_, _ = w.WriteString(`</a>`)
	return ast.WalkContinue, nil
}

func (r *DangerousHTMLRenderer) renderLinkIsUrl(verifyUrl string) bool {
	isURL := govalidator.IsURL(verifyUrl)
	isPath, _ := regexp.MatchString(`^/`, verifyUrl)
	return isURL || isPath
}
